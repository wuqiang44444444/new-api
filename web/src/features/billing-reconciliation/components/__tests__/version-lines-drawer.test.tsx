import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import type { BillingStatementVersionInfo } from '../../version-api'
import { VersionLinesDrawer } from '../version-lines-drawer'

const calls = vi.hoisted(() => ({ lines: vi.fn() }))
vi.mock('../../version-api', () => ({
  getSelfVersionLines: calls.lines,
  getAdminVersionLines: calls.lines,
}))

it('shows a frozen net amount before loading the individual money events', async () => {
  const version: BillingStatementVersionInfo = {
    id: 1,
    draft_public_id: 'frozen-1',
    status: 'confirmed',
    version_number: 1,
    user_id: 7,
    period_start: 1788192000,
    period_end_exclusive: 1790784000,
    confirmed_at: 1790784000,
    created_at: 1790784000,
    updated_at: 1790784000,
    public_reason: '',
    corrects_version_id: null,
    quota_per_unit: 1,
    currency: 'USD',
    currency_rate: 1,
  }
  calls.lines.mockImplementation(async (_id, params) => ({
    success: true,
    data: params.group_funds
      ? {
          total: 1,
          fund_groups: [
            {
              key: 'frozen-group',
              date: '2026-09-01',
              model_name: 'Video model',
              event_count: 2,
              net_quota: 70,
            },
          ],
        }
      : {
          total: 2,
          lines: [2, 6].map((type) => ({
            id: type,
            log_type: type,
            quota: type === 2 ? 100 : 30,
            created_at: 1788192001,
            customer_model: 'Video model',
            token_id: 1,
            facts: {},
          })),
        },
  }))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <VersionLinesDrawer
        version={version}
        open
        onOpenChange={vi.fn()}
        isAdmin={false}
      />
    </QueryClientProvider>
  )

  expect(await screen.findByText(/\$70/)).toBeInTheDocument()
  expect(screen.queryByText('Refund')).not.toBeInTheDocument()
  expect(calls.lines).toHaveBeenCalledTimes(1)
  screen.getByRole('button', { name: /View details/ }).focus()
  await userEvent.keyboard('{Enter}')
  expect(await screen.findByText('Refund')).toBeInTheDocument()
  expect(calls.lines).toHaveBeenLastCalledWith(
    version.draft_public_id,
    expect.objectContaining({ fund_group: 'frozen-group' }),
    expect.any(AbortSignal)
  )
})
