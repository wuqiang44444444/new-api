import { render, screen, within } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { UpstreamViewGroups } from '../admin'
import { CustomerUsageTable } from '../customer-usage-table'
import type { UsageAnalyticsMetrics, UsageAnalyticsPeriod } from '../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
const zero: UsageAnalyticsMetrics = {
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
const period: UsageAnalyticsPeriod = {
  period: 'week',
  date: '2026-09-14',
  start_timestamp: 0,
  end_timestamp: 604800,
  timezone: 'Asia/Shanghai',
  days: Array.from({ length: 7 }, (_, i) => ({
    date: `2026-09-${14 + i}`,
    start: i * 86400,
    end: (i + 1) * 86400,
    future: i === 6,
  })),
}
const daily = (amount: number) =>
  period.days.map((_, i) => ({ ...zero, total_calls: i === 1 ? amount : 0 }))

function expectDailyTable(name: string, calls: number) {
  const table = screen.getByRole('table', { name })
  for (const day of period.days) {
    expect(
      within(table).getByRole('columnheader', { name: day.date })
    ).toBeVisible()
  }
  expect(
    within(table).getByRole('columnheader', { name: 'Period total' })
  ).toBeVisible()
  const row = within(table)
    .getByRole('rowheader', { name: 'Calls' })
    .closest('tr')
  if (!row) throw new Error('Daily calls row is missing')
  const cells = within(row).getAllByRole('cell')
  expect(cells[0]).toHaveTextContent(/^0/)
  expect(cells[1]).toHaveTextContent(new RegExp(`^${calls}`))
  expect(cells[6]).toHaveTextContent('Not started')
}

it('shows each key and customer model across seven dates without opening a toggle', () => {
  render(
    <CustomerUsageTable
      period={period}
      view={{
        day_totals: daily(15),
        total: { ...zero, total_calls: 15 },
        keys: [
          {
            token_id: 11,
            token_name: 'Key A',
            days: daily(15),
            total: { ...zero, total_calls: 15 },
            models: [
              {
                model_name: 'model-a',
                days: daily(8),
                total: { ...zero, total_calls: 8 },
              },
              {
                model_name: 'model-b',
                days: daily(7),
                total: { ...zero, total_calls: 7 },
              },
            ],
          },
        ],
      }}
    />
  )
  expectDailyTable('API Key · Key A', 15)
  expectDailyTable('Customer model · model-a', 8)
  expectDailyTable('Customer model · model-b', 7)
})

it('shows URL, provider model and channel daily values at their own levels', () => {
  render(
    <UpstreamViewGroups
      period={period}
      view={{
        day_totals: daily(10),
        total: { ...zero, total_calls: 10 },
        url_groups: [
          {
            url_key: 'group-a',
            display_name: 'Group A',
            days: daily(10),
            total: { ...zero, total_calls: 10 },
            models: [
              {
                provider_model: 'provider-a',
                billing_mode: 'token',
                days: daily(10),
                total: { ...zero, total_calls: 10 },
                channels: [
                  {
                    channel_id: 1,
                    channel_name: 'Channel A',
                    discounts: [],
                    days: daily(6),
                    total: { ...zero, total_calls: 6 },
                  },
                  {
                    channel_id: 2,
                    channel_name: 'Channel B',
                    discounts: [],
                    days: daily(4),
                    total: { ...zero, total_calls: 4 },
                  },
                ],
              },
            ],
          },
        ],
      }}
    />
  )
  expectDailyTable('Group A', 10)
  expectDailyTable('Provider model · provider-a', 10)
  expectDailyTable('Channel · Channel A', 6)
  expectDailyTable('Channel · Channel B', 4)
})
