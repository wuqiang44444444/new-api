import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { ChannelUpstreamNameConfig } from '../channel-billing-config'

const mocks = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: mocks }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
beforeEach(() => {
  vi.clearAllMocks()
  mocks.get.mockResolvedValue({
    data: {
      success: true,
      data: {
        channel_id: 11,
        channel_name: 'Original channel',
        upstream_name: 'Supplier',
        deleted: true,
        expected_version: 7,
        discount: { value: '0.8', version: 7, source: 'database' },
      },
    },
  })
  mocks.put.mockResolvedValue({ data: { success: true } })
})
async function openEditor() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelUpstreamNameConfig />
    </QueryClientProvider>
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Edit upstream name' })
  )
  await userEvent.type(screen.getByLabelText('Channel ID'), '11')
  await userEvent.click(screen.getByRole('button', { name: 'Load' }))
  await screen.findByDisplayValue('Supplier')
}
it('only saves a name on the name action, including for a deleted channel', async () => {
  await openEditor()
  const name = screen.getByLabelText('Upstream name')
  await userEvent.clear(name)
  await userEvent.type(name, 'New supplier')
  await userEvent.click(screen.getByRole('button', { name: 'Save name' }))
  await waitFor(() => expect(mocks.put).toHaveBeenCalledTimes(1))
  expect(mocks.put).toHaveBeenCalledWith('/api/billing/admin/channel-name', {
    channel_id: 11,
    name: 'New supplier',
  })
})
it('keeps channel management focused on names', async () => {
  await openEditor()
  expect(screen.queryByLabelText('Discount percent')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Save discount' })
  ).not.toBeInTheDocument()
})
