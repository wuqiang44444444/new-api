import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { EvidenceOriginalDialog } from '../components/evidence-original-dialog'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: { number: number }) =>
      values ? key.replace('{{number}}', String(values.number)) : key,
  }),
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

it('shows opaque raw text without formatting and identifies a limited display window', async () => {
  const raw = '{"id":9007199254740993,"id":18446744073709551615,"amount":1.00}'
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { text: raw, body_status: 'available', display_limited: true },
    },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  expect(await screen.findByText(raw)).toBeVisible()
  expect(screen.getByRole('button', { name: 'Format JSON' })).toBeDisabled()
  expect(screen.getByRole('status')).toHaveTextContent(
    'Only the first 64 KiB is shown.'
  )
  expect(
    screen.getByRole('button', { name: 'Download original evidence' })
  ).toBeVisible()
})

it('shows malformed raw JSON unchanged', async () => {
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
  expect(screen.getByRole('button', { name: 'Format JSON' })).toBeEnabled()
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

it('formats only after a click and preserves numeric lexemes and duplicate fields', async () => {
  const raw = '{"id":9007199254740993,"id":18446744073709551615,"amount":1.00}'
  const formatted =
    '{\n  "id": 9007199254740993,\n  "id": 18446744073709551615,\n  "amount": 1.00\n}'
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { text: raw, body_status: 'available' } },
    })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          text: raw,
          formatted_text: formatted,
          json_status: 'formatted',
          body_status: 'available',
        },
      },
    })
  const client = new QueryClient()
  const rendered = render(
    <QueryClientProvider client={client}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  expect(await screen.findByText(raw)).toBeVisible()
  expect(api.get).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole('button', { name: 'Format JSON' }))
  await waitFor(() =>
    expect(document.querySelector('pre')?.textContent).toBe(formatted)
  )
  expect(api.get).toHaveBeenLastCalledWith(
    expect.any(String),
    expect.objectContaining({ params: { format: 'json' } })
  )
  expect(
    screen.getByRole('button', { name: 'Show original text' })
  ).toBeEnabled()
  rendered.unmount()
  expect(client.getQueriesData({ queryKey: ['evidence-original'] })).toEqual([])
})

it('keeps raw text visible when selected JSON formatting is unavailable', async () => {
  const raw = '{invalid'
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { text: raw, body_status: 'available' } },
    })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          text: raw,
          json_status: 'invalid_json',
          body_status: 'available',
        },
      },
    })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  await screen.findByText(raw)
  fireEvent.click(screen.getByRole('button', { name: 'Format JSON' }))
  expect(
    await screen.findByText(
      'JSON formatting is unavailable. Original text is shown.'
    )
  ).toBeVisible()
  expect(screen.getByText(raw)).toBeVisible()
})

it('shows separate WebSocket message boundaries and excludes binary payloads', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        text: null,
        body_status: 'available',
        messages: [
          {
            message_type: 1,
            offset: 0,
            created_at_ms: 1000,
            original_bytes: 2,
            text: '{}',
          },
          {
            message_type: 1,
            offset: 34,
            created_at_ms: 1001,
            original_bytes: 2,
            text: '{}',
          },
          {
            message_type: 2,
            offset: 68,
            created_at_ms: 1002,
            original_bytes: 200,
            text: null,
          },
        ],
      },
    },
  })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  expect(await screen.findAllByText('{}')).toHaveLength(2)
  expect(screen.getByText('Binary message: metadata only')).toBeVisible()
  expect(screen.getByText('Message 1')).toBeVisible()
  expect(screen.getByText('Message 2')).toBeVisible()
  expect(screen.getByText('Message 3')).toBeVisible()
})

it('allows returning to original text after a formatting request fails', async () => {
  const raw = '{}'
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { text: raw, body_status: 'available' } },
    })
    .mockRejectedValueOnce(new Error('format request failed'))
    .mockResolvedValue({
      data: { success: true, data: { text: raw, body_status: 'available' } },
    })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  await screen.findByText(raw)
  fireEvent.click(screen.getByRole('button', { name: 'Format JSON' }))
  await screen.findByRole('alert')
  fireEvent.click(screen.getByRole('button', { name: 'Show original text' }))
  expect(await screen.findByText(raw)).toBeVisible()
})

it('cancels an in-flight formatting request and clears both views on close', async () => {
  let finish!: (value: unknown) => void
  const pending = new Promise((resolve) => {
    finish = resolve
  })
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { text: '{}', body_status: 'available' } },
    })
    .mockReturnValueOnce(pending as ReturnType<typeof api.get>)
  const client = new QueryClient()
  const rendered = render(
    <QueryClientProvider client={client}>
      <EvidenceOriginalDialog id={1} eventId={2} open onOpenChange={() => {}} />
    </QueryClientProvider>
  )
  await screen.findByText('{}')
  fireEvent.click(screen.getByRole('button', { name: 'Format JSON' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  const signal = vi.mocked(api.get).mock.calls[1][1]?.signal
  expect(
    screen.getByRole('button', { name: 'Show original text' })
  ).toHaveAttribute('aria-busy', 'true')
  rendered.unmount()
  expect(signal?.aborted).toBe(true)
  await act(async () => {
    finish({
      data: {
        success: true,
        data: {
          text: '{}',
          formatted_text: 'private formatted response',
          body_status: 'available',
        },
      },
    })
    await pending
  })
  expect(client.getQueriesData({ queryKey: ['evidence-original'] })).toEqual([])
  expect(screen.queryByText('private formatted response')).toBeNull()
})
