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
import { afterEach, expect, it, vi } from 'vitest'

import { PageFooterProvider } from '@/components/layout/components/page-footer'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { usageLogSchema } from '../../data/schema'
import { UsageLogsProvider } from '../usage-logs-provider'
import { UsageLogsTable } from '../usage-logs-table'

const client = new QueryClient({
  defaultOptions: { queries: { retry: false } },
})

afterEach(() => {
  cleanup()
  document.querySelector('[data-test-footer]')?.remove()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  vi.restoreAllMocks()
})

async function renderLogs(
  behavior?:
    | 'expire-count'
    | 'legacy-page'
    | 'legacy-count'
    | 'old-status'
    | 'legacy-stat'
    | 'new-count-error'
) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'tester', role: ROLE.SUPER_ADMIN })
  const requests: URLSearchParams[] = []
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    let data: unknown = {}
    if (url === '/api/group/') data = []
    if (url === '/api/status') {
      data = {
        log_cursor_enabled: true,
        ...(behavior === 'old-status'
          ? {}
          : { log_deferred_total_enabled: true }),
      }
    }
    if (url.startsWith('/api/log/console/stat?')) {
      requests.push(new URL(url, 'http://localhost').searchParams)
      if (behavior === 'legacy-stat') return { status: 404, data: {} }
      data = { quota: 0, rpm: 0, tpm: 0, total: 41 }
    } else if (
      url.startsWith('/api/log/console/page?') ||
      url.startsWith('/api/log?')
    ) {
      const params = new URL(url, 'http://localhost').searchParams
      requests.push(params)
      const last = params.get('page_total') === 'last'
      const page = last ? 3 : Number(params.get('p'))
      const counted = params.get('page_total') === 'count'
      if (counted && behavior === 'expire-count') {
        return {
          data: { success: false, code: 'log_snapshot_expired' },
        }
      }
      if ((counted || last) && behavior === 'legacy-count') {
        return { status: 404, data: {} }
      }
      if (counted && behavior === 'new-count-error') {
        return {
          status: 200,
          data: { success: false, message: 'Count timed out' },
        }
      }
      if (
        page === 2 &&
        params.has('page_total') &&
        behavior === 'legacy-page'
      ) {
        return { status: 404, data: {} }
      }
      const legacy = !params.has('page_total')
      const exact = params.get('page_snapshot') === 'counted'
      const id = params.get('request_id')
      let total = 0
      if (id) total = 1
      else if (counted || last || exact || legacy) total = 41
      let snapshot: string | undefined
      if (!legacy) snapshot = counted || last || exact ? 'counted' : 'initial'
      data = {
        items: counted
          ? []
          : [
              usageLogSchema.parse({
                id: page,
                user_id: 7,
                request_id: id || `request-page-${page}`,
                created_at: 1788840000,
                type: 2,
                content: '',
              }),
            ],
        total,
        page,
        total_pending: !counted && !last && !exact && !id && !legacy,
        page_snapshot: snapshot,
        has_more: page < 3,
        start_timestamp: 1788700000,
        end_timestamp: 1788840000,
      }
    }
    return { status: 200, data: { success: true, data } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    validateSearch: (search: Record<string, unknown>) => search,
    component: () => (
      <UsageLogsProvider>
        <UsageLogsTable logCategory='common' />
      </UsageLogsProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    scrollRestoration: false,
    history: createMemoryHistory({
      initialEntries: [
        behavior === 'expire-count'
          ? '/usage-logs/common?pageSize=40'
          : '/usage-logs/common',
      ],
    }),
  })
  await router.load()
  const footer = document.createElement('div')
  footer.dataset.testFooter = ''
  document.body.append(footer)
  render(
    <QueryClientProvider client={client}>
      <PageFooterProvider container={footer}>
        <RouterProvider router={router} />
      </PageFooterProvider>
    </QueryClientProvider>
  )
  return requests
}

it('reads the next page without counting and loads the last page in one request', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs()
  await user.click(
    await screen.findByRole('button', { name: 'Go to next page' })
  )
  await waitFor(() =>
    expect(requests.some((params) => params.get('p') === '2')).toBe(true)
  )
  expect(requests.map((params) => params.get('page_total'))).toEqual([
    'defer',
    'defer',
  ])

  await user.click(
    await screen.findByRole('button', { name: 'Go to last page' })
  )
  expect(await screen.findByText('Page 3 of 3')).toBeVisible()
  expect(requests.map((params) => params.get('page_total'))).toEqual([
    'defer',
    'defer',
    'last',
  ])
  expect(requests.at(-1)?.get('page_snapshot')).toBe('initial')
})

it('uses the displayed range for range statistics even when the route has no dates', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs()
  await screen.findByRole('button', { name: 'View total' })
  await user.click(
    screen.getByRole('button', { name: 'View range statistics' })
  )
  await screen.findByText('Total Count')
  const stats = requests.find((params) => params.get('range_count') === 'true')
  expect(stats?.get('start_timestamp')).toBe('1788700000')
  expect(stats?.get('end_timestamp')).toBe('1788840000')
  expect(stats?.get('resolved_log_range')).toBe('true')
})

