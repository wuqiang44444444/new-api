import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { getBalance, type BalanceConnection } from './api'
import { UpstreamBalances } from './index'

const { get, t, locale } = vi.hoisted(() => ({
  get: vi.fn(),
  locale: { language: 'en' },
  t: (key: string, values?: Record<string, string>) =>
    key.replaceAll(/\{\{(\w+)\}\}/g, (_, name) => values?.[name] ?? name),
}))
vi.mock('@/lib/api', () => ({ api: { get } }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t,
    i18n: locale,
  }),
}))

afterEach(() => {
  cleanup()
  locale.language = 'en'
})

const connection: BalanceConnection = {
  id: 'opaque-reference',
  channel_id: 1,
  key_index: 0,
  key_label: '••••0001',
  origin: 'https://balance.example',
  url_key: 'https://balance.example',
  group_name: 'Balance upstream',
  channels: [{ id: 1, key_index: 0, name: 'Channel A', enabled: true }],
  queryable: true,
}
function reply(data: unknown) {
  return { data: { success: true, data } }
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <QueryClientProvider client={client}>
      <UpstreamBalances />
    </QueryClientProvider>
  )
  return { ...view, client }
}

it.each(['en', 'zhCN'])(
  'renders balances and timestamps in %s, preserving precise values and unsupported rows',
  async (language) => {
    locale.language = language
    get.mockImplementation((url: string) =>
      Promise.resolve(
        url.endsWith('/upstream-balances/')
          ? reply([
              connection,
              {
                ...connection,
                id: 'unsupported',
                channel_id: 2,
                origin: 'https://azure.example',
                url_key: 'https://azure.example',
                group_name: 'Azure upstream',
                queryable: false,
                reason: 'unsupported_credentials',
                channels: [
                  { id: 2, key_index: 0, name: 'Azure', enabled: false },
                ],
              },
            ])
          : reply({
              status: 'ok',
              amounts: [{ amount: '-0.123456789012345678', unit: 'CNY' }],
              scope: 'unconfirmed',
              checked_at: 1700000000,
            })
      )
    )
    const { client } = renderPage()
    fireEvent.click(
      await screen.findByRole('button', { name: /Balance upstream/ })
    )
    fireEvent.click(screen.getByRole('button', { name: /Azure upstream/ }))
    expect(await screen.findByText('-0.123456789012345678')).toBeInTheDocument()
    expect(screen.getByText('CNY')).toBeInTheDocument()
    expect(
      screen.getByText('Account or key scope unconfirmed')
    ).toBeInTheDocument()
    expect(
      screen.getByText('The channel credentials cannot query account balances')
    ).toBeInTheDocument()
    expect(get).toHaveBeenCalledTimes(2)
    const requests = get.mock.calls.map(([url]) => url)
    expect(requests).not.toContain('/api/upstream-balances/2/0')
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Search upstream connections' }),
      { target: { value: 'Azure' } }
    )
    expect(
      screen.queryByText('https://balance.example')
    ).not.toBeInTheDocument()
    expect(screen.getByText('https://azure.example')).toBeInTheDocument()
    client.clear()
  }
)

it('refreshes a row and all connections, replacing an old amount with a failure state', async () => {
  let response = {
    status: 'ok',
    amounts: [{ amount: '0', unit: 'USD' }],
    checked_at: 1700000000,
  } as object
  get.mockImplementation((url: string) =>
    Promise.resolve(
      reply(url.endsWith('/upstream-balances/') ? [connection] : response)
    )
  )
  const { client } = renderPage()
  fireEvent.click(
    await screen.findByRole('button', { name: /Balance upstream/ })
  )
  expect(
    await screen.findByText('0', { selector: 'span.font-semibold' })
  ).toBeInTheDocument()
  response = {
    status: 'error',
    reason: 'authentication_failed',
    checked_at: 1700000001,
  }
  fireEvent.click(
    screen.getByRole('button', {
      name: 'Refresh balance for https://balance.example ••••0001',
    })
  )
  expect(
    await screen.findByText('The upstream rejected the channel credentials')
  ).toBeInTheDocument()
  expect(
    screen.queryByText('0', { selector: 'span.font-semibold' })
  ).not.toBeInTheDocument()
  response = {
    status: 'unlimited',
    reason: 'unlimited_key',
    checked_at: 1700000002,
  }
  fireEvent.click(screen.getByRole('button', { name: 'Refresh all balances' }))
  expect(
    await screen.findByText('Unlimited key quota; account balance unavailable')
  ).toBeInTheDocument()
  expect(
    get.mock.calls.filter(([url]) => url === '/api/upstream-balances/')
  ).toHaveLength(2)
  expect(
    get.mock.calls.filter(([url]) => url === '/api/upstream-balances/1/0')
  ).toHaveLength(3)
  expect(screen.queryByText('100000000')).not.toBeInTheDocument()
  client.clear()
})

