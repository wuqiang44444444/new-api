import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import type { UsageAnalyticsMetrics, UsageAnalyticsPeriod } from '../types'
import { UpstreamUsageReport } from '../upstream-usage-report'

vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
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

it('shows one scoped upstream week with published, partial, empty, and future days', async () => {
  const user = userEvent.setup()
  const onDaySelect = vi.fn()
  const scopedPeriod = {
    ...period,
    days: period.days.map((day, index) => ({
      ...day,
      unpublished: index === 2,
    })),
  }
  render(
    <UpstreamUsageReport
      onDaySelect={onDaySelect}
      showModels={false}
      report={{
        period: scopedPeriod,
        filters: {},
        generated_at: 1,
        data_source: 'published_daily_usage',
        coverage_note: '',
        incomplete_days: ['2026-09-15'],
        result: {
          day_totals: daily(6),
          total: { ...zero, total_calls: 6 },
          channels: [],
          url_groups: [
            {
              url_key: 'https://example.test',
              display_name: 'Example',
              days: daily(6),
              total: { ...zero, total_calls: 6 },
              models: [],
            },
          ],
        },
      }}
    />
  )
  const table = screen.getByRole('table', { name: 'Daily breakdown' })
  expect(screen.getAllByRole('table')).toHaveLength(1)
  expect(
    within(table).getByRole('columnheader', {
      name: /2026-09-15.*Partial data/,
    })
  ).toBeVisible()
  expect(
    within(table).queryByRole('button', { name: '2026-09-16' })
  ).not.toBeInTheDocument()
  expect(within(table).getAllByText('Not published').length).toBeGreaterThan(0)
  expect(within(table).getAllByText('Not started').length).toBeGreaterThan(0)
  const calls = within(table).getByRole('row', { name: /^Calls/ })
  expect(within(calls).getAllByRole('cell')[0]).toHaveTextContent(/^0/)
  expect(within(calls).getAllByRole('cell')[1]).toHaveTextContent(/^6/)
  await user.click(within(table).getByRole('button', { name: '2026-09-15' }))
  expect(onDaySelect).toHaveBeenCalledWith('2026-09-15')
})
