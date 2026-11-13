import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { StrictMode, useState } from 'react'
import { afterEach, beforeEach, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { usageLogSchema } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

const summary = usageLogSchema.parse({
  id: 1,
  user_id: 7,
  created_at: 1000,
  type: 2,
  content: '',
  detail_ref: 'test-log-reference',
})

function DetailTrigger() {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type='button' onClick={() => setOpen(true)}>
        View details
      </button>
      <DetailsDialog
        log={summary}
        open={open}
        isAdmin
        isRoot={false}
        onOpenChange={setOpen}
      />
    </>
  )
}

const originalAdapter = api.defaults.adapter
const client = new QueryClient()
const detailResponse = {
  success: true,
  data: { ...summary, request_id: 'req-detail-1' },
}

beforeEach(() => {
  client.setQueryData(['status'], {})
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
})

it('shows full details on the first click when StrictMode restarts the fetch', async () => {
  api.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: detailResponse,
  })
  render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <DetailTrigger />
      </QueryClientProvider>
    </StrictMode>
  )

  fireEvent.click(screen.getByRole('button', { name: 'View details' }))

  expect(await screen.findByText('req-detail-1')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
})

it('shows Retry when the server rejects the detail read and loads the detail on retry', async () => {
  let response: unknown = { success: false }
  api.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: response,
  })
  render(
    <QueryClientProvider client={client}>
      <DetailTrigger />
    </QueryClientProvider>
  )

  fireEvent.click(screen.getByRole('button', { name: 'View details' }))
  const retry = await screen.findByRole('button', { name: 'Retry' })
  response = detailResponse
  fireEvent.click(retry)

  expect(await screen.findByText('req-detail-1')).toBeVisible()
})
