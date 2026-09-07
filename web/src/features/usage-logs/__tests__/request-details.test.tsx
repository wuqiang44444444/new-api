import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { TaskRequestDetails } from '../components/task-request-details'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
function show() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <TaskRequestDetails taskId='task-1' />
    </QueryClientProvider>
  )
}
describe('Request detail shortcuts', () => {
  it.each([
    ['User request details', 11],
    ['Transformed upstream request', 12],
  ] as const)(
    'opens the recorded text for %s without using the preview',
    async (label, eventId) => {
      const original = '{ "prompt": "<script>text only</script>" }'
      vi.mocked(api.get)
        .mockResolvedValueOnce({
          data: {
            success: true,
            data: { items: [{ id: 1, request_id: 'request-1' }], total: 1 },
          },
        })
        .mockResolvedValueOnce({
          data: {
            success: true,
            data: {
              evidence: { body_expired: false },
              events: [
                {
                  id: 11,
                  stage: 'north_receive',
                  has_body: true,
                  complete: true,
                  preview: 'truncated',
                },
                {
                  id: 12,
                  stage: 'southbound_send',
                  has_body: true,
                  complete: true,
                  preview: 'truncated',
                },
              ],
            },
          },
        })
        .mockResolvedValueOnce({ data: original })
      show()
      expect(api.get).not.toHaveBeenCalled()
      fireEvent.click(screen.getByRole('button', { name: label }))
      expect(await screen.findByText(original)).toBeTruthy()
      expect(api.get).toHaveBeenLastCalledWith(
        `/api/task_request_evidence/1/events/${eventId}/object`,
        expect.objectContaining({ responseType: 'text' })
      )
      expect(screen.queryByText('truncated')).toBeNull()
      expect(document.querySelector('script')).toBeNull()
    }
  )
  it('shows expired bodies without downloading them', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ id: 1, request_id: 'request-1' }], total: 1 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            evidence: { body_expired: true },
            events: [
              {
                id: 11,
                stage: 'north_receive',
                has_body: true,
                complete: true,
              },
            ],
          },
        },
      })
    show()
    fireEvent.click(
      screen.getByRole('button', { name: 'User request details' })
    )
    expect(await screen.findByText('Evidence body expired')).toBeTruthy()
    expect(api.get).toHaveBeenCalledTimes(2)
  })
  it('allows retry after a request fails', async () => {
    vi.mocked(api.get)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({
        data: { success: true, data: { items: [], total: 0 } },
      })
    show()
    fireEvent.click(
      screen.getByRole('button', { name: 'User request details' })
    )
    expect(await screen.findByRole('alert')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No request evidence recorded')).toBeTruthy()
  })
})
