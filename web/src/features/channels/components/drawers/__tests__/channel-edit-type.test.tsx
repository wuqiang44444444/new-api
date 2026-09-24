import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getChannel } from '../../../api'
import { channelSchema } from '../../../types'
import { ChannelsProvider } from '../../channels-provider'
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
  render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <ChannelMutateDrawer open onOpenChange={vi.fn()} currentRow={channel} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  await screen.findByDisplayValue('Fixture')
  const user = userEvent.setup()
  const type = screen.getByPlaceholderText('Search channel type...')
  await user.click(type)
  await user.clear(type)
  await user.type(type, 'MiniMax')
  await user.click(await screen.findByRole('option', { name: 'MiniMax' }))
  expect(screen.getByDisplayValue('Native API')).toBeTruthy()
  expect(type).toBeEnabled()
  await user.click(type)
  await user.clear(type)
  await user.type(type, 'OpenAI')
  await user.click(
    await screen.findByRole('option', { name: /^OpenAI$/ })
  )
  await waitFor(() =>
    expect(screen.queryByDisplayValue('Native API')).toBeNull()
  )
  expect(screen.getByRole('textbox', { name: 'Name *' })).toHaveValue('Fixture')
})
