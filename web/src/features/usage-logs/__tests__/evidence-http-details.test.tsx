import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { TaskEvidence } from '../components/task-evidence'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
afterEach(() => vi.resetAllMocks())

it('shows observed HTTP status and request metadata even when the response has no body', async () => {
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          items: [{ id: 1, request_id: 'request-1' }],
          total: 1,
        },
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
              name: 'polling',
              phase: 'completed',
              complete: true,
              has_body: false,
              byte_count: 0,
              status_code: 503,
              method: 'GET',
              target: 'https://provider.example/tasks/1',
              created_at: 1791532800,
              content_type: 'text/plain',
              detail: '{"response_received":true}',
              preview: '',
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
      <TaskEvidence taskId='task-1' isRoot={false} />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
  fireEvent.click(await screen.findByRole('button', { name: 'request-1' }))
  expect(await screen.findByText(/HTTP 503/)).toBeVisible()
  expect(screen.getByText('GET')).toBeVisible()
  expect(screen.getByText('https://provider.example/tasks/1')).toBeVisible()
  expect(screen.getByText('text/plain')).toBeVisible()
  expect(screen.queryByText(/response_received/)).toBeNull()
  expect(screen.getByText('Created At')).toBeVisible()
})

it('allows administrators to browse recorded requests without an error log or request ID', async () => {
  vi.mocked(api.get).mockResolvedValueOnce({
    data: { success: true, data: { items: [], total: 0 } },
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <TaskEvidence browse isRoot={false} />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Request evidence' }))
  expect(
    await screen.findByText(
      'No request evidence available. It may not have been recorded or may have expired.'
    )
  ).toBeVisible()
  expect(api.get).toHaveBeenCalledWith('/api/task_request_evidence', {
    params: { request_id: undefined, p: 1, page_size: 20 },
  })
})
