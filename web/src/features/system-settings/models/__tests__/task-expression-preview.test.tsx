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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { previewBillingExpressions } from '@/features/pricing/lib/billing-display-preview'

import { TaskExpressionPreview } from '../task-expression-preview'
import { TaskUsagePricingEditor } from '../task-usage-pricing-editor'

vi.mock('@/features/pricing/lib/billing-display-preview', () => ({
  previewBillingExpressions: vi.fn(),
}))

const schema = {
  tokens: { type: 'number' as const, unit: 'token' as const },
  video: { type: 'boolean' as const },
}
const expression =
  '(u("video") ? tier("video",u("tokens")*5) : tier("plain",u("tokens")*8))/1000000'
const response = {
  key: 'task-preview',
  evaluation: {
    raw_cost_usd: 5,
    quota: 2500000,
    matched_tier: 'video',
    pricing_time: '',
    usage_semantic: 'task',
    normalized_usage: {},
  },
}

beforeEach(() => vi.clearAllMocks())

describe('custom task expression preview', () => {
  it('uses the server engine for migrated expressions and preserves boolean samples', async () => {
    vi.mocked(previewBillingExpressions).mockResolvedValue([response])
    render(
      <TaskUsagePricingEditor
        billingExpr={expression}
        requestRuleExpr=''
        usageSchema={schema}
        onBillingExprChange={vi.fn()}
        onRequestRuleExprChange={vi.fn()}
      />
    )
    expect(
      screen.queryByText('Preview is unavailable for custom expressions.')
    ).not.toBeInTheDocument()
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Usage parameters' }),
      { target: { value: '{"tokens":1000000,"video":true}' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Calculate' }))
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('$5')
    )
    expect(previewBillingExpressions).toHaveBeenCalledWith([
      expect.objectContaining({
        expression,
        task_usage: true,
        sample: expect.objectContaining({
          usage: { tokens: 1000000, video: true },
        }),
      }),
    ])
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Usage parameters' }),
      { target: { value: '{"tokens":0,"video":true}' } }
    )
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('never reinterprets legacy token prices without an explicit choice', async () => {
    vi.mocked(previewBillingExpressions)
      .mockResolvedValueOnce([
        {
          key: 'task-preview',
          error: 'Token variables cannot be evaluated as task USD pricing.',
        },
      ])
      .mockResolvedValueOnce([response])
    render(
      <TaskExpressionPreview expression='tier("base", c*5)' schema={schema} />
    )
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Usage parameters' }),
      { target: { value: '{"tokens":1000000,"video":false}' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Calculate' }))
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Preview as legacy token pricing',
      })
    )
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(
        'Pricing settings are not changed.'
      )
    )
    expect(
      vi.mocked(previewBillingExpressions).mock.calls[1][0][0]
    ).toMatchObject({
      task_usage: false,
      sample: { completion_tokens: 1000000 },
    })
  })

  it('rejects invalid sample shapes before sending a request', async () => {
    render(<TaskExpressionPreview expression={expression} schema={schema} />)
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Usage parameters' }),
      { target: { value: '[]' } }
    )
    fireEvent.click(screen.getByRole('button', { name: 'Calculate' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Task usage samples must contain numbers, strings or booleans.'
    )
    expect(previewBillingExpressions).not.toHaveBeenCalled()
  })
})
