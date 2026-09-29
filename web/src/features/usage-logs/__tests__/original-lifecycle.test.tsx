import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { EvidenceOriginalDialog } from '../components/evidence-original-dialog'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

beforeEach(() => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'root', role: ROLE.SUPER_ADMIN })
})
afterEach(() => useAuthStore.getState().auth.reset())

it.each(['close', 'switch', 'logout'] as const)(
  'discards an in-flight original after %s and does not expose it later',
  async (action) => {
    const client = new QueryClient()
    let finish!: (value: { data: string }) => void
    const pending = new Promise<{ data: string }>((resolve) => {
      finish = resolve
    })
    vi.mocked(api.get)
      .mockReturnValueOnce(pending)
      .mockResolvedValue({ data: 'new event body' })
    const view = (eventId: number, open = true) => (
      <QueryClientProvider client={client}>
        <EvidenceOriginalDialog
          id={1}
          eventId={eventId}
          open={open}
          onOpenChange={() => {}}
        />
      </QueryClientProvider>
    )
    const rendered = render(view(2))
    await waitFor(() => expect(api.get).toHaveBeenCalledTimes(1))
    const signal = vi.mocked(api.get).mock.calls[0][1]?.signal
    if (action === 'close') rendered.rerender(view(2, false))
    if (action === 'switch') rendered.rerender(view(3))
    if (action === 'logout') {
      act(() => {
        client.clear()
        useAuthStore.getState().auth.reset()
      })
    }
    await act(async () => {
      finish({ data: 'old private body' })
      await pending
    })
    expect(signal?.aborted).toBe(true)
    expect(screen.queryByText('old private body')).toBeNull()
    expect(
      JSON.stringify(
        client
          .getQueryCache()
          .getAll()
          .map((q) => q.state.data)
      )
    ).not.toContain('old private body')
    if (action === 'switch') {
      expect(await screen.findByText('new event body')).toBeVisible()
    } else {
      expect(screen.queryByRole('dialog')).toBeNull()
    }
  }
)
