import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { BatchBillingDetails } from '../batch-billing-details'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
it('shows per-request usage and loads the next billing page', async () => {
  get.mockResolvedValue({
    data: {
      success: true,
      data: {
        status: 'completed',
        settle_state: 'settled',
        delivery_state: 'ready',
        request_count: 101,
        target_quota: 20,
        has_more: true,
        lines: [
          {
            custom_id: 'request-a',
            status: 'completed',
            input_tokens: 10,
            output_tokens: 5,
            cached_tokens: 0,
            quota: 20,
          },
        ],
      },
    },
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <BatchBillingDetails id='batch-one' />
    </QueryClientProvider>
  )
  expect(await screen.findByText('request-a')).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Next' }))
  await vi.waitFor(() =>
    expect(get).toHaveBeenCalledWith('/api/batch/batch-one/billing', {
      params: { offset: 100 },
    })
  )
})

it('keeps an uncharged request visible without presenting zero as measured usage', async () => {
  get.mockResolvedValue({
    data: {
      success: true,
      data: {
        status: 'cancelled',
        settle_state: 'settled',
        delivery_state: 'ready',
        request_count: 1,
        estimated_quota: 100,
        target_quota: 0,
        has_more: false,
        lines: [
          {
            custom_id: 'unexecuted',
            status: 'not_charged',
            usage_unavailable: true,
            input_tokens: 0,
            output_tokens: 0,
            cached_tokens: 0,
            quota: 0,
          },
        ],
      },
    },
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <BatchBillingDetails id='batch-cancelled' />
    </QueryClientProvider>
  )
  const row = await screen.findByRole('row', { name: /unexecuted/ })
  expect(within(row).getByText('No charge')).toBeVisible()
  expect(within(row).getAllByText('—')).toHaveLength(3)
  expect(within(row).getByText('0')).toBeVisible()
  expect(
    within(row).getByRole('button', { name: 'Charge calculation for {{id}}' })
  ).toBeEnabled()
})
