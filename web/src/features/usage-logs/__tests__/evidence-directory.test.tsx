import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { TaskEvidence } from '../components/task-evidence'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.resetAllMocks()
})
it('lets Admin select a raw event without object_key and loads no body with the directory', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'admin', role: ROLE.ADMIN })
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
          events: [
            {
              id: 2,
              seq: 2,
              name: 'north_receive',
              content_type: 'application/json',
            },
          ],
        },
      },
    })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: { text: '{"api_key":"synthetic"}', body_status: 'available' },
      },
    })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TaskEvidence taskId='task-1' isRoot={false} />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
  fireEvent.click(await screen.findByRole('button', { name: 'req-1' }))
  const view = await screen.findByRole('button', { name: 'View original text' })
  expect(api.get).toHaveBeenCalledTimes(2)
  expect(screen.queryByText(/Incomplete/)).toBeNull()
  expect(
    screen.getByRole('button', { name: 'Download original evidence' })
  ).toBeVisible()
  fireEvent.click(view)
  expect(await screen.findByText('{"api_key":"synthetic"}')).toBeVisible()
})
