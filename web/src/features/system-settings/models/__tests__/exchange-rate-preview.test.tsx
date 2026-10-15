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
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { RequestSimulation } from '../request-simulation'
import { TieredPricingEditor } from '../tiered-pricing-editor'

const expression =
  'tier("standard", (p * 12 + c * 36 + cr * 1 + cc * 15) / usd_exchange_rate())'

const rate = {
  source_key: 'USDExchangeRate',
  rate: 7,
  frozen_at: '2026-10-15T00:00:00Z',
}

function previewResponse(usd: number) {
  return {
    data: {
      success: true,
      data: [
        {
          key: 'preview',
          evaluation: {
            raw_cost_usd: usd,
            quota: 0,
            matched_tier: 'standard',
            pricing_time: rate.frozen_at,
            usage_semantic: 'normalized',
            normalized_usage: {},
            usd_exchange_rate: rate,
            billing_unit: 'token',
            request_rules: [],
          },
        },
      ],
    },
  }
}

beforeEach(() => vi.restoreAllMocks())

describe('exchange rate token pricing preview', () => {
  it('evaluates the original expression on the server and keeps cache inputs available', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue(previewResponse(0))
    render(
      <TieredPricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Estimated cost: $0')
    )
    expect(screen.getByRole('status')).toHaveTextContent(
      'Applied exchange rate (CNY/USD): 7'
    )
    expect(screen.getByLabelText('Cache Read')).toBeVisible()
    expect(screen.getByLabelText('Cache Write')).toBeVisible()
    expect(post).toHaveBeenCalledWith(
      '/api/option/billing-expression/preview',
      {
        items: [
          expect.objectContaining({
            expression,
            sample: expect.objectContaining({
              usage_semantic: 'normalized',
              normalized_usage: expect.objectContaining({
                p: 0,
                c: 0,
                cr: 0,
                cc: 0,
              }),
            }),
          }),
        ],
      }
    )

    post.mockResolvedValue(previewResponse(12 / 7))
    fireEvent.change(screen.getByLabelText('Input tokens'), {
      target: { value: '1000000' },
    })
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('$1.714285714286')
    )
    expect(post).toHaveBeenLastCalledWith(
      '/api/option/billing-expression/preview',
      {
        items: [
          expect.objectContaining({
            expression,
            sample: expect.objectContaining({
              normalized_usage: expect.objectContaining({
                p: 1000000,
                len: 1000000,
              }),
            }),
          }),
        ],
      }
    )
    expect(screen.queryByText(/Expression error:/)).not.toBeInTheDocument()
  })

  it('sends cache dimensions and a length override without changing the saved expression', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue(previewResponse(2))
    const onBillingExprChange = vi.fn()
    const onRequestRuleExprChange = vi.fn()
    render(
      <TieredPricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        onBillingExprChange={onBillingExprChange}
        onRequestRuleExprChange={onRequestRuleExprChange}
      />
    )
    fireEvent.change(screen.getByLabelText('Input tokens'), {
      target: { value: '1000000' },
    })
    fireEvent.change(screen.getByLabelText('Cache Read'), {
      target: { value: '200000' },
    })
    fireEvent.change(screen.getByLabelText('Cache Write'), {
      target: { value: '50000' },
    })
    fireEvent.change(screen.getByLabelText('Full input length override'), {
      target: { value: '1300000' },
    })
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Estimated cost: $2')
    )
    expect(post).toHaveBeenLastCalledWith(
      '/api/option/billing-expression/preview',
      {
        items: [
          expect.objectContaining({
            expression,
            sample: expect.objectContaining({
              normalized_usage: expect.objectContaining({
                p: 1000000,
                cr: 200000,
                cc: 50000,
                len: 1300000,
              }),
            }),
          }),
        ],
      }
    )
    expect(onBillingExprChange).not.toHaveBeenCalled()
    expect(onRequestRuleExprChange).not.toHaveBeenCalled()
  })

  it('hides a stale amount immediately and reports a failed request without a local fallback', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue(previewResponse(2))
    render(
      <TieredPricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Estimated cost: $2')
    )
    post.mockRejectedValue(new Error('network unavailable'))
    fireEvent.change(screen.getByLabelText('Input tokens'), {
      target: { value: '1000000' },
    })
    expect(screen.queryByText(/Estimated cost:/)).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true')
    expect(
      await screen.findByText('Preview failed', { exact: false })
    ).toBeVisible()
    expect(screen.queryByText(/Estimated cost:/)).not.toBeInTheDocument()
  })

  it('ignores a late response for an older sample', async () => {
    let resolveOld!: (value: ReturnType<typeof previewResponse>) => void
    const old = new Promise<ReturnType<typeof previewResponse>>((resolve) => {
      resolveOld = resolve
    })
    const post = vi
      .spyOn(api, 'post')
      .mockImplementationOnce(() => old)
      .mockResolvedValue(previewResponse(2))
    render(
      <TieredPricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    await waitFor(() => expect(post).toHaveBeenCalled())
    fireEvent.change(screen.getByLabelText('Input tokens'), {
      target: { value: '1000000' },
    })
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Estimated cost: $2')
    )
    await act(async () => resolveOld(previewResponse(99)))
    expect(screen.getByRole('status')).toHaveTextContent('Estimated cost: $2')
    expect(screen.queryByText(/\$99/)).not.toBeInTheDocument()
  })

  it('treats a response without an evaluation as an error', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: [] },
    })
    render(
      <TieredPricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(
      await screen.findByText('Preview failed', { exact: false })
    ).toBeVisible()
    expect(screen.queryByText(/Estimated cost:/)).not.toBeInTheDocument()
  })

  it('shows an invalid system rate as an error instead of a free price', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: {
        success: true,
        data: [
          {
            key: 'preview',
            error: 'USDExchangeRate option is invalid',
          },
        ],
      },
    })
    render(
      <TieredPricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(
      await screen.findByText(
        /Expression error:.*USDExchangeRate option is invalid/
      )
    ).toBeVisible()
    expect(screen.queryByText(/Estimated cost:/)).not.toBeInTheDocument()
    expect(screen.getByLabelText('Cache Read')).toBeVisible()
  })

  it('displays a server fixed request price without token scaling', async () => {
    const response = previewResponse(0.04)
    response.data.data[0].evaluation.billing_unit = 'request'
    vi.spyOn(api, 'post').mockResolvedValue(response)
    render(
      <TieredPricingEditor
        billingExpr='tier("request", fixed(0.04))'
        requestRuleExpr=''
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(
        'Estimated cost: $0.04/request'
      )
    )
  })

  it.each(['token', 'task'] as const)(
    'uses server USD and rate metadata for %s request simulation',
    async (mode) => {
      const post = vi.spyOn(api, 'post').mockResolvedValue(previewResponse(2))
      const fullExpression =
        mode === 'token'
          ? expression
          : 'tier("task", u("seconds") * 7 / usd_exchange_rate())'
      render(
        <RequestSimulation
          expression={fullExpression}
          mode={mode}
          tokens={{ p: 1000000 }}
          usage={{ seconds: 2 }}
        />
      )
      expect(post).not.toHaveBeenCalled()
      fireEvent.click(
        screen.getByRole('button', { name: 'Request simulation' })
      )
      await waitFor(() =>
        expect(screen.getByRole('status')).toHaveTextContent(
          'Simulated request cost: $2'
        )
      )
      expect(screen.getByRole('status')).toHaveTextContent(
        'Applied exchange rate (CNY/USD): 7'
      )
      expect(post).toHaveBeenCalledWith(
        '/api/option/billing-expression/preview',
        {
          items: [
            expect.objectContaining({
              expression: fullExpression,
              task_usage: mode === 'task',
              sample: expect.objectContaining({ body: {}, headers: {} }),
            }),
          ],
        }
      )
    }
  )
})
