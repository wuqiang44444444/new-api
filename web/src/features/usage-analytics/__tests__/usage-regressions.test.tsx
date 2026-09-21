import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { CustomerUsageTable, DayTotalsTable } from '../customer-usage-table'
import { UsagePeriodPicker } from '../period-picker'
import { UsageTokensCell } from '../metrics-cells'
import type { UsageAnalyticsMetrics } from '../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
const metrics: UsageAnalyticsMetrics = {
  total_calls: 2,
  success_calls: 1,
  failure_calls: 1,
  cancelled_calls: 0,
  other_result_calls: 0,
  input_tokens: 10,
  output_tokens: 20,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  image_count: 0,
  gross_quota: 100,
  refund_quota: 20,
  net_quota: 80,
}
it('shows measured zero seconds without requiring any image count', () => {
  render(<UsageTokensCell metrics={{ ...metrics, seconds: '0' }} />)
  expect(screen.getByText(/Seconds/)).toHaveTextContent('0')
})
it('shows missing seconds independently from token usage', () => {
  render(<UsageTokensCell metrics={{ ...metrics, seconds_missing_rows: 1 }} />)
  expect(screen.getByText(/Seconds unrecorded/)).toBeInTheDocument()
})
it('splits unit-known seconds gaps from generic missing seconds', () => {
  render(
    <UsageTokensCell
      metrics={{
        ...metrics,
        seconds_missing_rows: 3,
        seconds_value_missing_rows: 1,
      }}
    />
  )
  // The i18n mock returns raw keys: assert presence and the count split logic
  // through which keys render at all.
  expect(screen.getByText(/Seconds unrecorded/)).toBeInTheDocument()
  expect(
    screen.getByText(/Second unit known; measured duration unrecorded/)
  ).toBeInTheDocument()
})
it('daily breakdown includes result classes, output usage and refund, and marks future dates', () => {
  render(
    <DayTotalsTable
      dates={['2026-09-20', '2026-09-21']}
      futureFlags={[false, true]}
      days={[metrics, metrics]}
      total={metrics}
    />
  )
  expect(screen.getAllByText(/Failed/).length).toBeGreaterThan(0)
  expect(screen.getAllByText(/Output/).length).toBeGreaterThan(0)
  expect(screen.getAllByText(/Refund/).length).toBeGreaterThan(0)
  expect(screen.getAllByText('Not started').length).toBeGreaterThan(0)
})

it('day mode hides the duplicate period-total column, week mode keeps it', () => {
  render(
    <DayTotalsTable
      dates={['2026-09-21']}
      futureFlags={[false]}
      days={[metrics]}
      total={metrics}
    />
  )
  expect(screen.queryByText('Period total')).not.toBeInTheDocument()
  cleanup()
  render(
    <DayTotalsTable
      dates={['2026-09-15', '2026-09-16']}
      futureFlags={[false, false]}
      days={[metrics, metrics]}
      total={metrics}
    />
  )
  expect(screen.getByText('Period total')).toBeInTheDocument()
})

it('period navigation uses dedicated period labels and moves by the period length', async () => {
  const onDateChange = vi.fn()
  render(
    <UsagePeriodPicker
      period='day'
      date='2026-09-21'
      resolved={undefined}
      onPeriodChange={vi.fn()}
      onDateChange={onDateChange}
    />
  )
  await userEvent.click(screen.getByRole('button', { name: 'Previous period' }))
  expect(onDateChange).toHaveBeenLastCalledWith('2026-09-20')
  await userEvent.click(screen.getByRole('button', { name: 'Next period' }))
  expect(onDateChange).toHaveBeenLastCalledWith('2026-09-22')
  cleanup()
  render(
    <UsagePeriodPicker
      period='week'
      date='2026-09-16'
      resolved={undefined}
      onPeriodChange={vi.fn()}
      onDateChange={onDateChange}
    />
  )
  await userEvent.click(screen.getByRole('button', { name: 'Previous period' }))
  expect(onDateChange).toHaveBeenLastCalledWith('2026-09-09')
  await userEvent.click(screen.getByRole('button', { name: 'Next period' }))
  expect(onDateChange).toHaveBeenLastCalledWith('2026-09-23')
})

it('customer rows show period metrics directly without a daily-breakdown toggle', () => {
  const view = {
    day_totals: [metrics],
    total: metrics,
    keys: [
      {
        token_id: 0,
        token_name: 'unknown',
        days: [metrics],
        total: metrics,
        models: [
          { model_name: 'unknown', days: [metrics], total: metrics },
        ],
      },
    ],
  }
  render(
    <CustomerUsageTable
      view={view}
      period={{
        period: 'day',
        date: '2026-09-21',
        start_timestamp: 0,
        end_timestamp: 86400,
        timezone: 'Asia/Shanghai',
        days: [{ date: '2026-09-21', start: 0, end: 86400 }],
      }}
    />
  )
  expect(screen.queryByText('Daily breakdown')).not.toBeInTheDocument()
  expect(screen.getByText('Total', { exact: true })).toBeInTheDocument()
})
