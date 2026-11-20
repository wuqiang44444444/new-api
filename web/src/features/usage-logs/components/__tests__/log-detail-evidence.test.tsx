import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { usageLogSchema } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it.each([
  {
    label: 'an asynchronous task refund',
    type: 6,
    requestId: 'task-billing:6249:adjustment',
    filter: { task_id: 'task_video' },
  },
  {
    label: 'an asynchronous task creation charge',
    type: 2,
    requestId: 'task-billing:6249:create',
    filter: { task_id: 'task_video' },
  },
  {
    label: 'an HTTP log that also has a task ID',
    type: 2,
    requestId: 'gateway-http-request',
    filter: { request_id: 'gateway-http-request' },
  },
])('loads request evidence for $label', async ({ type, requestId, filter }) => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    const matches =
      url === '/api/task_request_evidence' &&
      Object.entries(filter).every(
        ([key, value]) => (config?.params as Record<string, unknown> | undefined)?.[key] === value
      )
    const items = matches
      ? [{ id: 1, request_id: 'captured-request', body_expired: false }]
      : []
    return { data: { success: true, data: { items, total: items.length } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { gcTime: 0, retry: false } },
  })
  const user = userEvent.setup()
  const log = usageLogSchema.parse({
    id: 1,
    user_id: 1,
    created_at: 1000,
    type,
    content: '',
    model_name: 'model',
    quota: 100,
    request_id: requestId,
    other: JSON.stringify({ task_id: 'task_video' }),
  })
  render(
    <QueryClientProvider client={client}>
      <DetailsDialog
        log={log}
        isAdmin
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )

  await user.click(screen.getByRole('button', { name: 'Request evidence' }))
  expect(get).toHaveBeenCalledWith('/api/task_request_evidence', {
    params: { ...filter, p: 1, page_size: 20 },
  })
  expect(
    await screen.findByRole('button', { name: 'captured-request' })
  ).toBeVisible()
})