it('shows a distinct empty state and a recoverable inventory error', async () => {
  get.mockRejectedValueOnce(new Error('forbidden'))
  get.mockResolvedValue(reply([]))
  const { client } = renderPage()
  expect(
    await screen.findByText('Unable to load upstream connections')
  ).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No upstream connections')).toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  client.clear()
})

it('limits concurrent queries and does not send cancelled queued requests', async () => {
  const finish: (() => void)[] = []
  get.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish.push(() =>
          resolve(reply({ status: 'ok', checked_at: 1, amounts: [] }))
        )
      })
  )
  const controllers = Array.from({ length: 5 }, () => new AbortController())
  const requests = controllers.map((controller, index) =>
    getBalance({ ...connection, channel_id: index + 1 }, controller.signal)
  )
  const outcomes = Promise.allSettled(requests)
  expect(get).toHaveBeenCalledTimes(4)
  controllers[4].abort()
  finish.forEach((resolve) => resolve())
  expect((await outcomes)[4].status).toBe('rejected')
  await waitFor(() => expect(get).toHaveBeenCalledTimes(4))
})

it('groups keys by URL, uses reconciliation names and preserves expansion across refresh', async () => {
  const second = {
    ...connection,
    id: 'second-key',
    key_label: '••••0002',
    key_index: 1,
  }
  const other = {
    ...connection,
    id: 'different-path',
    url_key: 'https://balance.example/v2',
    group_name: 'Balance upstream',
    key_label: '••••0003',
  }
  get.mockImplementation((url: string) =>
    Promise.resolve(
      reply(
        url.endsWith('/upstream-balances/')
          ? [connection, second, other]
          : {
              status: 'ok',
              amounts: [{ amount: '12.34', unit: 'USD' }],
              checked_at: 1700000000,
            }
      )
    )
  )
  const { client } = renderPage()
  const group = await screen.findByRole('button', {
    name: /Balance upstream.*2 keys/,
  })
  expect(
    screen.getAllByRole('button', { name: /Balance upstream/ })
  ).toHaveLength(2)
  expect(group).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByText('••••0001')).not.toBeInTheDocument()
  fireEvent.click(group)
  expect(await screen.findByText('••••0001')).toBeInTheDocument()
  expect(screen.getByText('••••0002')).toBeInTheDocument()
  expect(screen.queryByText('••••0003')).not.toBeInTheDocument()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Refresh all balances' })
    ).toBeEnabled()
  )
  fireEvent.click(screen.getByRole('button', { name: 'Refresh all balances' }))
  await waitFor(() => expect(group).toHaveAttribute('aria-expanded', 'true'))
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Search upstream connections' }),
    { target: { value: '0002' } }
  )
  expect(
    screen.getAllByRole('button', { name: /Balance upstream/ })
  ).toHaveLength(1)
  expect(screen.getByText('••••0001')).toBeInTheDocument()
  fireEvent.change(
    screen.getByRole('textbox', { name: 'Search upstream connections' }),
    { target: { value: 'Balance upstream' } }
  )
  expect(
    screen.getAllByRole('button', { name: /Balance upstream/ })
  ).toHaveLength(2)
  client.clear()
})
