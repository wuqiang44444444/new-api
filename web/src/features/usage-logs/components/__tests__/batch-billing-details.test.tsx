import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
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
