import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  render,
  screen,
  waitFor,
  within,
  cleanup,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import type { CustomerExportJobView } from '@/features/billing-reconciliation/export-api'
import { api } from '@/lib/api'

import { AdminUsage } from '../admin'
import { MyUsage } from '../index'

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
    expires_at: 4102444800,
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
  return render(
    <QueryClientProvider client={client}>{element}</QueryClientProvider>
  )
}
beforeEach(() => {
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/billing/exports') {
      return { data: { success: true, data: { items: [job], total: 1 } } }
    }
    if (url === '/api/billing/exports/cex_submitted') {
      return { data: { success: true, data: queued } }
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
  vi.resetAllMocks()
})

it('opens existing export records and downloads without submitting a generation', async () => {
  const user = userEvent.setup()
  mount(<MyUsage period='day' date='2026-10-12' onSearchChange={vi.fn()} />)
  await user.click(
    await screen.findByRole('button', { name: 'Export and download' })
  )
  expect(await screen.findByText('Ready to download')).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Download' }))
  expect(
    await screen.findByRole('link', { name: 'Download usage.csv' })
  ).toHaveAttribute('href', 'https://downloads.example/usage.csv')
  expect(api.post).not.toHaveBeenCalled()
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
  await user.click(screen.getByRole('button', { name: 'Generate CSV' }))
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
    await user.click(screen.getByRole('button', { name: 'Generate CSV' }))
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
  await user.click(await screen.findByRole('button', { name: 'Customer 43' }))
  const detail = screen.getByText('Customer #43 usage').parentElement
  if (!detail) throw new Error('Customer detail header is missing')
  await user.click(
    within(detail).getByRole('button', { name: 'Export and download' })
  )
  const select = screen.getByRole('combobox', { name: 'Export model' })
  await user.click(select)
  await user.type(select, 'selected')
  await user.click(await screen.findByRole('option', { name: 'selected' }))
  await user.click(screen.getByRole('button', { name: 'Generate CSV' }))
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
  await user.click(screen.getByRole('button', { name: 'Generate CSV' }))
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
  await user.click(screen.getByRole('button', { name: 'Generate CSV' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Export request failed.'
  )
  expect(select).toHaveValue('selected')
  expect(screen.getByRole('button', { name: 'Generate CSV' })).toBeEnabled()
  expect(api.post).toHaveBeenCalledTimes(1)
})
