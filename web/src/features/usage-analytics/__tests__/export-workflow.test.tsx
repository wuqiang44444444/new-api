import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  render,
  screen,
  waitFor,
  within,
  cleanup,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import type { CustomerExportJobView } from '@/features/billing-reconciliation/export-api'
import { api } from '@/lib/api'

import { AdminUsage } from '../admin'
import { MyUsage } from '../index'
import type { UsagePeriodKind } from '../types'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
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
const period = {
  period: 'day',
  date: '2026-10-12',
  timezone: 'Asia/Shanghai',
  start_timestamp: 1791734400,
  end_timestamp: 1791820800,
  days: [{ date: '2026-10-12', start: 1791734400, end: 1791820800 }],
}
const upstreamURL = 'https://example.test'
const upstreamVersions = [
  { start: period.start_timestamp, generation: 'a'.repeat(32) },
]
const monthStart = period.start_timestamp - 11 * 86400
const upstreamMonth = {
  ...period,
  period: 'month',
  start_timestamp: monthStart,
  end_timestamp: monthStart + 31 * 86400,
  days: Array.from({ length: 31 }, (_, i) => ({
    date: `2026-10-${String(i + 1).padStart(2, '0')}`,
    start: monthStart + i * 86400,
    end: monthStart + (i + 1) * 86400,
    future: i === 30,
  })),
}
const upstreamMonthVersions = [
  { start: monthStart, generation: 'b'.repeat(32) },
  ...upstreamVersions,
]
const channelUsage = {
  channel_id: 17,
  channel_name: 'Primary upstream',
  total: {
    ...total,
    total_calls: 43,
    success_calls: 43,
    reference_amount: 100,
  },
}
const job: CustomerExportJobView = {
  job_id: 'cex_existing',
  user_id: 7,
  target_user_id: 7,
  job_type: 'usage_summary',
  status: 'succeeded',
  filters: {
    field_version: 4,
    start_timestamp: period.start_timestamp,
    end_timestamp: period.end_timestamp,
    timezone: 'Asia/Shanghai',
    usage_view: 'self',
  },
  progress: { scanned: 1, matched: 1, written: 1, files: 1 },
  cancel_requested: false,
  created_at: period.start_timestamp,
  artifact: {
    files: [
      {
        file_name: 'usage.csv',
        object_key: '',
        size_bytes: 100,
        line_count: 1,
        sha256: '',
      },
    ],
    line_count: 1,
    size_bytes: 100,
    generated_at: period.start_timestamp,
    expires_at: Math.floor(Date.now() / 1000) + 600,
  },
}
const queued = {
  ...job,
  job_id: 'cex_submitted',
  status: 'queued' as const,
  artifact: undefined,
}
const clients: QueryClient[] = []
function mount(element: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const usage = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: () => element,
  })
  const billing = createRoute({ getParentRoute: () => root, path: '/billing' })
  const router = createRouter({
    routeTree: root.addChildren([usage, billing]),
    history: createMemoryHistory(),
  })
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}
beforeEach(() => {
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (
      url === '/api/usage/admin/customers' &&
      config?.params?.group_by === 'model'
    ) {
      return { data: { success: true, data: { items: [], has_more: false } } }
    }
    if (url === '/api/usage/admin/upstream-summary') {
      if (config?.params?.rank_metric) {
        const secondPage = config.params.rank_page === 2
        return {
          data: {
            success: true,
            data: {
              has_more: !secondPage,
              items: [
                {
                  channel_id: secondPage ? 18 : channelUsage.channel_id,
                  name: secondPage
                    ? 'Second upstream'
                    : channelUsage.channel_name,
                  calls: 43,
                  tokens: 0,
                  missing_tokens: 0,
                  amount: 100,
                  pending_amount: 0,
                },
              ],
            },
          },
        }
      }
      const selected = config?.params?.channel_id === channelUsage.channel_id
      const metrics = selected
        ? channelUsage.total
        : { ...total, total_calls: 995 }
      const monthly = config?.params?.period === 'month'
      return {
        data: {
          success: true,
          data: {
            period: monthly ? upstreamMonth : period,
            generated_at: period.start_timestamp,
            published_days: monthly
              ? ['2026-10-01', period.date]
              : [period.date],
            incomplete_days: monthly ? [period.date] : [],
            report_days: monthly ? upstreamMonthVersions : upstreamVersions,
            result: {
              total: metrics,
              day_totals: monthly
                ? upstreamMonth.days.map((day) =>
                    day.date === period.date ? metrics : total
                  )
                : [metrics],
              channels: [channelUsage],
              url_groups: [
                {
                  url_key: upstreamURL,
                  display_name: upstreamURL,
                  total: metrics,
                  days: [metrics],
                  models: selected
                    ? [
                        {
                          provider_model: 'provider-text',
                          billing_mode: 'token',
                          total: metrics,
                          days: [metrics],
                          channels: [],
                        },
                      ]
                    : [],
                },
              ],
            },
          },
        },
      }
    }
    if (url === '/api/billing/exports') {
      return { data: { success: true, data: { items: [job], total: 1 } } }
    }
    if (url === '/api/billing/exports/cex_submitted') {
      return { data: { success: true, data: queued } }
    }
    if (url === '/api/billing/exports/cex_existing') {
      return { data: { success: true, data: job } }
    }
    if (String(url).endsWith('/download')) {
      return {
        data: {
          success: true,
          data: {
            files: [
              {
                file_name: 'usage.csv',
                url: 'https://downloads.example/usage.csv',
                expires_at: Math.floor(Date.now() / 1000) + 600,
                size_bytes: 100,
                line_count: 1,
                sha256: '',
              },
            ],
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: {
          period,
          result: {
            total,
            day_totals: [total],
            keys: [
              {
                token_id: 1,
                token_name: 'key',
                days: [total],
                total,
                models: [
                  { model_name: 'selected', days: [total], total },
                  { model_name: 'other', days: [total], total },
                ],
              },
            ],
            customers: [
              {
                user_id: 43,
                username: 'customer',
                display_name: 'Customer 43',
                total,
              },
            ],
            url_groups: [],
          },
        },
      },
    }
  })
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: queued },
  })
})
afterEach(async () => {
  cleanup()
  for (const c of clients.splice(0)) {
    await c.cancelQueries()
    c.clear()
  }
  vi.restoreAllMocks()
  vi.resetAllMocks()
})

