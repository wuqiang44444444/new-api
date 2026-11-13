/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { PageFooterProvider } from '@/components/layout/components/page-footer'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { usageLogSchema } from '../../data/schema'
import { UsageLogsProvider, useUsageLogsContext } from '../usage-logs-provider'
import { UsageLogsTable } from '../usage-logs-table'

function LogsFixture() {
  const { viewScope, setViewScope } = useUsageLogsContext()
  return (
    <>
      <button
        type='button'
        onClick={() => setViewScope(viewScope === 'all' ? 'self' : 'all')}
      >
        Switch scope
      </button>
      <UsageLogsTable logCategory='common' />
    </>
  )
}

async function renderLogs(props: {
  role: number
  enabledPlans: boolean[]
  activeSubscription: boolean
  cursorEnabled?: boolean
  initialPage?: number
}) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'tester', role: props.role })
  const records = ['wallet', 'subscription'].map((source, index) =>
    usageLogSchema.parse({
      id: index + 1,
      user_id: 1,
      created_at: 1788840000,
      type: 2,
      content: '',
      quota: 5000,
      other: JSON.stringify({ billing_source: source }),
    })
  )
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    let data: unknown = { quota: 0, rpm: 0, tpm: 0 }
    if (url === '/api/status') {
      data = { log_cursor_enabled: props.cursorEnabled ?? true }
    } else if (url === '/api/subscription/admin/plans') {
      data = props.enabledPlans.map((enabled) => ({ plan: { enabled } }))
    } else if (url === '/api/subscription/self') {
      data = {
        subscriptions: props.activeSubscription
          ? [{ subscription: { status: 'active' } }]
          : [],
        all_subscriptions: [{ subscription: { status: 'expired' } }],
      }
    } else if (
      url.startsWith('/api/log?') ||
      url.startsWith('/api/log/self?')
    ) {
      const params = new URL(url, 'http://localhost').searchParams
      data = {
        items: records,
        total: props.initialPage ? 102 : records.length,
        ...(params.has('page_snapshot')
          ? { page_snapshot: 'test-snapshot' }
          : {}),
      }
    } else if (url === '/api/group/') {
      data = []
    } else if (url === '/api/user/self/groups') {
      data = {}
    }
    return { data: { success: true, data } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: LogsFixture,
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({
      initialEntries: [
        props.initialPage
          ? `/usage-logs/common?page=${props.initialPage}&pageSize=100`
          : '/usage-logs/common',
      ],
    }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const footer = document.createElement('div')
  footer.dataset.testFooter = ''
  document.body.append(footer)
  render(
    <QueryClientProvider client={client}>
      <PageFooterProvider container={footer}>
        <UsageLogsProvider>
          <RouterProvider router={router} />
        </UsageLogsProvider>
      </PageFooterProvider>
    </QueryClientProvider>
  )
  await screen.findAllByText('$0.01')
  await waitFor(() => expect(client.isFetching()).toBe(0))
  return client
}

afterEach(() => {
  cleanup()
  document
    .querySelectorAll('[data-test-footer]')
    .forEach((node) => node.remove())
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

test('requests ordinary logs by page number when cursor pagination is disabled', async () => {
  await renderLogs({
    role: ROLE.USER,
    enabledPlans: [],
    activeSubscription: false,
    cursorEnabled: false,
    initialPage: 2,
  })
  const requests = vi
    .mocked(api.get)
    .mock.calls.flatMap(([url]) =>
      url.startsWith('/api/log/self?')
        ? [new URL(url, 'http://localhost').searchParams]
        : []
    )
  expect(requests).toHaveLength(1)
  expect(requests[0].get('p')).toBe('2')
  expect(requests[0].has('page_cursor')).toBe(false)
})

test('shows the exact page total and reuses the snapshot when jumping or changing page size', async () => {
  const user = userEvent.setup()
  await renderLogs({
    role: ROLE.USER,
    enabledPlans: [],
    activeSubscription: false,
    initialPage: 2,
  })
  const requests = () =>
    vi
      .mocked(api.get)
      .mock.calls.flatMap(([url]) =>
        url.startsWith('/api/log/self?')
          ? [new URL(url, 'http://localhost').searchParams]
          : []
      )
  expect(screen.getByText('Page 2 of 2')).toBeVisible()
  expect(screen.getByText('102')).toBeVisible()
  expect(requests()[0].get('page_snapshot')).toBe('start')
  expect(requests()[0].get('p')).toBe('2')
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeDisabled()

  await user.click(screen.getByRole('button', { name: 'Go to first page' }))
  expect(await screen.findByText('Page 1 of 2')).toBeVisible()
  await waitFor(() => expect(requests().at(-1)?.get('p')).toBe('1'))
  expect(requests().at(-1)?.get('page_snapshot')).toBe('test-snapshot')

  await user.click(screen.getByRole('combobox', { name: 'Rows per page' }))
  await user.click(await screen.findByRole('option', { name: '20' }))
  expect(await screen.findByText('Page 1 of 6')).toBeVisible()
  expect(requests().at(-1)?.get('page_size')).toBe('20')
  expect(requests().at(-1)?.get('page_snapshot')).toBe('test-snapshot')

  const pageNumber = screen.getByRole('spinbutton', { name: 'Page number' })
  await user.clear(pageNumber)
  await user.type(pageNumber, '7{Enter}')
  expect(requests().at(-1)?.get('p')).toBe('1')
  await user.clear(pageNumber)
  await user.type(pageNumber, '5{Enter}')
  expect(await screen.findByText('Page 5 of 6')).toBeVisible()
  expect(requests().at(-1)?.get('p')).toBe('5')
  expect(requests().at(-1)?.get('page_snapshot')).toBe('test-snapshot')
})

test.each([{ enabledPlans: [] }, { enabledPlans: [false] }])(
  'hides the wallet icon in admin view when no plan is enabled but keeps the subscription marker ($enabledPlans)',
  async ({ enabledPlans }) => {
    await renderLogs({
      role: ROLE.ADMIN,
      enabledPlans,
      activeSubscription: true,
    })

    expect(
      screen.queryByRole('img', { name: 'Wallet' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
  }
)

test('shows both source icons in admin view when any system plan is enabled', async () => {
  await renderLogs({
    role: ROLE.ADMIN,
    enabledPlans: [false, true],
    activeSubscription: false,
  })

  expect(screen.getByRole('img', { name: 'Wallet' })).toBeVisible()
  expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
})

test('keeps the subscription marker on a subscription-billed log for a user with only expired subscriptions', async () => {
  await renderLogs({
    role: ROLE.USER,
    enabledPlans: [true],
    activeSubscription: false,
  })

  expect(screen.queryByRole('img', { name: 'Wallet' })).not.toBeInTheDocument()
  expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
})

test('shows both source icons for a user with an active subscription', async () => {
  await renderLogs({
    role: ROLE.USER,
    enabledPlans: [],
    activeSubscription: true,
  })

  expect(screen.getByRole('img', { name: 'Wallet' })).toBeVisible()
  expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()
})

test('uses personal subscriptions when an admin switches to only-self view', async () => {
  const user = userEvent.setup()
  const client = await renderLogs({
    role: ROLE.ADMIN,
    enabledPlans: [true],
    activeSubscription: false,
  })
  expect(screen.getByRole('img', { name: 'Wallet' })).toBeVisible()

  await user.click(screen.getByRole('button', { name: 'Switch scope' }))
  await waitFor(() => expect(client.isFetching()).toBe(0))
  expect(screen.queryByRole('img', { name: 'Wallet' })).not.toBeInTheDocument()
  expect(screen.getByRole('img', { name: 'Subscription' })).toBeVisible()

  await user.click(screen.getByRole('button', { name: 'Switch scope' }))
  expect(await screen.findByRole('img', { name: 'Wallet' })).toBeVisible()
})
