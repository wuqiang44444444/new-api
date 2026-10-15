/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { USD_PRICING_CURRENCY } from '@/features/model-pricing/currency'
import { api } from '@/lib/api'

import { RequestSimulation } from '../request-simulation'
import { TieredPricingEditor } from '../tiered-pricing-editor'

const requestExpression =
  'tier("base", p * 2) * (param("priority") == true ? 2 : 1) * (header("tier") == "fast" ? 3 : 1)'

afterEach(() => vi.useRealTimers())

function response(cost: number, matchedTier = 'base') {
  return {
    data: {
      success: true,
      data: [
        {
          key: 'preview',
          evaluation: {
            raw_cost_usd: cost,
            quota: 0,
            matched_tier: matchedTier,
            pricing_time: '2026-10-15T00:00:00Z',
            usage_semantic: 'normalized',
            normalized_usage: {},
            request_rules: [
              {
                cond: 'param("priority") == true',
                multiplier: 2,
                matched: true,
              },
              {
                cond: 'header("tier") == "fast"',
                multiplier: 3,
                matched: true,
              },
            ],
          },
        },
      ],
    },
  }
}

beforeEach(() => vi.spyOn(api, 'post').mockResolvedValue(response(2)))

const mockCost = (cost: number, tier = 'base') =>
  vi.mocked(api.post).mockResolvedValue(response(cost, tier))

async function expectCost(cost: string) {
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent(
      `Simulated request cost: $${cost}`
    )
  )
}