it('opens existing export records and downloads without submitting a generation', async () => {
  const user = userEvent.setup()
  const download = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.href).toBe('https://downloads.example/usage.csv')
    })
  mount(<MyUsage period='day' date='2026-10-12' onSearchChange={vi.fn()} />)
  await user.click(
    await screen.findByRole('button', { name: 'Export and download' })
  )
  expect(await screen.findByText('Ready to download')).toBeVisible()
  expect(screen.queryByText('Customer #7')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Export scope' })
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Download CSV' }))
  await waitFor(() => expect(download).toHaveBeenCalledTimes(1))
  expect(api.post).not.toHaveBeenCalled()
})

it('downloads a reusable result returned by the export request and pins its existing record', async () => {
  const user = userEvent.setup()
  const download = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(() => {})
  vi.mocked(api.post).mockResolvedValueOnce({
    data: { success: true, data: job },
  })
  mount(<MyUsage period='day' date='2026-10-12' onSearchChange={vi.fn()} />)
  await user.click(
    await screen.findByRole('button', { name: 'Export and download' })
  )
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))

  expect(
    await screen.findByRole('heading', { name: 'This export' })
  ).toBeVisible()
  await waitFor(() => expect(download).toHaveBeenCalledTimes(1))
  expect(api.get).toHaveBeenCalledWith(
    '/api/billing/exports/cex_existing/download'
  )
  expect(screen.getAllByRole('article')).toHaveLength(1)
  expect(api.post).toHaveBeenCalledTimes(1)
})

