import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

import { AdminUsage } from '../admin'
import {
  getAdminUsageCustomerSummary,
  getAdminUsageUpstreamSummary,
  getUsageSelfSummary,
} from '../api'
import { MyUsage } from '../index'

const get = vi.hoisted(() => vi.fn())
vi.mock('@/lib/api', () => ({ api: { get } }))
vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({
    t: (key: string, options?: { period?: string }) =>
      key.replace('{{period}}', options?.period ?? '{{period}}'),
    i18n: { language: 'en' },
  }),
}))

const period = {
  period: 'day',
  date: '2026-09-21',
  timezone: 'Asia/Shanghai',
  start_timestamp: 1789920000,
  end_timestamp: 1790006400,
  days: [{ date: '2026-09-21', start: 1789920000, end: 1790006400 }],
}
const total = {
  total_calls: 0,
  success_calls: 0,
  failure_calls: 0,
  cancelled_calls: 0,
  other_result_calls: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  image_count: 0,
  gross_quota: 0,
  refund_quota: 0,
  net_quota: 0,
}
function reply(result: object) {
  return {
    data: {
      success: true,
      message: '',
      data: { period, result: { total, day_totals: [total], ...result } },
    },
  }
}

it('shows the customer scope and a single empty state when no customers have usage', async () => {
  get.mockResolvedValue(reply({ customers: null }))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AdminUsage period='day' date='2026-09-21' onSearchChange={vi.fn()} />
    </QueryClientProvider>
  )
  expect(
    screen.getByRole('heading', { name: 'Admin customer usage' })
  ).toBeVisible()
  expect(
    await screen.findByText('No recorded usage for 2026-09-21')
  ).toBeVisible()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('link', { name: 'View my usage logs' })
  ).not.toBeInTheDocument()
  client.clear()
})

it('links the published empty personal day to self logs within the exact reporting period', async () => {
  const response = reply({ keys: [] })
  get.mockResolvedValue({
    data: {
      ...response.data,
      data: { ...response.data.data, published_days: [period.date] },
    },
  })
  const root = createRootRoute()
  const usage = createRoute({
    getParentRoute: () => root,
    path: '/usage',
    component: () => (
      <MyUsage period='day' date={period.date} onSearchChange={vi.fn()} />
    ),
  })
  const logs = createRoute({
    getParentRoute: () => root,
    path: '/usage-logs/$section',
  })
  const router = createRouter({
    routeTree: root.addChildren([usage, logs]),
    history: createMemoryHistory({ initialEntries: ['/usage'] }),
  })
  const client = new QueryClient()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )

  const link = await screen.findByRole('link', { name: 'View my usage logs' })
  const url = new URL((link as HTMLAnchorElement).href)
  expect(url.pathname).toBe('/usage-logs/common')
  expect(Object.fromEntries(url.searchParams)).toEqual({
    scope: 'self',
    startTime: '1789920000000',
    endTime: '1790006399000',
  })
  expect(
    screen.getByText('Reporting period (Beijing time): 2026-09-21')
  ).toBeVisible()
  expect(
    screen.queryByRole('heading', { name: 'Calls' })
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  client.clear()
})

it('shows an unpublished personal day as being prepared', async () => {
  const response = reply({ keys: [] })
  get.mockResolvedValue({
    data: {
      ...response.data,
      data: { ...response.data.data, published_days: [] },
    },
  })
  const client = new QueryClient()
  render(
    <QueryClientProvider client={client}>
      <MyUsage period='day' date={period.date} onSearchChange={vi.fn()} />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText('Some usage data is still being prepared')
  ).toBeVisible()
  expect(screen.queryByText(/No recorded usage/)).not.toBeInTheDocument()
  client.clear()
})

it('normalizes nullable key and URL slices at the API boundary', async () => {
  get.mockResolvedValue(reply({ keys: null }))
  expect(
    (await getUsageSelfSummary({ period: 'day', date: '2026-09-21' })).data
      .result.keys
  ).toEqual([])
  expect(
    (
      await getAdminUsageCustomerSummary({
        period: 'day',
        date: '2026-09-21',
        user_id: 7,
      })
    ).data.result.keys
  ).toEqual([])
  get.mockResolvedValue(reply({ url_groups: null }))
  expect(
    (await getAdminUsageUpstreamSummary({ period: 'day', date: '2026-09-21' }))
      .data.result.url_groups
  ).toEqual([])
})
