import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { UsageLogs } from '../../index'
import { UsageLogsProvider } from '../usage-logs-provider'
import { UsageLogsTable } from '../usage-logs-table'

it.each([
  {
    name: 'keeps the selected billing group open when resetting pagination',
    direct: false,
  },
  {
    name: 'opens matching billing events without requesting groups',
    direct: true,
  },
  {
    name: 'keeps personal billing grouped when an old link has an upstream request ID',
    direct: false,
    legacyFilter: true,
  },
])('$name', async ({ direct, legacyFilter }) => {
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const group = {
    key: '2026-11-03.test-group',
    date: '2026-11-03',
    model_name: 'test-model',
    net_quota: 5000,
    event_count: 1,
  }
  const statementRequests: URLSearchParams[] = []
  const requests = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const { pathname, searchParams } = new URL(url, 'http://localhost')
    if (pathname !== '/api/billing/statement/self/logs') {
      return { data: { success: true, data: {} } }
    }
    statementRequests.push(searchParams)
    return {
      data: {
        success: true,
        data:
          searchParams.get('group_funds') === 'true'
            ? { items: [], total: 101, quota: 5000, fund_groups: [group] }
            : { items: [], total: 0, quota: 0 },
      },
    }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: () => <UsageLogsTable logCategory='common' />,
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({
      initialEntries: [
        direct
          ? '/usage-logs/common?billing=true&billingView=events&tokenId=187&billingMode=token&model=test-model&billingReportVersion=published-report'
          : `/usage-logs/common?billing=true&page=2&pageSize=100&model=test-model${legacyFilter ? '&upstreamRequestId=old-upstream-id' : ''}`,
      ],
    }),
  })

  try {
    render(
      <QueryClientProvider client={client}>
        <UsageLogsProvider>
          <RouterProvider router={router} />
        </UsageLogsProvider>
      </QueryClientProvider>
    )
    if (direct) {
      expect(await screen.findByText('No Logs Found')).toBeVisible()
      expect(statementRequests).toHaveLength(1)
      const [request] = statementRequests
      expect(request.get('model_name')).toBe('test-model')
      expect(request.get('token_id')).toBe('187')
      expect(request.get('billing_mode')).toBe('token')
      expect(request.get('report_version')).toBe('published-report')
      expect(request.has('group_funds')).toBe(false)
      expect(request.has('fund_group')).toBe(false)
      expect(screen.queryByRole('button', { name: 'Back' })).toBeNull()
      return
    }

    expect(await screen.findByText('Page 2 of 2')).toBeVisible()
    if (legacyFilter) {
      expect(statementRequests[0].get('group_funds')).toBe('true')
      expect(statementRequests[0].has('upstream_request_id')).toBe(false)
    }
    expect(
      screen.getByRole('combobox', { name: 'Rows per page' })
    ).toHaveTextContent('100')
    await user.click(screen.getByRole('button', { name: 'View details (1)' }))

    expect(await screen.findByText('No Logs Found')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Back' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'View details (1)' })
    ).toBeNull()
  } finally {
    cleanup()
    client.clear()
    requests.mockRestore()
  }
})

it('filters by the old upstream request ID in All and groups funds after switching to Only Mine', async () => {
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const adminRequests: URLSearchParams[] = []
  const selfRequests: URLSearchParams[] = []
  const requests = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const { pathname, searchParams } = new URL(url, 'http://localhost')
    if (pathname === '/api/billing/admin/customer-logs') {
      adminRequests.push(searchParams)
      return { data: { success: true, data: { items: [], total: 0 } } }
    }
    if (pathname === '/api/billing/statement/self/logs') {
      selfRequests.push(searchParams)
      const grouped = searchParams.get('group_funds') === 'true'
      return {
        data: {
          success: true,
          data: {
            items: [],
            total: grouped ? 1 : 0,
            quota: 5000,
            fund_groups: grouped
              ? [
                  {
                    key: '2026-11-03.test-group',
                    date: '2026-11-03',
                    model_name: 'test-model',
                    net_quota: 5000,
                    event_count: 2,
                  },
                ]
              : undefined,
          },
        },
      }
    }
    return { data: { success: true, data: {} } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: UsageLogs,
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({
      initialEntries: [
        '/usage-logs/common?scope=all&billing=true&billingUserId=42&upstreamRequestId=old-upstream-id',
      ],
    }),
  })
  const { user: previousUser, setUser } = useAuthStore.getState().auth

  try {
    setUser({ id: 1, username: 'admin', role: 10 })
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )

    expect(await screen.findByText('No Logs Found')).toBeVisible()
    expect(screen.getByRole('tab', { name: 'All' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(adminRequests).toHaveLength(1)
    expect(adminRequests[0].get('user_id')).toBe('42')
    expect(adminRequests[0].get('upstream_request_id')).toBe('old-upstream-id')
    expect(adminRequests[0].has('group_funds')).toBe(false)
    expect(selfRequests).toHaveLength(0)

    await user.click(screen.getByRole('tab', { name: 'Only Mine' }))

    expect(
      await screen.findByRole('button', { name: 'View details (2)' })
    ).toBeVisible()
    expect(screen.getByRole('tab', { name: 'Only Mine' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(router.state.location.search).toMatchObject({
      scope: 'self',
      upstreamRequestId: 'old-upstream-id',
    })
    expect(adminRequests).toHaveLength(1)
    expect(selfRequests).toHaveLength(1)
    expect(selfRequests[0].has('upstream_request_id')).toBe(false)
    expect(selfRequests[0].get('group_funds')).toBe('true')
  } finally {
    cleanup()
    setUser(previousUser)
    client.clear()
    requests.mockRestore()
  }
})