it('generates the explicitly selected customer model and displays the submitted job', async () => {
  const user = userEvent.setup()
  mount(<MyUsage period='day' date='2026-10-12' onSearchChange={vi.fn()} />)
  await user.click(
    await screen.findByRole('button', { name: 'Export and download' })
  )
  const select = screen.getByRole('combobox', { name: 'Export model' })
  await user.click(select)
  await user.type(select, 'selected')
  await user.click(await screen.findByRole('option', { name: 'selected' }))
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledExactlyOnceWith(
      '/api/usage/self/exports',
      expect.objectContaining({
        model_name: 'selected',
        date: '2026-10-12',
        period: 'day',
        language: 'en',
      })
    )
  )
  expect(await screen.findByText('Queued')).toBeVisible()
})

it('uses the selected API key for the overview, model ranking and export', async () => {
  const user = userEvent.setup()
  const first = { ...total, total_calls: 3, input_tokens: 24, net_quota: 100 }
  const second = {
    ...total,
    total_calls: 1,
    output_tokens: 6,
    net_quota: 50,
    seconds: '29',
  }
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      data: {
        period: { ...period, period: 'week' },
        result: {
          total: { ...first, total_calls: 4, output_tokens: 6, net_quota: 150 },
          keys: [
            {
              token_id: 1,
              token_name: 'first-key',
              total: first,
              models: [{ model_name: 'model-a', total: first }],
            },
            {
              token_id: 0,
              token_name: '',
              total: second,
              models: [{ model_name: 'model-b', total: second }],
            },
          ],
        },
      },
    },
  })
  const onSearchChange = vi.fn()
  mount(
    <MyUsage period='week' date={period.date} onSearchChange={onSearchChange} />
  )
  const overview = await screen.findByRole('region', { name: 'Usage' })
  expect(
    screen.queryByRole('button', { name: period.date })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Monthly' }))
  expect(onSearchChange).toHaveBeenCalledWith({
    period: 'month',
    date: period.date,
  })
  expect(within(overview).getAllByRole('heading')).toHaveLength(7)
  expect(screen.queryByText('Pricing and discounts')).not.toBeInTheDocument()
  const modelRanking = screen.getByRole('region', { name: 'Model ranking' })
  expect(
    within(modelRanking).getAllByRole('heading', { level: 3 })
  ).toHaveLength(3)
  expect(within(modelRanking).getByText('75%')).toBeVisible()
  expect(within(modelRanking).getByText('80%')).toBeVisible()
  expect(within(modelRanking).getByText('66.67%')).toBeVisible()
  expect(screen.queryByRole('table')).not.toBeInTheDocument()
  const filter = screen.getByRole('combobox', { name: 'API key' })
  await user.click(filter)
  await user.type(filter, 'No API Key')
  await user.click(await screen.findByRole('option', { name: 'No API Key' }))
  expect(within(overview).queryByText('24')).not.toBeInTheDocument()
  expect(within(overview).getAllByText('6')).toHaveLength(2)
  expect(within(modelRanking).getAllByText('model-b')).toHaveLength(3)
  expect(within(modelRanking).queryByText('model-a')).not.toBeInTheDocument()
  expect(within(modelRanking).getAllByText('100%')).toHaveLength(3)
  await user.click(screen.getByRole('button', { name: 'Model usage details' }))
  const modelDetails = screen.getByRole('table', {
    name: 'Model usage details',
  })
  expect(within(modelDetails).getByText('29 billing seconds')).toBeVisible()
  expect(within(modelDetails).queryByText('model-a')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(screen.getByText('API key: No API Key')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/usage/self/exports',
      expect.objectContaining({ token_id: 0, date: period.date })
    )
  )
})

