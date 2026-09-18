import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { AxiosError, AxiosHeaders } from 'axios'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { TaskRequestDetails } from '../components/task-request-details'
import { getTaskRequestBodies } from '../evidence-api'

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

function recordedEvents(events: Record<string, unknown>[]) {
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
          events: events.map((event) => ({
            stage: 'north_receive',
            has_body: true,
            complete: true,
            body_status: 'available',
            ...event,
          })),
        },
      },
    })
}

function objectReadError(status: number, data: string) {
  const config = { headers: new AxiosHeaders() }
  return new AxiosError(
    'Object request failed',
    'ERR_BAD_RESPONSE',
    config,
    undefined,
    { data, status, statusText: 'Failed', headers: {}, config }
  )
}

it('keeps readable records when other bodies cannot be decrypted, are missing or are binary', async () => {
  recordedEvents([
    { id: 11, body_status: 'decrypt_failed' },
    { id: 12 },
    { id: 13, body_status: 'missing' },
    { id: 14, body_status: 'binary' },
    { id: 15 },
  ])
  vi.mocked(api.get)
    .mockResolvedValueOnce({ data: 'readable original' })
    .mockResolvedValueOnce({ data: '' })
  show()
  fireEvent.click(screen.getByRole('button', { name: 'User request details' }))
  expect(await screen.findByText('readable original')).toBeTruthy()
  expect(
    screen.getByText('Evidence body cannot be decrypted or authenticated')
  ).toBeTruthy()
  expect(screen.getByText('Evidence body file is missing')).toBeTruthy()
  expect(screen.getByText('Binary evidence has no text preview')).toBeTruthy()
  expect(screen.queryByRole('alert')).toBeNull()
  expect(document.querySelectorAll('pre')).toHaveLength(2)
  expect(api.get).toHaveBeenCalledTimes(4)
})

it.each([
  [410, '{"body_status":"missing"}', 'Evidence body file is missing'],
  [
    503,
    '{"body_status":"storage_unavailable"}',
    'Evidence storage is unavailable',
  ],
  [503, '<html>gateway unavailable</html>', 'Evidence body could not be read'],
])(
  'isolates HTTP %s failures between preview and download',
  async (status, data, message) => {
    recordedEvents([{ id: 11 }, { id: 12 }])
    vi.mocked(api.get)
      .mockRejectedValueOnce(objectReadError(status, data))
      .mockResolvedValueOnce({ data: 'second original' })
    show()
    fireEvent.click(
      screen.getByRole('button', { name: 'User request details' })
    )
    expect(await screen.findByText('second original')).toBeTruthy()
    expect(screen.getByText(message)).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByText(data)).toBeNull()
  }
)

it('allows a transient per-object failure to be retried without discarding readable records', async () => {
  recordedEvents([{ id: 11 }, { id: 12 }])
  vi.mocked(api.get)
    .mockRejectedValueOnce(new AxiosError('offline', 'ERR_NETWORK'))
    .mockResolvedValueOnce({ data: 'second original' })
  show()
  fireEvent.click(screen.getByRole('button', { name: 'User request details' }))
  expect(await screen.findByText('second original')).toBeTruthy()
  expect(screen.getByText('Evidence body could not be read')).toBeTruthy()
  recordedEvents([{ id: 11 }, { id: 12 }])
  vi.mocked(api.get)
    .mockResolvedValueOnce({ data: 'recovered original' })
    .mockResolvedValueOnce({ data: 'second original' })
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('recovered original')).toBeTruthy()
  expect(screen.queryByText('Evidence body could not be read')).toBeNull()
})

it.each([401, 403])(
  'keeps HTTP %s as an authentication failure',
  async (status) => {
    recordedEvents([{ id: 11 }, { id: 12 }])
    const error = objectReadError(status, '{"message":"Access denied"}')
    vi.mocked(api.get).mockRejectedValueOnce(error)
    await expect(getTaskRequestBodies('task-1', 'north_receive')).rejects.toBe(
      error
    )
    expect(api.get).toHaveBeenCalledTimes(3)
  }
)
