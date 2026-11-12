import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'

import type { UsageAnalyticsMetrics, UsageUpstreamModelRow } from '../types'
import { UpstreamUsageReport } from '../upstream-usage-report'

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

function model(
  name: string,
  metrics: Partial<UsageAnalyticsMetrics>
): UsageUpstreamModelRow {
  return {
    provider_model: name,
    billing_mode: 'token',
    total: { ...zero, ...metrics },
    days: [],
    channels: [],
  }
}

it('shows model-share charts and expands independently metered usage across upstream groups', async () => {
  const user = userEvent.setup()
  const total = {
    ...zero,
    total_calls: 12,
    input_tokens: 90,
    output_tokens: 30,
  }
  render(
    <UpstreamUsageReport
      onDaySelect={() => {}}
      showModels
      report={{
        period: {
          period: 'day',
          date: '2026-11-11',
          start_timestamp: 0,
          end_timestamp: 86400,
          timezone: 'Asia/Shanghai',
          days: [{ date: '2026-11-11', start: 0, end: 86400 }],
        },
        result: {
          total,
          day_totals: [total],
          channels: [],
          url_groups: [
            {
              url_key: 'https://a.example.test',
              display_name: 'A',
              days: [],
              total,
              models: [
                model('Model A', {
                  total_calls: 4,
                  input_tokens: 30,
                  output_tokens: 10,
                  reference_amount: 500000,
                  image_count: 2,
                  seconds: '0.1',
                }),
                model('Model B', {
                  total_calls: 3,
                  input_tokens: 60,
                  output_tokens: 20,
                  reference_amount: 1000000,
                }),
              ],
            },
            {
              url_key: 'https://b.example.test',
              display_name: 'B',
              days: [],
              total,
              models: [
                model('Model A', {
                  total_calls: 5,
                  rows_missing_tokens: 1,
                  known_reference_amount: 500000,
                  seconds: '0.2',
                  seconds_missing_rows: 1,
                  reference_pending: true,
                }),
                model('Model C', { reference_pending: true }),
              ],
            },
          ],
        },
        filters: {},
        generated_at: 1,
        data_source: 'published_daily_usage',
        coverage_note: '',
      }}
    />
  )

  const charts = within(
    screen.getByRole('region', { name: 'Upstream model breakdown' })
  )
  const calls = within(charts.getByRole('group', { name: 'Calls' }))
  const tokens = within(charts.getByRole('group', { name: 'Total tokens' }))
  const amount = within(charts.getByRole('group', { name: 'Reference amount' }))
  expect(calls.getByText('Model A').closest('li')).toHaveTextContent(/9\s*75%/)
  expect(tokens.getByText('Model A').closest('li')).toHaveTextContent(
    /40\s*33.33%/
  )
  expect(tokens.getByText('Known subtotal')).toBeVisible()
  expect(amount.getByText('Model A').closest('li')).toHaveTextContent(/50%/)
  expect(amount.getByText('Model B').closest('li')).toHaveTextContent(/50%/)
  expect(amount.getByText('Known subtotal')).toBeVisible()
  expect(amount.queryByText('Model C')).not.toBeInTheDocument()
  expect(screen.queryByText('Reference amount details')).not.toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  const details = screen.getByRole('button', { name: 'Model usage details' })
  expect(details).toHaveAttribute('aria-expanded', 'false')
  await user.click(details)
  expect(details).toHaveAttribute('aria-expanded', 'true')
  const table = screen.getByRole('table', { name: 'Model usage details' })
  expect(
    within(table)
      .getAllByRole('columnheader')
      .map((header) => header.textContent)
  ).toEqual([
    'Model',
    'Calls',
    'Total tokens',
    'Non-Token usage',
    'Reference amount',
  ])
  const row = within(table).getByRole('row', { name: /Model A/ })
  expect(within(row).getByText('2 images')).toBeVisible()
  const seconds = within(row).getByText('0.3 billing seconds')
  expect(seconds).toBeVisible()
  expect(seconds.parentElement).toHaveTextContent('Known subtotal')
})