it.each(['customers', 'upstream'] as const)(
  'opens admin %s exports without submitting until generation is selected',
  async (section) => {
    const user = userEvent.setup()
    mount(
      <AdminUsage
        period='day'
        date='2026-10-12'
        section={section}
        onSearchChange={vi.fn()}
      />
    )
    await user.click(
      await screen.findByRole('button', { name: 'Export and download' })
    )
    expect(api.post).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('combobox', { name: 'Export model' })
    ).not.toBeInTheDocument()
    if (section === 'customers') {
      expect(screen.getByText('All customers')).toBeVisible()
    }
    await user.click(
      screen.getByRole('button', {
        name: section === 'customers' ? 'Export CSV' : 'Generate CSV',
      })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith(
        '/api/usage/admin/exports',
        expect.objectContaining({ view: section })
      )
    )
  }
)

it('keeps admin customer identity and model scope when generating from customer details', async () => {
  const user = userEvent.setup()
  mount(<AdminUsage period='day' date='2026-10-12' onSearchChange={vi.fn()} />)
  await user.click(await screen.findByRole('button', { name: 'Customer list' }))
  await user.click(await screen.findByRole('button', { name: 'Customer 43' }))
  expect(
    screen.getByRole('heading', { name: 'Customer 43 · #43' })
  ).toHaveFocus()
  expect(
    screen.queryByRole('region', { name: 'All-customer usage' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(
    within(screen.getByRole('dialog')).getByText('Customer 43 · #43')
  ).toBeVisible()
  const select = screen.getByRole('combobox', { name: 'Export model' })
  await user.click(select)
  await user.type(select, 'selected')
  await user.click(await screen.findByRole('option', { name: 'selected' }))
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/usage/admin/exports',
      expect.objectContaining({
        view: 'customer',
        user_id: 43,
        model_name: 'selected',
      })
    )
  )
  await user.keyboard('{Escape}')
  await user.click(
    screen.getByRole('button', { name: 'Back to customer usage' })
  )
  expect(
    await screen.findByRole('button', { name: 'Customer 43' })
  ).toHaveFocus()
})

it('keeps the all-customer total while search and export use the matching customers', async () => {
  const user = userEvent.setup()
  const customer = {
    user_id: 43,
    username: 'customer',
    display_name: 'Customer 43',
    total: { ...total, total_calls: 7 },
  }
  const second = {
    user_id: 44,
    username: 'another',
    display_name: 'Another customer',
    total: { ...total, total_calls: 36 },
  }
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/usage/admin/customers') {
      if (config?.params?.group_by === 'model') {
        return { data: { success: true, data: { items: [], has_more: false } } }
      }
      const search = config?.params?.search
      return {
        data: {
          success: true,
          data: {
            period,
            result: {
              total: { ...total, total_calls: search ? 7 : 43 },
              customers: search ? [customer] : [customer, second],
            },
          },
        },
      }
    }
    return { data: { success: true, data: { items: [] } } }
  })
  mount(<AdminUsage period='day' date={period.date} onSearchChange={vi.fn()} />)
  const overview = await screen.findByRole('region', {
    name: 'All-customer usage',
  })
  expect(await within(overview).findByText('43')).toBeVisible()
  expect(
    screen.queryByRole('region', { name: 'Customers' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Customer list' }))
  const customers = screen.getByRole('region', { name: 'Customers' })
  await user.click(
    within(customers).getByRole('combobox', { name: 'Sort customers by' })
  )
  await user.click(await screen.findByRole('option', { name: 'Calls' }))
  expect(within(customers).getAllByRole('row')[1]).toHaveTextContent(
    'Another customer'
  )

  await user.type(
    screen.getByRole('textbox', { name: 'Search customer' }),
    'Customer 43'
  )
  const matching = await screen.findByRole('region', {
    name: 'Matching customers',
  })
  expect(
    await within(matching).findByRole('button', { name: 'Customer 43' })
  ).toBeVisible()
  expect(within(overview).getByText('43')).toBeVisible()
  expect(within(matching).getByText('Filtered total')).toBeVisible()
  expect(
    within(matching).queryByRole('button', { name: 'Another customer' })
  ).not.toBeInTheDocument()

  await user.click(
    within(matching).getByRole('button', { name: 'Export search results' })
  )
  expect(screen.getByText('Customer search results')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/usage/admin/exports',
      expect.objectContaining({ view: 'customers', search: 'Customer 43' })
    )
  )
})

