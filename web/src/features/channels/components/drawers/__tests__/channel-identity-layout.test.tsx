import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { ChannelsProvider } from '../../channels-provider'
import { ChannelMutateDrawer } from '../channel-mutate-drawer'

vi.mock('../../../api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../api')>()
  return {
    ...original,
    getGroups: async () => ({ success: true, data: ['default'] }),
    getAllModels: async () => ({ success: true, data: [] }),
    getPrefillGroups: async () => ({ success: true, data: [] }),
  }
})

test('channel name follows type before the MiniMax access method in reading order', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelMutateDrawer open onOpenChange={vi.fn()} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  const type = screen.getByPlaceholderText('Search channel type...')
  await user.click(type)
  await user.clear(type)
  await user.type(type, 'MiniMax')
  await user.click(await screen.findByRole('option', { name: 'MiniMax' }))
  const name = screen.getByRole('textbox', { name: 'Name *' })
  const access = screen.getByRole('combobox', { name: 'Access method' })
  expect(
    type.compareDocumentPosition(name) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    name.compareDocumentPosition(access) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  name.focus()
  await user.tab()
  expect(access).toHaveFocus()
})
