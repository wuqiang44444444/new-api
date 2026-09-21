import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { AdminUsage } from '../admin'
import {
  getAdminUsageCustomerSummary,
  getAdminUsageUpstreamSummary,
  getUsageSelfSummary,
} from '../api'

const get = vi.hoisted(() => vi.fn())
vi.mock('@/lib/api', () => ({ api: { get } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}))

const period = {
  period: 'day',
  date: '2026-09-21',
  timezone: 'Asia/Shanghai',
  start_timestamp: 1789920000,
  end_timestamp: 1790006400,
  days: [{ date: '2026-09-21', start: 1789920000, end: 1790006400 }],
}
const total = {
  total_calls: 0,
  success_calls: 0,
  failure_calls: 0,
  cancelled_calls: 0,
  other_result_calls: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  image_count: 0,
  gross_quota: 0,
  refund_quota: 0,
  net_quota: 0,
}
function reply(result: object) {
  return {
    data: {
      success: true,
      message: '',
      data: { period, result: { total, day_totals: [total], ...result } },
    },
  }
}

it('opens customer usage when the server returns a null customer slice', async () => {
  get.mockResolvedValue(reply({ customers: null }))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AdminUsage period='day' date='2026-09-21' onSearchChange={vi.fn()} />
    </QueryClientProvider>
  )
  expect(await screen.findByText('No usage records')).toBeInTheDocument()
  expect(
    screen.getByRole('columnheader', { name: 'Customer' })
  ).toBeInTheDocument()
  expect(screen.getByText('Total', { exact: true })).toBeInTheDocument()
  client.clear()
})

it('normalizes nullable key and URL slices at the API boundary', async () => {
  get.mockResolvedValue(reply({ keys: null }))
  expect(
    (await getUsageSelfSummary({ period: 'day', date: '2026-09-21' })).data
      .result.keys
  ).toEqual([])
  expect(
    (
      await getAdminUsageCustomerSummary({
        period: 'day',
        date: '2026-09-21',
        user_id: 7,
      })
    ).data.result.keys
  ).toEqual([])
  get.mockResolvedValue(reply({ url_groups: null }))
  expect(
    (await getAdminUsageUpstreamSummary({ period: 'day', date: '2026-09-21' }))
      .data.result.url_groups
  ).toEqual([])
})
