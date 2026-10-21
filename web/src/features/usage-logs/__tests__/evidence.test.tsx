import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { TaskEvidence } from '../components/task-evidence'
import { getEvidenceList } from '../evidence-api'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
beforeEach(() =>
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'root', role: ROLE.SUPER_ADMIN })
)
afterEach(() => useAuthStore.getState().auth.reset())
function show(isRoot = false) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <TaskEvidence taskId='task-1' isRoot={isRoot} />
    </QueryClientProvider>
  )
}
describe('Task evidence', () => {
  it('loads only when opened and displays the empty state', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: { items: [], total: 0 } },
    })
    show()
    expect(api.get).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    expect(
      await screen.findByText(
        'No request evidence available. It may not have been recorded or may have expired.'
      )
    ).toBeTruthy()
  })
  it('shows an error and a retry action when the query fails', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('offline'))
    show()
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
  it('shows a database query failure instead of no records and allows retry', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: { success: false, message: 'Failed to load request evidence' },
      })
      .mockResolvedValueOnce({
        data: { success: true, data: { items: [], total: 0 } },
      })
    show()
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load request evidence'
    )
    expect(
      screen.queryByText(
        'No request evidence available. It may not have been recorded or may have expired.'
      )
    ).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByText(
        'No request evidence available. It may not have been recorded or may have expired.'
      )
    ).toBeVisible()
    expect(screen.queryByRole('alert')).toBeNull()
  })
  it('distinguishes expired bodies from missing records and prevents original downloads', async () => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ id: 1, request_id: 'expired-request' }], total: 1 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            evidence: { body_expired: true },
            events: [
              {
                id: 2,
                name: 'polling',
                phase: 'completed',
                complete: true,
                has_body: true,
                body_status: 'expired',
                preview: '',
                byte_count: 8,
                status_code: 200,
              },
            ],
          },
        },
      })
    show(true)
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'expired-request' })
    )
    expect(
      (await screen.findAllByText('Evidence body expired')).length
    ).toBeGreaterThan(0)
    expect(
      screen.queryByText(
        'No request evidence available. It may not have been recorded or may have expired.'
      )
    ).toBeNull()
    expect(
      screen.queryByRole('button', { name: 'Download original evidence' })
    ).toBeNull()
  })
  it('offers the original view to Root only after an explicit click and clears it on close', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const original = '{"prompt":"secret business body","url":"https://x?sig=1"}'
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ id: 1, request_id: 'req-view' }], total: 1 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            evidence: { body_expired: false },
            events: [
              {
                id: 2,
                name: 'north_receive',
                phase: 'completed',
                complete: true,
                has_body: true,
                body_status: 'available',
                preview: '{"prompt":"signed-url-masked"}',
                byte_count: 10,
                status_code: 200,
              },
            ],
          },
        },
      })
      .mockResolvedValue({
        data: {
          success: true,
          data: { text: original, body_status: 'available' },
        },
      })
    render(
      <QueryClientProvider client={client}>
        <TaskEvidence taskId='task-1' isRoot />
      </QueryClientProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    fireEvent.click(await screen.findByRole('button', { name: 'req-view' }))
    // The default preview stays masked; the original is not in the document.
    await screen.findByRole('button', { name: 'View original text' })
    expect(screen.queryByText('{"prompt":"signed-url-masked"}')).toBeNull()
    expect(screen.queryByText(original)).toBeNull()
    fireEvent.click(
      await screen.findByRole('button', { name: 'View original text' })
    )
    expect(await screen.findByText(original)).toBeTruthy()
    expect(screen.getByRole('dialog')).toHaveAccessibleDescription(
      'Captured content may be incomplete and may contain credentials.'
    )
    expect(api.get).toHaveBeenLastCalledWith(
      '/api/task_request_evidence/1/events/2/content',
      expect.objectContaining({ skipErrorHandler: true })
    )
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(screen.queryByText(original)).toBeNull())
    await waitFor(() => {
      const cachedBodies = client
        .getQueriesData({ queryKey: ['evidence-original'] })
        .map(([, data]) => data)
        .filter((data) => data !== undefined)
      expect(cachedBodies).toEqual([])
    })
  })
  it('discards a late original response after the dialog closed', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    let resolveInFlight: (value: { data: string }) => void = () => {}
    const inFlight = new Promise<{ data: string }>((resolve) => {
      resolveInFlight = resolve
    })
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ id: 1, request_id: 'req-late' }], total: 1 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            evidence: { body_expired: false },
            events: [
              {
                id: 3,
                name: 'southbound_send',
                phase: 'completed',
                complete: true,
                has_body: true,
                body_status: 'available',
                preview: 'masked preview',
                byte_count: 10,
                status_code: 200,
              },
            ],
          },
        },
      })
      .mockReturnValueOnce(inFlight)
    render(
      <QueryClientProvider client={client}>
        <TaskEvidence taskId='task-1' isRoot />
      </QueryClientProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
    fireEvent.click(await screen.findByRole('button', { name: 'req-late' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'View original text' })
    )
    expect(await screen.findByRole('status')).toBeTruthy()
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Close' }))
    resolveInFlight({ data: 'late unmasked original' })
    await waitFor(() => {
      const cachedBodies = client
        .getQueriesData({ queryKey: ['evidence-original'] })
        .map(([, data]) => data)
        .filter((data) => data !== undefined)
      expect(cachedBodies).toEqual([])
    })
    expect(screen.queryByText('late unmasked original')).toBeNull()
  })
})

it('queries synchronous audio evidence by platform request ID without a task ID', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { items: [], total: 0 } },
  })
  await getEvidenceList({ request_id: 'audio-request' }, 1)
  expect(api.get).toHaveBeenCalledWith('/api/task_request_evidence', {
    params: { request_id: 'audio-request', p: 1, page_size: 20 },
  })
})
