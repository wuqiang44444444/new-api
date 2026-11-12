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
])('$name', async ({ direct }) => {
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
          : '/usage-logs/common?billing=true&page=2&pageSize=100&model=test-model',
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
