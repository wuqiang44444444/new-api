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
      .mockResolvedValue({
        data: {
          success: true,
          data: { text: 'new event body', body_status: 'available' },
        },
      })
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

it('formats only whitespace and preserves exact numeric tokens and duplicate JSON keys', async () => {
  const raw = '{"id":9007199254740993,"id":18446744073709551615,"amount":1.00}'
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { text: raw, body_status: 'available' } },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  expect(await screen.findByText(raw)).toBeVisible()
  const { fireEvent } = await import('@testing-library/react')
  fireEvent.click(screen.getByRole('button', { name: 'Format JSON' }))
  expect(screen.getByText(/9007199254740993/)).toHaveTextContent(
    '18446744073709551615'
  )
  expect(screen.getByText(/9007199254740993/)).toHaveTextContent('1.00')
  fireEvent.click(screen.getByRole('button', { name: 'Raw' }))
  expect(screen.getByText(raw)).toBeVisible()
})

it('keeps malformed JSON readable when formatting fails', async () => {
  const raw = '{"api_key":"synthetic'
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { text: raw, body_status: 'available' } },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  expect(await screen.findByText(raw)).toBeVisible()
  const { fireEvent } = await import('@testing-library/react')
  fireEvent.click(screen.getByRole('button', { name: 'Format JSON' }))
  expect(screen.getByText(raw)).toBeVisible()
  expect(screen.getByRole('status')).toHaveTextContent(
    'Unable to format JSON. Showing raw content.'
  )
})

it('previews captured raster bytes without fetching remote URLs', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        text: null,
        base64: 'iVBORw0KGgo=',
        content_type: 'image/png',
        body_status: 'available',
      },
    },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  const preview = await screen.findByRole('img', {
    name: 'Evidence image preview',
  })
  expect(preview).toHaveAttribute('src', 'data:image/png;base64,iVBORw0KGgo=')
  expect(api.get).toHaveBeenCalledTimes(1)
})

it('displays SVG evidence as literal data and never mounts its markup', async () => {
  const text = '<svg onload="throw new Error()"><script>unsafe()</script></svg>'
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { text, content_type: 'image/svg+xml', body_status: 'available' },
    },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  expect(await screen.findByText(text)).toBeVisible()
  expect(
    screen.queryByRole('img', { name: 'Evidence image preview' })
  ).toBeNull()
  expect(document.querySelector('script')).toBeNull()
  expect(document.querySelector('[onload]')).toBeNull()
})

it('does not request or mount original evidence for an ordinary user', () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 8, username: 'user', role: ROLE.USER })
  vi.mocked(api.get).mockClear()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(api.get).not.toHaveBeenCalled()
})
