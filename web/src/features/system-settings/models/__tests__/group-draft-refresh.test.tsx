import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { RatioSettingsCard } from '../ratio-settings-card'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))
const groups = {
  GroupRatio: '{"default":1}',
  TopupGroupRatio: '{}',
  GroupDescriptions: '{}',
  UserUsableGroups: '{"default":"Standard"}',
  GroupGroupRatio: '{}',
  AutoGroups: '[]',
  MaxTokenAutoGroups: 5,
  DefaultUseAutoGroup: false,
  GroupSpecialUsableGroup: '{}',
}
const models = {
  ModelPrice: '{}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  ExposeRatioEnabled: false,
  BillingMode: '{}',
  BillingExpr: '{}',
  PluginBillingExpr: '{}',
  TaskPreConsumeTokens: '{}',
}
const clients: QueryClient[] = []
function Fixture(props: { groups: typeof groups; client: QueryClient }) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <QueryClientProvider client={props.client}>
      <SettingsPageProvider actionsContainer={actions}>
        <div ref={setActions} />
        <RatioSettingsCard
          modelDefaults={models}
          groupDefaults={props.groups}
          toolPricesDefault='{}'
          visibleTabs={['groups']}
        />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}
afterEach(async () => {
  cleanup()
  for (const client of clients.splice(0)) {
    await client.cancelQueries()
    client.clear()
  }
  vi.resetAllMocks()
})

it('keeps unsaved group rows when the settings query refreshes', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: false, message: 'Pricing is outside this fixture' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const user = userEvent.setup()
  const view = render(<Fixture groups={groups} client={client} />)
  await user.click(screen.getByRole('button', { name: 'Add group' }))
  const name = screen.getByDisplayValue('group_1')
  await user.type(name, '_draft')
  view.rerender(<Fixture groups={{ ...groups }} client={client} />)
  expect(name).toBeInTheDocument()
  expect(name).toHaveValue('group_1_draft')
  expect(api.put).not.toHaveBeenCalled()
})

it('loads external group settings when the form has no unsaved changes', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: false, message: 'Pricing is outside this fixture' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const view = render(<Fixture groups={groups} client={client} />)
  view.rerender(
    <Fixture
      groups={{
        ...groups,
        GroupRatio: '{"updated":0.8}',
        UserUsableGroups: '{}',
      }}
      client={client}
    />
  )
  expect(await screen.findByDisplayValue('updated')).toBeVisible()
  expect(screen.queryByDisplayValue('default')).not.toBeInTheDocument()
})

it('keeps the complete draft when a later option save fails after a successful option refresh', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: false, message: 'Pricing is outside this fixture' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const user = userEvent.setup()
  const view = render(<Fixture groups={groups} client={client} />)
  vi.mocked(api.put).mockImplementation(async (_url, body) => {
    const request = body as { key: string; value: string }
    if (request.key === 'GroupRatio') {
      view.rerender(
        <Fixture
          groups={{ ...groups, GroupRatio: request.value }}
          client={client}
        />
      )
      return { data: { success: true } }
    }
    throw new Error('The later option could not be saved')
  })
  await user.click(screen.getByRole('button', { name: 'Add group' }))
  const name = screen.getByDisplayValue('group_1')
  await user.type(name, '_draft')
  await user.click(screen.getByRole('button', { name: 'Save group settings' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Your changes have been kept.'
  )
  expect(name).toBeInTheDocument()
  expect(name).toHaveValue('group_1_draft')
  expect(api.put).toHaveBeenCalledTimes(2)
})

it('preserves an unnamed draft on refresh even when serialized settings equal the saved values', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: false, message: 'Pricing is outside this fixture' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const initial = { ...groups, GroupDescriptions: '{"default":"Standard"}' }
  const user = userEvent.setup()
  const view = render(<Fixture groups={initial} client={client} />)
  await user.click(screen.getByRole('button', { name: 'Add group' }))
  const name = screen.getByDisplayValue('group_1')
  await user.clear(name)
  view.rerender(<Fixture groups={{ ...initial }} client={client} />)
  expect(name).toBeInTheDocument()
  expect(name).toHaveValue('')
  expect(screen.getByRole('alert')).toHaveTextContent(
    'Enter a name for every pricing group.'
  )
})

it('accepts a later settings refresh after an explicit save succeeds', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: false, message: 'Pricing is outside this fixture' },
  })
  vi.mocked(api.put).mockResolvedValue({ data: { success: true } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const user = userEvent.setup()
  const view = render(<Fixture groups={groups} client={client} />)
  await user.click(screen.getByRole('button', { name: 'Add group' }))
  await user.type(screen.getByDisplayValue('group_1'), '_saved')
  await user.click(screen.getByRole('button', { name: 'Save group settings' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(3))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Save group settings' })
    ).toBeEnabled()
  )
  view.rerender(
    <Fixture
      groups={{
        ...groups,
        GroupRatio: '{"after_save":1}',
        UserUsableGroups: '{}',
      }}
      client={client}
    />
  )
  expect(await screen.findByDisplayValue('after_save')).toBeVisible()
  expect(screen.queryByDisplayValue('group_1_saved')).not.toBeInTheDocument()
})
