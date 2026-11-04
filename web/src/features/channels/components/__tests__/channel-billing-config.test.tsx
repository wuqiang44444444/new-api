import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { ChannelBillingConfig } from '../channel-billing-config'

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
      <ChannelBillingConfig />
    </QueryClientProvider>
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Channel billing configuration' })
  )
  await userEvent.type(screen.getByLabelText('Channel ID'), '11')
  fireEvent.change(screen.getByLabelText('Billing month'), {
    target: { value: '2026-09' },
  })
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
it('keeps the selected month and expected version on manual discount updates', async () => {
  await openEditor()
  const percent = screen.getByLabelText('Discount percent')
  await userEvent.clear(percent)
  await userEvent.type(percent, '75')
  await userEvent.click(screen.getByRole('button', { name: 'Save discount' }))
  await waitFor(() => expect(mocks.put).toHaveBeenCalledTimes(1))
  expect(mocks.put).toHaveBeenCalledWith(
    '/api/billing/admin/upstream-discounts',
    {
      channel_id: 11,
      period_start: Date.parse('2026-09-01T00:00:00+08:00') / 1000,
      discount: '0.75000000',
      expected_version: 7,
    }
  )
})
it('preserves the draft after a version conflict and clears it on a month change', async () => {
  await openEditor()
  mocks.put.mockRejectedValueOnce(new Error('version conflict'))
  const percent = screen.getByLabelText('Discount percent')
  await userEvent.clear(percent)
  await userEvent.type(percent, '70')
  await userEvent.click(screen.getByRole('button', { name: 'Save discount' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save discount' })).toBeEnabled()
  )
  expect(percent).toHaveValue('70')
  fireEvent.change(screen.getByLabelText('Billing month'), {
    target: { value: '2026-10' },
  })
  await waitFor(() =>
    expect(screen.getByLabelText('Discount percent')).toHaveValue('80')
  )
  expect(mocks.put).toHaveBeenCalledTimes(1)
})
it('preserves all eight stored coefficient decimals when saving without an edit', async () => {
  mocks.get.mockResolvedValue({
    data: {
      success: true,
      data: {
        channel_id: 11,
        upstream_name: 'Supplier',
        expected_version: 7,
        discount: { value: '0.12345678', version: 7, source: 'database' },
      },
    },
  })
  await openEditor()
  expect(screen.getByLabelText('Discount percent')).toHaveValue('12.345678')
  await userEvent.click(screen.getByRole('button', { name: 'Save discount' }))
  await waitFor(() => expect(mocks.put).toHaveBeenCalledTimes(1))
  expect(mocks.put.mock.calls[0][1].discount).toBe('0.12345678')
})