it('switches monthly admin cards, model charts and CSV to the selected customer', async () => {
  const user = userEvent.setup()
  const globalMetrics = {
    ...total,
    total_calls: 12,
    image_count: 8,
    input_tokens: 28,
  }
  const customerMetrics = {
    ...total,
    total_calls: 3,
    image_count: 2,
    output_tokens: 9,
  }
  const refs = [{ start: period.start_timestamp, generation: 'a'.repeat(32) }]
  const month = { ...period, period: 'month' }
  const customer = {
    user_id: 43,
    username: 'customer',
    display_name: 'Customer 43',
    total: customerMetrics,
  }
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (
      url === '/api/usage/admin/customers' &&
      config?.params?.group_by === 'model'
    ) {
      return {
        data: {
          success: true,
          data: {
            has_more: false,
            items: [
              {
                name: 'global-model',
                calls: globalMetrics.total_calls,
                tokens: 0,
                missing_tokens: 0,
                amount: globalMetrics.net_quota,
                pending_amount: 0,
                non_token: { image_count: 8, seconds: '29' },
              },
            ],
          },
        },
      }
    }
    let data: unknown = { items: [] }
    if (url === '/api/usage/admin/customers') {
      data = {
        period: month,
        report_days: refs,
        result: { total: globalMetrics, customers: [customer] },
      }
    }
    if (url === '/api/usage/admin/customer-summary') {
      data = {
        period: month,
        report_days: refs,
        result: {
          total: customerMetrics,
          keys: [
            {
              token_id: 1,
              token_name: 'key',
              total: customerMetrics,
              models: [
                { model_name: 'customer-model', total: customerMetrics },
              ],
            },
          ],
        },
      }
    }
    return { data: { success: true, data } }
  })
  mount(
    <AdminUsage period='month' date={period.date} onSearchChange={vi.fn()} />
  )
  expect(
    await screen.findByRole('button', { name: 'Monthly', pressed: true })
  ).toBeVisible()
  const model = await screen.findByRole('row', { name: /global-model/ })
  expect(within(model).getByText('8 images')).toBeVisible()
  expect(within(model).getByText('29 billing seconds')).toBeVisible()
  const allCards = screen.getByRole('region', { name: 'Usage' })
  expect(within(allCards).getAllByRole('heading')).toHaveLength(8)
  expect(within(allCards).getByText('8')).toBeVisible()
  expect(
    screen.queryByRole('table', { name: 'Daily customer usage' })
  ).not.toBeInTheDocument()
  const customerFilter = screen.getByRole('combobox', { name: 'Customer' })
  await user.click(customerFilter)
  await user.type(customerFilter, 'Customer 43')
  await user.click(
    await screen.findByRole('option', { name: 'Customer 43 · #43' })
  )
  expect(await screen.findAllByText('customer-model')).not.toHaveLength(0)
  expect(screen.queryByText('global-model')).not.toBeInTheDocument()
  expect(
    within(screen.getByRole('region', { name: 'Usage' })).getByText('2')
  ).toBeVisible()
  expect(api.get).toHaveBeenCalledWith(
    '/api/usage/admin/customer-summary',
    expect.objectContaining({
      params: expect.objectContaining({
        period: 'month',
        user_id: 43,
        report_days: JSON.stringify(refs),
      }),
    })
  )
  await user.click(screen.getByRole('button', { name: 'Export and download' }))
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/usage/admin/exports',
      expect.objectContaining({
        period: 'month',
        view: 'customer',
        user_id: 43,
        report_days: refs,
      })
    )
  )
})

