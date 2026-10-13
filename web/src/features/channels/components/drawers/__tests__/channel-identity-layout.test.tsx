import {createRouter, createRootRoute, createMemoryHistory, RouterContextProvider} from '@tanstack/react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
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
  const previousUser = useAuthStore.getState().auth.user
  useAuthStore.getState().auth.setUser({id:1,username:'fixture',role:100})
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({routeTree:createRootRoute(),history:createMemoryHistory({initialEntries:['/']})})
  render(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}><ChannelsProvider>
        <ChannelMutateDrawer open onOpenChange={vi.fn()} />
      </ChannelsProvider></RouterContextProvider>
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  const type = screen.getByPlaceholderText('Search providers, plugins, or type numbers')
  await user.click(type)
  await user.clear(type)
  await user.type(type, 'MiniMax')
  await user.click(await screen.findByRole('option', { name: 'MiniMax Built-in #35' }))
  const name = screen.getByRole('textbox', { name: 'Name *' })
  const access = screen.getByRole('combobox', { name: 'Access method' })
  expect(
    screen.getByRole('button', {name: /Change provider/}).compareDocumentPosition(name) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    name.compareDocumentPosition(access) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(access).not.toBeDisabled()
  useAuthStore.getState().auth.setUser(previousUser)
  client.clear()
})
