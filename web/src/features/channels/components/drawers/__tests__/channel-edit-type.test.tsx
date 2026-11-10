import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createRouter,
  createRootRoute,
  createMemoryHistory,
  RouterContextProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getChannel } from '../../../api'
import { channelSchema } from '../../../types'
import { ChannelsDialogs } from '../../channels-dialogs'
import { ChannelsProvider, useChannels } from '../../channels-provider'
import { ChannelMutateDrawer } from '../channel-mutate-drawer'

vi.mock('../../../api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../api')>()
  return {
    ...original,
    getChannel: vi.fn(),
    getGroups: async () => ({ success: true, data: ['default'] }),
    getAllModels: async () => ({ success: true, data: [] }),
    getPrefillGroups: async () => ({ success: true, data: [] }),
  }
})

const previousUser = useAuthStore.getState().auth.user
afterEach(() => useAuthStore.getState().auth.setUser(previousUser))

test('an existing non-MiniMax channel can select native MiniMax and change back before saving', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture', role: ROLE.SUPER_ADMIN })
  const channel = channelSchema.parse({
    id: 1,
    type: 1,
    name: 'Fixture',
    key: '',
    models: 'customer',
    status: 1,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  })
  vi.mocked(getChannel).mockResolvedValue({ success: true, data: channel })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <ChannelsProvider>
          <ChannelMutateDrawer
            open
            onOpenChange={vi.fn()}
            currentRow={channel}
          />
        </ChannelsProvider>
      </RouterContextProvider>
    </QueryClientProvider>
  )
  await screen.findByDisplayValue('Fixture')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /Change provider/ }))
  const type = screen.getByPlaceholderText(
    'Search providers, plugins, or type numbers'
  )
  await user.click(type)
  await user.clear(type)
  await user.type(type, 'MiniMax')
  await user.click(
    await screen.findByRole('option', { name: 'MiniMax Built-in #35' })
  )
  expect(screen.getByText('Native API')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: /Change provider/ }))
  const newType = screen.getByPlaceholderText(
    'Search providers, plugins, or type numbers'
  )
  await user.click(newType)
  await user.clear(newType)
  await user.type(newType, 'OpenAI')
  await user.click(
    await screen.findByRole('option', { name: 'OpenAI Built-in #1' })
  )
  await waitFor(() => expect(screen.queryByText('Native API')).toBeNull())
  expect(screen.getByRole('textbox', { name: 'Name *' })).toHaveValue('Fixture')
})

function EditChannelFixture(props: {
  channel: ReturnType<typeof channelSchema.parse>
}) {
  const { setOpen, setCurrentRow } = useChannels()
  return (
    <>
      <button
        type='button'
        onClick={() => {
          setCurrentRow(props.channel)
          setOpen('update-channel')
        }}
      >
        Edit fixture channel
      </button>
      <ChannelsDialogs />
    </>
  )
}

test('searching Nexhina while editing opens a separate new MiniMax channel without copying the original configuration', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture', role: ROLE.SUPER_ADMIN })
  const channel = channelSchema.parse({
    id: 1,
    type: 61,
    name: 'Existing Hailuo',
    key: '',
    models: 'original-customer-model',
    settings: '{"task_plugin_key":"hailuo"}',
    base_url: 'https://original.invalid',
    status: 1,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
  })
  vi.mocked(getChannel).mockResolvedValue({ success: true, data: channel })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <ChannelsProvider>
          <EditChannelFixture channel={channel} />
        </ChannelsProvider>
      </RouterContextProvider>
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Edit fixture channel' }))
  const name = await screen.findByDisplayValue('Existing Hailuo')
  await user.type(name, ' draft')
  await user.click(screen.getByRole('button', { name: 'Change provider' }))
  await user.type(
    screen.getByPlaceholderText('Search providers, plugins, or type numbers'),
    'Nexhina'
  )
  await user.click(
    await screen.findByRole('option', {
      name: 'Create MiniMax standard video channel',
    })
  )
  expect(
    await screen.findByRole('alertdialog', { name: 'Discard unsaved changes?' })
  ).toBeVisible()
  await user.click(
    screen.getByRole('button', {
      name: 'Create MiniMax standard video channel',
    })
  )
  expect(
    await screen.findByRole('heading', { name: 'Create Channel' })
  ).toBeVisible()
  expect(screen.getByRole('combobox', { name: 'Access method' })).toHaveValue(
    ''
  )
  expect(screen.getByRole('textbox', { name: 'Name *' })).toHaveValue('')
  expect(screen.queryByDisplayValue('https://original.invalid')).toBeNull()
  expect(screen.queryByText('original-customer-model')).toBeNull()
  client.clear()
})