it('prevents another generation while submission is pending', async () => {
  const user = userEvent.setup()
  let finish: (value: unknown) => void = () => {}
  vi.mocked(api.post).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve
      })
  )
  mount(<MyUsage period='day' date='2026-10-12' onSearchChange={vi.fn()} />)
  await user.click(
    await screen.findByRole('button', { name: 'Export and download' })
  )
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  const pending = await screen.findByRole('button', { name: 'Submitting...' })
  expect(pending).toBeDisabled()
  await user.click(pending)
  expect(api.post).toHaveBeenCalledTimes(1)
  finish({ data: { success: true, data: queued } })
  expect(await screen.findByText('Queued')).toBeVisible()
})

it('keeps the export form and model selection after a rejected submission', async () => {
  const user = userEvent.setup()
  vi.mocked(api.post).mockResolvedValueOnce({
    data: { success: false, message_key: 'Export request failed.' },
  })
  mount(<MyUsage period='day' date='2026-10-12' onSearchChange={vi.fn()} />)
  await user.click(
    await screen.findByRole('button', { name: 'Export and download' })
  )
  const select = screen.getByRole('combobox', { name: 'Export model' })
  await user.click(select)
  await user.type(select, 'selected')
  await user.click(await screen.findByRole('option', { name: 'selected' }))
  await user.click(screen.getByRole('button', { name: 'Export CSV' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Export request failed.'
  )
  expect(select).toHaveValue('selected')
  expect(screen.getByRole('button', { name: 'Export CSV' })).toBeEnabled()
  expect(api.post).toHaveBeenCalledTimes(1)
})

it('uses the selected URL, channel and frozen day for details and export', async () => {
  const user = userEvent.setup()
  mount(
    <AdminUsage
      period='day'
      date={period.date}
      section='upstream'
      onSearchChange={vi.fn()}
    />
  )
  expect(await screen.findByText('995')).toBeVisible()
  await user.click(screen.getByRole('combobox', { name: 'Upstream URL' }))
  await user.click(await screen.findByRole('option', { name: upstreamURL }))
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/usage/admin/upstream-summary',
      expect.objectContaining({
        params: expect.objectContaining({ url_key: upstreamURL }),
      })
    )
  )
  await user.click(await screen.findByRole('button', { name: 'Next page' }))
  expect(
    await screen.findByRole('button', { name: 'Second upstream · #18' })
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Total tokens' }))
  expect(api.get).toHaveBeenCalledWith(
    '/api/usage/admin/upstream-summary',
    expect.objectContaining({
      params: expect.objectContaining({
        rank_metric: 'tokens',
        rank_page: 1,
        url_key: upstreamURL,
        report_days: JSON.stringify(upstreamVersions),
      }),
    })
  )
  const rankingButton = await screen.findByRole('button', {
    name: 'Primary upstream · #17',
  })
  await user.click(rankingButton)
  expect(
    screen.getByRole('heading', { name: 'Primary upstream · #17' })
  ).toHaveFocus()
  const charts = await screen.findByRole('region', {
    name: 'Upstream model breakdown',
  })
  expect(
    within(within(charts).getByRole('group', { name: 'Calls' })).getByText(
      'provider-text'
    )
  ).toBeVisible()
  expect(screen.queryByText('995', { exact: true })).not.toBeInTheDocument()
  expect(api.get).toHaveBeenCalledWith(
    '/api/usage/admin/upstream-summary',
    expect.objectContaining({
      params: expect.objectContaining({
        channel_id: 17,
        url_key: upstreamURL,
        report_days: JSON.stringify(upstreamVersions),
      }),
    })
  )
  await user.click(screen.getByRole('button', { name: 'Export and download' }))
  const drawer = screen.getByRole('dialog')
  expect(within(drawer).getByText('Primary upstream · #17')).toBeVisible()
  expect(within(drawer).getByText(upstreamURL)).toBeVisible()
  await user.click(within(drawer).getByRole('button', { name: 'Generate CSV' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/usage/admin/exports',
      expect.objectContaining({
        view: 'upstream',
        channel_id: 17,
        url_key: upstreamURL,
        report_days: upstreamVersions,
      })
    )
  )
  await user.keyboard('{Escape}')
  await user.click(
    screen.getByRole('button', { name: 'Back to channel ranking' })
  )
  expect(rankingButton).toHaveFocus()
  expect(screen.getByText('995')).toBeVisible()
})

it('offers a natural month when viewing upstream usage and keeps it when switching tabs', async () => {
  const user = userEvent.setup()
  const onSearchChange = vi.fn()
  const view = mount(
    <AdminUsage
      period='day'
      date={period.date}
      section='upstream'
      onSearchChange={onSearchChange}
    />
  )
  await user.click(await screen.findByRole('button', { name: 'Monthly' }))
  expect(onSearchChange).toHaveBeenLastCalledWith({
    period: 'month',
    date: period.date,
    section: 'upstream',
  })
  view.unmount()

  mount(
    <AdminUsage
      period='month'
      date={period.date}
      section='customers'
      onSearchChange={onSearchChange}
    />
  )
  await user.click(await screen.findByRole('tab', { name: 'Upstream usage' }))
  expect(onSearchChange).toHaveBeenLastCalledWith({
    period: 'month',
    date: period.date,
    section: 'upstream',
  })
})

it('opens a scoped day from the upstream month and returns to the same monthly export scope', async () => {
  function MonthlyUpstream() {
    const [search, setSearch] = useState<{
      period: UsagePeriodKind
      date: string
    }>({
      period: 'month',
      date: period.date,
    })
    return (
      <AdminUsage {...search} section='upstream' onSearchChange={setSearch} />
    )
  }

  const user = userEvent.setup()
  mount(<MonthlyUpstream />)
  await user.click(
    await screen.findByRole('button', { name: 'Primary upstream · #17' })
  )
  const dailyBreakdown = await screen.findByRole('button', {
    name: 'Daily breakdown',
  })
  expect(dailyBreakdown).toHaveAttribute('aria-expanded', 'false')
  await user.click(dailyBreakdown)
  const days = screen.getByRole('table', { name: 'Daily breakdown' })
  expect(within(days).getAllByRole('row')).toHaveLength(32)
  expect(
    within(days).getByRole('rowheader', { name: /2026-10-12.*Partial data/ })
  ).toBeVisible()
  expect(
    within(days).getByRole('row', { name: /2026-10-02.*Not published/ })
  ).toBeVisible()
  expect(
    within(days).getByRole('row', { name: /2026-10-31.*Not started/ })
  ).toBeVisible()
  await user.click(within(days).getByRole('button', { name: period.date }))
  expect(
    screen.getByRole('button', { name: 'Daily', pressed: true })
  ).toBeVisible()
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/usage/admin/upstream-summary',
      expect.objectContaining({
        params: expect.objectContaining({
          period: 'day',
          channel_id: channelUsage.channel_id,
          report_days: JSON.stringify(upstreamVersions),
        }),
      })
    )
  )
  await user.click(
    screen.getByRole('button', { name: 'Back to monthly summary' })
  )
  expect(
    screen.getByRole('button', { name: 'Monthly', pressed: true })
  ).toBeVisible()
  await screen.findByRole('region', { name: 'Upstream model breakdown' })
  await user.click(screen.getByRole('button', { name: 'Export and download' }))
  const drawer = screen.getByRole('dialog')
  expect(within(drawer).getByText(/2026-10-01.*2026-10-31/)).toBeVisible()
  await user.click(within(drawer).getByRole('button', { name: 'Generate CSV' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/usage/admin/exports',
      expect.objectContaining({
        period: 'month',
        view: 'upstream',
        channel_id: channelUsage.channel_id,
        report_days: upstreamMonthVersions,
      })
    )
  )
})
