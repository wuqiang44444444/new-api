import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { UpstreamReconciliationView } from '../upstream-reconciliation'

const mocks = vi.hoisted(() => ({
  summary: vi.fn(),
  details: vi.fn(),
  exports: vi.fn(),
}))
vi.mock('../../api', () => ({
  getAdminUpstreamReconciliation: mocks.summary,
  getAdminUpstreamDetails: mocks.details,
}))
vi.mock('../../export-api', () => ({ createUpstreamExport: mocks.exports }))
vi.mock('../../export-jobs-drawer', () => ({ ExportJobsDrawer: () => null }))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, vars?: Record<string, unknown>) =>
      key.replaceAll(/{{(\w+)}}/g, (_, name) => String(vars?.[name] ?? name)),
  }),
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const period = { start_timestamp: 1788192000, end_timestamp: 1790783999 }
const usage = {
  requests: 1,
  input_tokens: 10,
  output_tokens: 5,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  billable_calls: 0,
}
const channel = (id: number) => ({
  channel_id: id,
  channel_name: `Channel ${id}`,
  upstream_name: 'Same supplier',
  discount: { value: '0.8', version: 1, source: 'database' },
  usage,
  models: [],
  original_amount: 100,
  reference_amount: 80,
})
function view() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UpstreamReconciliationView
        month='2026-09'
        onMonthChange={vi.fn()}
        period={period}
      />
    </QueryClientProvider>
  )
}
beforeEach(() => {
  vi.clearAllMocks()
  mocks.summary.mockImplementation(async (params) => ({
    success: true,
    data: {
      generated_at: 1,
      result: {
        channels: params.level === 'models' ? [] : [channel(11), channel(12)],
        models:
          params.level === 'models'
            ? [
                {
                  provider_model: 'provider-model',
                  billing_mode: 'token',
                  customer_models: ['customer'],
                  usage,
                  detail_filter: { channel_id: params.channel_id },
                  original_amount: 100,
                  reference_amount: 80,
                },
              ]
            : [],
        page: params.page ?? 1,
        page_size: params.page_size ?? 10,
        total: params.level === 'models' ? 1 : 2,
        model_count: 2,
      },
    },
  }))
  mocks.details.mockResolvedValue({
    success: true,
    data: { result: { items: [], total: 0, page: 1, page_size: 50 } },
  })
  mocks.exports.mockResolvedValue({ job_id: 'export-1' })
})
it('shows independent channels without exposing old URL grouping or edit controls', async () => {
  view()
  expect(await screen.findByText(/Channel 11/)).toBeInTheDocument()
  expect(screen.getByText(/Channel 12/)).toBeInTheDocument()
  expect(screen.getAllByText('Same supplier')).toHaveLength(2)
  expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument()
  expect(mocks.summary).toHaveBeenCalledWith(
    expect.objectContaining({ level: 'channels' }),
    expect.any(AbortSignal)
  )
  expect(mocks.summary.mock.calls[0][0]).not.toHaveProperty('url_key')
})
it('loads models only after expanding the selected channel', async () => {
  view()
  await screen.findAllByRole('button', { name: 'Expand models' })
  expect(
    mocks.summary.mock.calls.some(([params]) => params.level === 'models')
  ).toBe(false)
  await userEvent.click(
    screen.getAllByRole('button', { name: 'Expand models' })[1]
  )
  expect(
    await screen.findByText('customer - provider-model')
  ).toBeInTheDocument()
  expect(mocks.summary).toHaveBeenCalledWith(
    expect.objectContaining({ level: 'models', channel_id: 12 }),
    expect.any(AbortSignal)
  )
})
it('exports only the clicked channel and preserves the exact period', async () => {
  view()
  await screen.findAllByRole('button', { name: 'Export summary' })
  await userEvent.click(
    screen.getAllByRole('button', { name: 'Export summary' })[1]
  )
  await waitFor(() => expect(mocks.exports).toHaveBeenCalledTimes(1))
  expect(mocks.exports).toHaveBeenCalledWith({
    job_type: 'upstream_summary',
    channel_id: 12,
    start_timestamp: period.start_timestamp,
    end_timestamp: period.end_timestamp + 1,
  })
})
it('keeps export retryable after a failed submission', async () => {
  mocks.exports.mockRejectedValueOnce(new Error('storage unavailable'))
  view()
  await screen.findAllByRole('button', { name: 'Export summary' })
  await userEvent.click(
    screen.getAllByRole('button', { name: 'Export summary' })[0]
  )
  await waitFor(() =>
    expect(
      screen.getAllByRole('button', { name: 'Export summary' })[0]
    ).toBeEnabled()
  )
  await userEvent.click(
    screen.getAllByRole('button', { name: 'Export summary' })[0]
  )
  await waitFor(() => expect(mocks.exports).toHaveBeenCalledTimes(2))
})
it('searches beyond the current page on the server', async () => {
  view()
  await screen.findAllByRole('button', { name: 'Export summary' })
  await userEvent.type(
    screen.getByLabelText('Channel name or ID'),
    'deleted supplier'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(mocks.summary).toHaveBeenCalledWith(
      expect.objectContaining({
        level: 'channels',
        search: 'deleted supplier',
        page: 1,
      }),
      expect.any(AbortSignal)
    )
  )
})
it('shows a retryable load error instead of claiming empty usage', async () => {
  mocks.summary.mockRejectedValueOnce(new Error('database unavailable'))
  view()
  expect(
    await screen.findByText('Unable to load upstream reconciliation')
  ).toBeInTheDocument()
  expect(screen.queryByText('No upstream usage')).not.toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: /retry/i }))
  expect(await screen.findByText(/Channel 11/)).toBeInTheDocument()
})
