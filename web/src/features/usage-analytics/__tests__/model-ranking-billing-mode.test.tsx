import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { UsageReportRankItem } from '../types'
import {
  AdminCustomerModelRanking,
  PublishedUsageRanking,
} from '../usage-rankings'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

it('shows the token breakdown and missing records within their applicable billing rows', () => {
  render(
    <AdminCustomerModelRanking
      rows={[
        {
          name: 'mixed-image',
          billing_mode: 'token',
          calls: 2,
          tokens: 1250,
          missing_tokens: 2,
          token_usage: {
            input_tokens: 750,
            output_tokens: 500,
            cache_read_tokens: 100,
            cache_write_tokens: 0,
            token_details: { image_output: 300 },
          },
          amount: 50,
          pending_amount: 0,
        },
        {
          name: 'mixed-image',
          billing_mode: 'per_image',
          calls: 1,
          tokens: 987,
          missing_tokens: 0,
          amount: 200,
          pending_amount: 0,
          non_token: { image_count: 3, image_count_known: true },
        },
        {
          name: 'mixed-image',
          calls: 1,
          tokens: 42,
          missing_tokens: 1,
          amount: 0,
          pending_amount: 0,
        },
      ]}
    />
  )

  const table = screen.getByRole('table', { name: 'Model ranking' })
  const token = within(table).getByRole('row', { name: /Token billing/ })
  expect(within(token).getByText('1,250')).toBeVisible()
  expect(within(token).queryByText('Known subtotal')).not.toBeInTheDocument()
  expect(within(token).getByText('Input 750 · Output 500')).toBeVisible()
  expect(
    within(token).getByText(/Cache read 100 · Cache write 0/)
  ).toBeVisible()
  expect(within(token).getByText(/2 rows unrecorded/)).toBeVisible()
  expect(within(token).getByText('Image output tokens 300')).toBeVisible()
  expect(within(token).getByText('Not applicable')).toBeInTheDocument()
  const image = within(table).getByRole('row', { name: /Per image/ })
  expect(within(image).getByText('3 images')).toBeVisible()
  expect(within(image).queryByText('987')).not.toBeInTheDocument()
  const unknown = within(table).getByRole('row', {
    name: /Unknown billing mode/,
  })
  expect(within(unknown).queryByText('42')).not.toBeInTheDocument()
  expect(within(unknown).getByText('1 rows unrecorded')).toBeVisible()
})

it.each(['single customer', 'all customers'] as const)(
  'shows failed calls next to total calls for each billing mode in %s',
  async (view) => {
    const common = {
      name: 'mixed-image',
      calls: 776,
      tokens: 0,
      missing_tokens: 0,
      amount: 0,
      pending_amount: 0,
    }
    const rows: UsageReportRankItem[] = [
      { ...common, billing_mode: 'token', failure_calls: 0 },
      { ...common, billing_mode: 'unknown', failure_calls: 776 },
    ]
    if (view === 'single customer') {
      render(<AdminCustomerModelRanking rows={rows} />)
    } else {
      vi.mocked(api.get).mockResolvedValueOnce({
        data: { success: true, data: { items: rows, total: rows.length } },
      })
      render(
        <QueryClientProvider client={new QueryClient()}>
          <PublishedUsageRanking
            kind='model'
            params={{ period: 'month', date: '2026-11-12' }}
          />
        </QueryClientProvider>
      )
    }

    const table = within(
      await screen.findByRole('table', { name: 'Model ranking' })
    )
    const headers = table.getAllByRole('columnheader')
    const calls = headers.indexOf(
      table.getByRole('columnheader', { name: 'Calls' })
    )
    const failed = headers.indexOf(
      table.getByRole('columnheader', { name: 'Failed calls' })
    )
    expect(failed).toBe(calls + 1)
    const token = within(table.getByRole('row', { name: /Token billing/ }))
    expect(token.getAllByRole('cell')[failed]).toHaveTextContent(/^0$/)
    const unknown = within(
      table.getByRole('row', { name: /Unknown billing mode/ })
    )
    expect(unknown.getAllByRole('cell')[failed]).toHaveTextContent(/^776$/)
  }
)
