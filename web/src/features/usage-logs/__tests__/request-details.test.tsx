import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { AxiosError } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { TaskRequestDetails } from '../components/task-request-details'
import { readEvidenceOriginalText } from '../evidence-api'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'admin', role: ROLE.ADMIN })
})
afterEach(() => useAuthStore.getState().auth.reset())
it.each([
  ['User request details', 'north_receive', 11],
  ['Transformed upstream request', 'southbound_send', 12],
] as const)(
  'lists the locations for %s and reads only after selection',
  async (label, name, eventId) => {
    vi.mocked(api.get)
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ id: 1, request_id: 'req-1' }], total: 1 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            evidence: { body_expired: false },
            events: [{ id: eventId, name }],
          },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: {
            text: '{"input":"<script>literal</script>"}',
            body_status: 'available',
          },
        },
      })
    render(
      <QueryClientProvider client={new QueryClient()}>
        <TaskRequestDetails taskId='task-1' />
      </QueryClientProvider>
    )
    expect(api.get).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: label }))
    const select = await screen.findByRole('button', {
      name: `View original text · ${eventId}`,
    })
    expect(api.get).toHaveBeenCalledTimes(2)
    fireEvent.click(select)
    expect(
      await screen.findByText('{"input":"<script>literal</script>"}')
    ).toBeVisible()
    expect(document.querySelector('script')).toBeNull()
  }
)
it.each([401, 403])(
  'propagates an HTTP %s authorization failure instead of treating it as missing evidence',
  async (status) => {
    const error = new AxiosError('denied', undefined, undefined, undefined, {
      status,
      data: {},
      headers: {},
      statusText: '',
      config: {} as never,
    })
    vi.mocked(api.get).mockRejectedValue(error)
    await expect(readEvidenceOriginalText(1, 2)).rejects.toBe(error)
  }
)
it.each(['missing', 'decrypt_failed', 'storage_unavailable'])(
  'keeps the selected object error category %s',
  async (status) => {
    const error = new AxiosError(
      'unavailable',
      undefined,
      undefined,
      undefined,
      {
        status: 410,
        data: { body_status: status },
        headers: {},
        statusText: '',
        config: {} as never,
      }
    )
    vi.mocked(api.get).mockRejectedValue(error)
    await expect(readEvidenceOriginalText(1, 2)).resolves.toEqual({
      text: null,
      bodyStatus: status,
    })
  }
)