describe('request simulation', () => {
  test('reports task usage from the authoritative engine without displaying a zero estimate', async () => {
    vi.mocked(api.post).mockResolvedValue({
      data: {
        success: true,
        data: [
          {
            key: 'preview',
            error:
              'task usage expressions use a different unit contract and are not supported by this preview',
          },
        ],
      },
    })
    render(
      <TieredPricingEditor
        billingExpr='tier("standard", u("seconds") * 0.09)'
        requestRuleExpr=''
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(
      await screen.findByText(/Expression error:.*task usage/)
    ).toBeVisible()
    expect(screen.queryByText(/Estimated cost:/)).not.toBeInTheDocument()
  })
  test('simulates billable image count separately from requested n and rejects invalid quantities', async () => {
    render(
      <RequestSimulation
        expression='tier("image", fixed(0.04)) * image_count'
        mode='token'
        currency={USD_PRICING_CURRENCY}
      />
    )
    mockCost(0.08)
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: 'Request simulation' }))
    fireEvent.input(
      screen.getByRole('textbox', { name: 'Simulated request body' }),
      { target: { value: '{"n":3}' } }
    )
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Billable image count' }),
      { target: { value: '2' } }
    )
    await expectCost('0.08')
    expect(api.post).toHaveBeenLastCalledWith(
      '/api/option/billing-expression/preview',
      {
        items: [
          expect.objectContaining({
            sample: expect.objectContaining({ image_count: 2, body: { n: 3 } }),
          }),
        ],
      }
    )
    vi.mocked(api.post).mockResolvedValue({
      data: {
        success: true,
        data: [
          { key: 'preview', error: 'image_count must be between 1 and 128' },
        ],
      },
    })
    fireEvent.change(
      screen.getByRole('spinbutton', { name: 'Billable image count' }),
      { target: { value: '129' } }
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'image_count must be between 1 and 128'
    )
  })
  test('simulates schema boolean usage without changing public task matrix rules', async () => {
    const user = userEvent.setup()
    render(
      <RequestSimulation
        expression='u("audio") == true ? tier("audio", u("seconds") * 0.8) : tier("silent", u("seconds") * 0.4)'
        usage={{ seconds: 5, audio: true }}
        usageSchema={{
          seconds: { type: 'number', unit: 'second' },
          audio: { type: 'boolean', description: 'Audio' },
        }}
        mode='task'
      />
    )
    mockCost(4)
    await user.click(screen.getByRole('button', { name: 'Request simulation' }))
    await expectCost('4')
    await user.click(screen.getByRole('combobox', { name: 'Audio' }))
    mockCost(2)
    await user.click(screen.getByRole('option', { name: 'No' }))
    await expectCost('2')
  })
  test('only supplies request context after opening and recalculates body and header rules', async () => {
    const user = userEvent.setup()
    render(
      <RequestSimulation
        expression={requestExpression}
        tokens={{ p: 1000000 }}
        mode='token'
        currency={USD_PRICING_CURRENCY}
      />
    )
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Request simulation' }))
    await expectCost('2')
    mockCost(4)
    fireEvent.input(
      screen.getByRole('textbox', { name: 'Simulated request body' }),
      { target: { value: '{"priority":true}' } }
    )
    await expectCost('4')
    const headers = screen.getByRole('group', {
      name: 'Simulated request headers',
    })
    await user.click(within(headers).getByRole('button', { name: 'JSON Mode' }))
    mockCost(12)
    fireEvent.input(within(headers).getByRole('textbox', { name: 'JSON' }), {
      target: { value: '{" TiEr ": " fast "}' },
    })
    await expectCost('12')
    expect(
      within(screen.getByRole('status')).getAllByText(/Matched/)
    ).toHaveLength(2)
  })

  test('removes the previous result when JSON becomes invalid', async () => {
    const user = userEvent.setup()
    render(
      <RequestSimulation
        expression={requestExpression}
        tokens={{ p: 1000000 }}
        mode='token'
      />
    )
    await user.click(screen.getByRole('button', { name: 'Request simulation' }))
    fireEvent.input(
      screen.getByRole('textbox', { name: 'Simulated request body' }),
      { target: { value: '{' } }
    )
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Request body must be a JSON object.'
    )
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    fireEvent.input(
      screen.getByRole('textbox', { name: 'Simulated request body' }),
      { target: { value: '{}' } }
    )
    await expectCost('2')
  })

  test('uses a specified instant and rejects a timestamp without a timezone', async () => {
    const user = userEvent.setup()
    render(
      <RequestSimulation
        expression='hour("Asia/Shanghai") < 12 ? tier("morning", 1) : tier("afternoon", 2)'
        mode='task'
      />
    )
    await user.click(screen.getByRole('button', { name: 'Request simulation' }))
    await user.click(screen.getByRole('combobox', { name: 'Simulation time' }))
    await user.click(screen.getByRole('option', { name: 'Specified time' }))
    const input = screen.getByRole('textbox', { name: 'Specified time' })
    mockCost(1, 'morning')
    fireEvent.change(input, { target: { value: '2026-09-07T09:00:00+08:00' } })
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Hit tier: morning')
    )
    mockCost(2, 'afternoon')
    fireEvent.change(input, { target: { value: '2026-09-07T14:00:00+08:00' } })
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(
        'Hit tier: afternoon'
      )
    )
    expect(api.post).toHaveBeenLastCalledWith(
      '/api/option/billing-expression/preview',
      {
        items: [
          expect.objectContaining({
            sample: expect.objectContaining({
              pricing_time: '2026-09-07T06:00:00.000Z',
            }),
          }),
        ],
      }
    )
    fireEvent.change(input, { target: { value: '2026-09-07T14:00:00' } })
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Enter an ISO date and time with a timezone offset.'
    )
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  test('keeps task output in cost units and refreshes on usage changes', async () => {
    const user = userEvent.setup()
    const expression =
      'tier("task", u("seconds") * 0.4) * (param("priority") == true ? 2 : 1)'
    const view = render(
      <RequestSimulation
        expression={expression}
        usage={{ seconds: 5 }}
        mode='task'
      />
    )
    await user.click(screen.getByRole('button', { name: 'Request simulation' }))
    await expectCost('2')
    mockCost(4)
    view.rerender(
      <RequestSimulation
        expression={expression}
        usage={{ seconds: 10 }}
        mode='task'
      />
    )
    await expectCost('4')
  })

  test('keeps the basic token estimate separate and never publishes simulated values to pricing', async () => {
    const user = userEvent.setup()
    const onBillingExprChange = vi.fn()
    const onRequestRuleExprChange = vi.fn()
    render(
      <TieredPricingEditor
        modelName='test-model'
        billingExpr='tier("base", p * 2 + c * 8)'
        requestRuleExpr='(param("priority") == true ? 2 : 1)'
        onBillingExprChange={onBillingExprChange}
        onRequestRuleExprChange={onRequestRuleExprChange}
      />
    )
    fireEvent.change(screen.getByLabelText('Input tokens'), {
      target: { value: '1000000' },
    })
    fireEvent.blur(screen.getByLabelText('Input tokens'))
    await waitFor(() =>
      expect(
        screen.getByText(/Estimated cost/).parentElement
      ).toHaveTextContent('$2')
    )
    onBillingExprChange.mockClear()
    onRequestRuleExprChange.mockClear()
    await user.click(screen.getByRole('button', { name: 'Request simulation' }))
    mockCost(4)
    fireEvent.input(
      screen.getByRole('textbox', { name: 'Simulated request body' }),
      { target: { value: '{"priority":true}' } }
    )
    await waitFor(() =>
      expect(screen.getByText(/Simulated request cost/)).toHaveTextContent('$4')
    )
    expect(screen.getByText(/Estimated cost/).parentElement).toHaveTextContent(
      '$2'
    )
    expect(onBillingExprChange).not.toHaveBeenCalled()
    expect(onRequestRuleExprChange).not.toHaveBeenCalled()
  })
})