it('restarts at the first page after reopening an expired count snapshot', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs('expire-count')
  await user.click(
    await screen.findByRole('button', { name: 'Go to next page' })
  )
  await screen.findByText('Page 2')
  await user.click(screen.getByRole('button', { name: 'View total' }))
  await user.click(await screen.findByRole('button', { name: 'Refresh' }))
  await screen.findByText('Page 1')
  expect(requests.at(-1)?.get('page_snapshot')).toBe('start')
  expect(requests.at(-1)?.get('page_total')).toBe('defer')
  expect(requests.at(-1)?.get('page_size')).toBe('40')
})

it('uses exact numbered pages after the next-page request reaches an older instance', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs('legacy-page')
  await user.click(
    await screen.findByRole('button', { name: 'Go to next page' })
  )
  await screen.findByText('Page 2 of 3')
  expect(requests.at(-1)?.has('page_snapshot')).toBe(false)
  expect(requests.at(-1)?.get('start_timestamp')).toBe('1788700000')
  await user.click(screen.getByRole('button', { name: 'Go to next page' }))
  await screen.findByText('Page 3 of 3')
  expect(requests.at(-1)?.has('page_total')).toBe(false)
})

it('performs one legacy count when the new count route reaches V50', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs('legacy-count')
  await user.click(
    await screen.findByRole('button', { name: 'Go to last page' })
  )
  expect(
    await screen.findByText('Total loaded. Select the page again.')
  ).toBeVisible()
  const legacy = () => requests.filter((params) => !params.has('page_total'))
  expect(legacy()).toHaveLength(1)
  expect(legacy()[0].get('p')).toBe('1')

  await user.click(screen.getByRole('button', { name: 'Go to last page' }))
  await screen.findByText('Page 3 of 3')
  expect(legacy().map((params) => params.get('p'))).toEqual(['1', '3'])
})

it('uses the atomic last-page read even when a total is already cached', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs()
  await user.click(await screen.findByRole('button', { name: 'View total' }))
  await screen.findByText('Page 1 of 3')
  await user.click(screen.getByRole('button', { name: 'Go to last page' }))
  await screen.findByText('Page 3 of 3')
  expect(requests.map((params) => params.get('page_total'))).toEqual([
    'defer',
    'count',
    'last',
  ])
})

it('bounds the first V50 request when status comes from the old snapshot version', async () => {
  const requests = await renderLogs('old-status')
  await screen.findByText('Page 1 of 3')
  expect(requests).toHaveLength(1)
  expect(requests[0].has('page_snapshot')).toBe(false)
  const start = Number(requests[0].get('start_timestamp'))
  const end = Number(requests[0].get('end_timestamp'))
  expect(start).toBeGreaterThan(0)
  expect(end - start).toBe(3600)
})

it('does not fall back when the new count handler reports a query failure', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs('new-count-error')
  await user.click(await screen.findByRole('button', { name: 'View total' }))
  expect(await screen.findByText('Failed to load total')).toBeVisible()
  expect(requests.map((params) => params.get('page_total'))).toEqual([
    'defer',
    'count',
  ])
})

it('does not try the old statistics endpoint after the new route returns 404', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs('legacy-stat')
  await screen.findByRole('button', { name: 'View total' })
  await user.click(
    screen.getByRole('button', { name: 'View range statistics' })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Range statistics are temporarily unavailable.'
  )
  expect(screen.queryByText('Total Count')).toBeNull()
  const stats = requests.find((params) => params.get('range_count') === 'true')
  expect(stats?.get('start_timestamp')).toBe('1788700000')
  expect(stats?.get('end_timestamp')).toBe('1788840000')
  expect(
    vi
      .mocked(api.get)
      .mock.calls.some(([url]) => url.startsWith('/api/log/stat?'))
  ).toBe(false)
})

it('sends a pasted Request ID without implicit dates and displays the searched range', async () => {
  const user = userEvent.setup()
  const requests = await renderLogs()
  await screen.findByRole('button', { name: 'View total' })
  await user.type(
    screen.getByRole('textbox', { name: 'Request ID' }),
    '  old-request  {Enter}'
  )
  expect(await screen.findByText(/^Searched .* to /)).toBeVisible()
  const search = requests.find(
    (params) => params.get('request_id') === 'old-request'
  )
  expect(search?.has('start_timestamp')).toBe(false)
  expect(search?.has('end_timestamp')).toBe(false)
  await user.click(
    screen.getByRole('button', { name: 'View range statistics' })
  )
  await screen.findByText('Total Count')
  const stats = requests.find((params) => params.get('range_count') === 'true')
  expect(stats?.get('request_id')).toBe('old-request')
  expect(stats?.get('start_timestamp')).toBe('1788700000')
  expect(stats?.get('end_timestamp')).toBe('1788840000')
})
