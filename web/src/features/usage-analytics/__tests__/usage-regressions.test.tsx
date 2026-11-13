import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { UsageNonTokenCell } from '../metrics-cells'
import { UsagePeriodPicker } from '../period-picker'
import type { UsageAnalyticsMetrics } from '../types'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string | number>) =>
      key.replaceAll(/{{(\w+)}}/g, (match, name) =>
        String(values?.[name] ?? match)
      ),
  }),
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
  render(<UsageNonTokenCell metrics={{ ...metrics, seconds: '0' }} />)
  expect(screen.getByText(/billing seconds/i)).toHaveTextContent('0')
})
it('shows missing seconds independently from token usage', () => {
  render(
    <UsageNonTokenCell metrics={{ ...metrics, seconds_missing_rows: 1 }} />
  )
  expect(screen.getByText(/Seconds unrecorded/)).toBeInTheDocument()
})
it('splits unit-known seconds gaps from generic missing seconds', () => {
  render(
    <UsageNonTokenCell
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
it('shows an explicitly recorded zero image count and marks partial seconds separately', () => {
  render(
    <UsageNonTokenCell
      metrics={{
        ...metrics,
        image_count_known: true,
        seconds: '29.25',
        seconds_missing_rows: 2,
      }}
    />
  )
  expect(screen.getByText('0 images')).toBeInTheDocument()
  expect(screen.getByText('29.25 billing seconds')).toBeInTheDocument()
  expect(screen.queryByText('Known subtotal')).not.toBeInTheDocument()
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

it('moves between calendar months without overflowing a month-end date', async () => {
  const onDateChange = vi.fn()
  render(
    <UsagePeriodPicker
      period='month'
      date='2024-01-31'
      resolved={undefined}
      onPeriodChange={vi.fn()}
      onMonthSelect={vi.fn()}
      onDateChange={onDateChange}
    />
  )
  expect(screen.getByRole('button', { name: 'Monthly' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Next period' }))
  expect(onDateChange).toHaveBeenLastCalledWith('2024-02-01')
  await userEvent.click(screen.getByRole('button', { name: 'Previous period' }))
  expect(onDateChange).toHaveBeenLastCalledWith('2023-12-01')
})

it('separates customer and test pricing quantities from unrecorded delivery', () => {
  render(
    <UsageNonTokenCell
      metrics={{
        ...metrics,
        billable_images: 561,
        test_billable_images: 6,
        billable_image_missing_rows: 1,
        image_count_missing_rows: 553,
      }}
    />
  )
  expect(screen.getByText('Billable image count · 561')).toBeVisible()
  expect(screen.getByText('Channel test billable images · 6')).toBeVisible()
  expect(screen.getByText('Image count unrecorded')).toBeVisible()
  expect(screen.getByText('Known subtotal')).toBeVisible()
})
