import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { formatCustomerStatementQuota } from '../../lib'
import { UpstreamReconciliationView } from '../upstream-reconciliation'

const mocks = vi.hoisted(() => ({
  summary: vi.fn(),
  details: vi.fn(),
  exports: vi.fn(),
  sign: vi.fn(),
  download: vi.fn(),
  config: vi.fn(),
  saveDiscount: vi.fn(),
  getChannels: vi.fn(),
  searchChannels: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: { get: mocks.config, put: mocks.saveDiscount },
}))
vi.mock('@/features/channels/api', () => ({
  getChannels: mocks.getChannels,
  searchChannels: mocks.searchChannels,
}))
vi.mock('../../api', () => ({
  getAdminUpstreamReconciliation: mocks.summary,
  getAdminUpstreamDetails: mocks.details,
}))
vi.mock('../../export-api', () => ({
  createUpstreamExport: mocks.exports,
  requestExportDownload: mocks.sign,
  downloadExportFile: mocks.download,
  listSelfExports: async () => ({ items: [], total: 0 }),
  isExportJobActive: () => false,
  getExport: async () => undefined,
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    i18n: { language: 'en' },
    t: (key: string, vars?: Record<string, unknown>) =>
      key.replaceAll(/{{(\w+)}}/g, (_, name) => String(vars?.[name] ?? name)),
  }),
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))
const version = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
const upstreamUrl = 'https://billing.example/v1'
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
const urlChannel = (id: number, url = upstreamUrl) => ({
  ...channel(id),
  url_key: url,
  base_url: url,
  url_name: url,
})
async function openChannel(id: number) {
  const name = await screen.findByText(`Channel ${id}`, { exact: true })
  const row = name.closest('tr')
  if (!row) throw new Error('Expected a channel summary row')
  await userEvent.click(
    within(row).getByRole('button', { name: 'View details' })
  )
}

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
      billing_report: {
        version,
        published_days: 1,
        expected_days: 30,
        as_of: 1,
      },
      result: {
        url_channels:
          params.level === 'url_channels'
            ? [urlChannel(11), urlChannel(12)]
            : [],
        model_rows:
          params.level === 'url_models'
            ? [
                {
                  ...channel(params.channel_id),
                  provider_model: 'provider-model',
                  billing_mode: 'token',
                  customer_models: ['customer'],
                  detail_filter: { channel_id: params.channel_id },
                },
              ]
            : [],
        page: params.page ?? 1,
        page_size: params.page_size ?? 10,
        total: params.level === 'url_models' ? 1 : 2,
        model_count: 2,
        data_quality: {
          status: 'partial',
          evidence_coverage: { rows: 12, gap_rows: 3, usage_gap_rows: 3 },
        },
      },
    },
  }))
  mocks.config.mockResolvedValue({
    data: {
      success: true,
      data: {
        channel_id: 12,
        channel_name: 'Channel 12',
        discount: { value: '0.8', source: 'database', version: 7 },
        expected_version: 7,
      },
    },
  })
  mocks.getChannels.mockResolvedValue({
    success: true,
    data: { items: [], total: 0 },
  })
  mocks.searchChannels.mockResolvedValue({
    success: true,
    data: {
      items: [
        { id: 99, name: 'New upstream', base_url: 'https://new.example/v1' },
      ],
      total: 1,
    },
  })
  mocks.saveDiscount.mockResolvedValue({
    data: { success: true, data: { version: 8 } },
  })
  mocks.details.mockResolvedValue({
    success: true,
    data: { result: { items: [], total: 0, page: 1, page_size: 50 } },
  })
  mocks.exports.mockResolvedValue({ job_id: 'export-1', status: 'succeeded' })
  mocks.sign.mockResolvedValue({ files: [], emptyResult: true })
})
it('shows one row per channel even when both channels share a URL', async () => {
  view()
  const table = await screen.findByRole('table')
  expect(within(table).getAllByText(upstreamUrl)).toHaveLength(2)
  expect(within(table).getByText('Channel 11')).toBeVisible()
  expect(within(table).getByText('Channel 12')).toBeVisible()
  expect(
    within(table).getAllByRole('button', { name: 'Set discount' })
  ).toHaveLength(2)
  expect(mocks.summary).toHaveBeenCalledWith(
    expect.objectContaining({ level: 'url_channels' }),
    expect.any(AbortSignal)
  )
  expect(mocks.summary.mock.calls[0][0]).not.toHaveProperty('url_key')
})
it('left-aligns the report status when it wraps below the upstream controls', async () => {
  view()

  const report = await screen.findByText(/1 of 30 days published/)
  const status = report.closest('div')?.parentElement

  expect(status?.parentElement).toHaveClass('flex-wrap', 'justify-between')
  expect(status).not.toHaveClass('lg:ms-auto')
})
it('lets a report label longer than the toolbar wrap within its available width', async () => {
  view()

  expect(await screen.findByText(/1 of 30 days published/)).toHaveClass(
    'max-w-full',
    'whitespace-normal',
    'h-auto'
  )
})
it('refreshes the list, total and pinned model details after saving the discount', async () => {
  let saved = false
  mocks.saveDiscount.mockImplementationOnce(async () => {
    saved = true
    return { data: { success: true, data: { version: 8 } } }
  })
  const read = mocks.summary.getMockImplementation()
  if (!read) throw new Error('Missing upstream report fixture')
  mocks.summary.mockImplementation(async (params) => {
    const response = await read(params)
    if (!saved) return response
    const result = response.data.result
    const updated = {
      discount: { value: '0.75', version: 8, source: 'database' },
      reference_amount: 75,
    }
    return {
      ...response,
      data: {
        ...response.data,
        result: {
          ...result,
          url_channels: result.url_channels.map(
            (row: ReturnType<typeof urlChannel>) =>
              row.channel_id === 12 ? { ...row, ...updated } : row
          ),
          model_rows: result.model_rows.map(
            (row: ReturnType<typeof channel>) =>
              row.channel_id === 12 ? { ...row, ...updated } : row
          ),
          totals: { usage, original_amount: 200, reference_amount: 155 },
        },
      },
    }
  })
  view()
  await openChannel(12)
  expect(await screen.findByText('×0.8 (20% off)')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Back' }))
  const row = await screen.findByRole('row', { name: /Channel 12/ })
  await userEvent.click(
    within(row).getByRole('button', { name: 'Set discount' })
  )
  const dialog = within(screen.getByRole('dialog', { name: 'Set discount' }))
  expect(
    dialog.getByText(
      'This discount applies to all URLs for this channel in 2026-09.'
    )
  ).toBeVisible()
  const percent = await dialog.findByLabelText('Discount percent')
  expect(percent).toHaveValue('80')
  expect(dialog.queryByLabelText('Channel ID')).not.toBeInTheDocument()
  expect(mocks.config).toHaveBeenCalledWith(
    '/api/billing/admin/channel-config',
    {
      params: { channel_id: 12, period_start: period.start_timestamp },
    }
  )
  await userEvent.clear(percent)
  await userEvent.type(percent, '75')
  await userEvent.click(dialog.getByRole('button', { name: 'Save discount' }))
  await waitFor(() =>
    expect(mocks.saveDiscount).toHaveBeenCalledWith(
      '/api/billing/admin/upstream-discounts',
      {
        channel_id: 12,
        period_start: period.start_timestamp,
        discount: '0.75000000',
        expected_version: 7,
      },
      { skipErrorHandler: true }
    )
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Set discount' })
    ).not.toBeInTheDocument()
  )
  const updatedRow = within(
    await screen.findByRole('row', { name: /Channel 12/ })
  )
  expect(updatedRow.getByText('×0.75 (25% off)')).toBeVisible()
  expect(updatedRow.getByText(formatCustomerStatementQuota(75))).toBeVisible()
  expect(screen.getByText(formatCustomerStatementQuota(155))).toBeVisible()
  await openChannel(12)
  const models = within(await screen.findByRole('table'))
  expect(await models.findByText('×0.75 (25% off)')).toBeVisible()
  expect(models.getByText(formatCustomerStatementQuota(75))).toBeVisible()
})

it('previews and submits the discount from the selected day through month end', async () => {
  view()
  const row = await screen.findByRole('row', { name: /Channel 12/ })
  await userEvent.click(
    within(row).getByRole('button', { name: 'Set discount' })
  )
  const dialog = within(screen.getByRole('dialog', { name: 'Set discount' }))
  const scope = await dialog.findByLabelText('Discount effective range')
  await userEvent.selectOptions(scope, 'date')
  fireEvent.change(dialog.getByLabelText('Effective date (Beijing time)'), {
    target: { value: '2026-09-12' },
  })
  const percent = dialog.getByLabelText('Discount percent')
  await userEvent.clear(percent)
  await userEvent.type(percent, '75')
  expect(dialog.getByText('2026-09-01 – 2026-09-11')).toBeVisible()
  expect(dialog.getByText('2026-09-12 – 2026-09-30')).toBeVisible()
  await userEvent.click(dialog.getByRole('button', { name: 'Save discount' }))
  await waitFor(() =>
    expect(mocks.saveDiscount).toHaveBeenCalledWith(
      '/api/billing/admin/upstream-discounts',
      expect.objectContaining({
        effective_date: '2026-09-12',
        discount: '0.75000000',
        expected_version: 7,
      }),
      { skipErrorHandler: true }
    )
  )
})

it('preserves the discount draft and requires reloading after a version conflict', async () => {
  mocks.saveDiscount.mockRejectedValueOnce(
    Object.assign(new Error('version conflict'), {
      isAxiosError: true,
      response: { status: 409 },
    })
  )
  view()
  const row = await screen.findByRole('row', { name: /Channel 12/ })
  await userEvent.click(
    within(row).getByRole('button', { name: 'Set discount' })
  )
  const dialog = within(screen.getByRole('dialog', { name: 'Set discount' }))
  const percent = await dialog.findByLabelText('Discount percent')
  await userEvent.clear(percent)
  await userEvent.type(percent, '12.345678')
  await userEvent.click(dialog.getByRole('button', { name: 'Save discount' }))
  await dialog.findByRole('button', { name: 'Refresh' })
  expect(dialog.getByRole('button', { name: 'Save discount' })).toBeDisabled()
  expect(percent).toHaveValue('12.345678')
  mocks.config.mockResolvedValueOnce({
    data: {
      success: true,
      data: {
        channel_id: 12,
        channel_name: 'Channel 12',
        discount: { value: '0.7', source: 'database', version: 8 },
        expected_version: 8,
      },
    },
  })
  await userEvent.click(dialog.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(dialog.getByRole('button', { name: 'Save discount' })).toBeEnabled()
  )
  expect(percent).toHaveValue('12.345678')
  expect(dialog.getByText('×0.7 (30% off)')).toBeVisible()
  await userEvent.click(dialog.getByRole('button', { name: 'Save discount' }))
  await waitFor(() =>
    expect(mocks.saveDiscount).toHaveBeenLastCalledWith(
      '/api/billing/admin/upstream-discounts',
      {
        channel_id: 12,
        period_start: period.start_timestamp,
        discount: '0.12345678',
        expected_version: 8,
      },
      { skipErrorHandler: true }
    )
  )
})

it('allows opening discount settings for a channel absent from the report', async () => {
  view()
  await screen.findByRole('table')
  const toolbarButton = screen
    .getAllByRole('button', { name: 'Set discount' })
    .find((button) => !button.closest('table'))
  if (!toolbarButton) throw new Error('Expected a toolbar discount action')
  await userEvent.click(toolbarButton)
  const dialog = within(screen.getByRole('dialog', { name: 'Set discount' }))
  await userEvent.type(dialog.getByLabelText('Channel'), 'New upstream')
  await userEvent.click(dialog.getByRole('button', { name: 'Search' }))
  await userEvent.click(
    await dialog.findByRole('button', { name: /New upstream.*#99/ })
  )
  expect(await dialog.findByLabelText('Discount percent')).toBeVisible()
  expect(mocks.config).toHaveBeenCalledWith(
    '/api/billing/admin/channel-config',
    {
      params: { channel_id: 99, period_start: period.start_timestamp },
    }
  )
})

it('opens report evidence categories on demand and queries the selected records', async () => {
  const detail = {
    row_id: 1,
    time: period.start_timestamp + 1,
    channel_id: 11,
    channel_name: 'Channel 11',
    provider_model: 'provider-model',
    customer_model: 'customer',
    billing_mode: 'token',
    recorded_input_tokens: 10,
    output_tokens: 5,
    original_amount: 100,
    request_id: 'request-test',
    upstream_request_id: 'upstream-test',
  }
  mocks.details.mockResolvedValue({
    success: true,
    data: {
      result: {
        items: [
          { ...detail, event: 'call' },
          { ...detail, row_id: 2, event: 'task_adjustment' },
        ],
        total: 2,
      },
    },
  })
  view()
  const evidence = await screen.findByRole('button', {
    name: '3 of 12 records lack usage details · amounts available',
  })
  expect(screen.getByText(/Channel 11/)).toBeVisible()
  expect(
    screen.queryByText('Amount available, usage details missing: 3 records')
  ).not.toBeInTheDocument()
  expect(mocks.details).not.toHaveBeenCalled()

  await userEvent.click(evidence)
  await userEvent.click(
    screen.getByRole('button', {
      name: 'View records: Amount available, usage details missing: 3 records',
    })
  )
  await waitFor(() =>
    expect(mocks.details).toHaveBeenCalledWith(
      expect.objectContaining({ ...period, evidence_filter: 'usage_gap' })
    )
  )
  const details = within(screen.getByRole('dialog', { name: 'View details' }))
  expect(
    await details.findByRole('cell', { name: 'Not applicable' })
  ).toBeVisible()
  expect(details.getByRole('cell', { name: 'Not recorded' })).toBeVisible()
  expect(
    details.queryByRole('columnheader', { name: 'Upstream task ID' })
  ).toBeNull()
  await userEvent.click(
    details.getByRole('button', { name: 'Export and download' })
  )
  expect(mocks.exports).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(mocks.exports).toHaveBeenCalledWith(
      expect.objectContaining({
        job_type: 'upstream_details',
        evidence_filter: 'usage_gap',
      })
    )
  )
})

it('loads only the clicked URL and channel on entry to details', async () => {
  view()
  await screen.findByRole('table')
  expect(
    mocks.summary.mock.calls.some(([params]) => params.level === 'url_models')
  ).toBe(false)
  await openChannel(12)
  expect(
    await screen.findByText('customer - provider-model')
  ).toBeInTheDocument()
  expect(mocks.summary).toHaveBeenCalledWith(
    expect.objectContaining({
      level: 'url_models',
      url_key: upstreamUrl,
      channel_id: 12,
      report_version: version,
    }),
    expect.any(AbortSignal)
  )
})

it('opens records by channel when the log source does not publish daily reports', async () => {
  const publishedResponse = mocks.summary.getMockImplementation()
  if (!publishedResponse) {
    throw new Error('Expected an upstream summary fixture')
  }
  mocks.summary.mockImplementation(async (params) => {
    const response = await publishedResponse(params)
    delete response.data.billing_report
    return response
  })

  view()
  await openChannel(12)
  await userEvent.click(
    await screen.findByRole('button', { name: 'View records' })
  )

  const models = mocks.summary.mock.calls.find(
    ([params]) => params.level === 'url_models'
  )?.[0]
  expect(models).toMatchObject({ channel_id: 12 })
  expect(models.url_key).toBeUndefined()
  expect(models.report_version).toBeUndefined()
  await waitFor(() => {
    const request = mocks.details.mock.calls.at(-1)?.[0]
    expect(request).toMatchObject({
      channel_id: 12,
      model_name: 'provider-model',
    })
    expect(request.url_key).toBeUndefined()
    expect(request.report_version).toBeUndefined()
  })
})
it('opens the overview export without submitting and exports the full month explicitly', async () => {
  view()
  await screen.findByRole('table')
  await userEvent.click(
    screen.getByRole('button', { name: 'Export and download' })
  )
  const drawer = within(
    screen.getByRole('dialog', { name: 'Export and download' })
  )
  expect(drawer.getByText('All channels for the billing month')).toBeVisible()
  expect(mocks.exports).not.toHaveBeenCalled()
  await userEvent.click(drawer.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(mocks.exports).toHaveBeenCalledWith({
      job_type: 'upstream_details',
      report_version: version,
      all_channels: true,
      start_timestamp: period.start_timestamp,
      end_timestamp: period.end_timestamp + 1,
    })
  )
})

it('exports the clicked channel and immediately downloads a reusable file', async () => {
  const url = 'https://downloads.example/summary.csv'
  mocks.exports.mockResolvedValueOnce({
    job_id: 'reused-export',
    status: 'succeeded',
  })
  mocks.sign.mockResolvedValueOnce({ files: [{ url }], emptyResult: false })
  view()
  await openChannel(12)
  expect(screen.queryByRole('button', { name: 'Export summary' })).toBeNull()
  await userEvent.click(
    screen.getByRole('button', { name: 'Export and download' })
  )
  expect(mocks.exports).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() => expect(mocks.exports).toHaveBeenCalledTimes(1))
  expect(mocks.exports).toHaveBeenCalledWith({
    job_type: 'upstream_details',
    channel_id: 12,
    report_version: version,
    url_key: upstreamUrl,
    start_timestamp: period.start_timestamp,
    end_timestamp: period.end_timestamp + 1,
  })
  await waitFor(() => expect(mocks.download).toHaveBeenCalledWith(url))
  expect(mocks.sign).toHaveBeenCalledExactlyOnceWith('reused-export')
})

it('keeps export retryable after a failed submission', async () => {
  mocks.exports.mockRejectedValueOnce(new Error('storage unavailable'))
  view()
  await openChannel(11)
  expect(screen.queryByRole('button', { name: 'Export summary' })).toBeNull()
  await userEvent.click(
    screen.getByRole('button', { name: 'Export and download' })
  )
  expect(mocks.exports).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Export CSV' })).toBeEnabled()
  )
  await userEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() => expect(mocks.exports).toHaveBeenCalledTimes(2))
})
it('searches beyond the current page on the server', async () => {
  view()
  await screen.findByRole('table')
  await userEvent.type(
    screen.getByLabelText('Upstream URL or channel'),
    'deleted supplier'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(mocks.summary).toHaveBeenCalledWith(
      expect.objectContaining({
        level: 'url_channels',
        search: 'deleted supplier',
        page: 1,
      }),
      expect.any(AbortSignal)
    )
  )
  await userEvent.click(
    await screen.findByRole('button', {
      name: /3 of 12 records lack usage details/,
    })
  )
  expect(
    screen.getByText('Open a matching channel to view records for this search.')
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /^View records:/ })
  ).not.toBeInTheDocument()
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

it('opens the selected URL from the summary table and keeps its scope in drilldowns and exports', async () => {
  const url = 'https://billing.example/v1'
  const otherUrl = 'https://other.example/v1'
  const version = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
  mocks.summary.mockImplementation(async (params) => {
    if (params.level === 'url_models') {
      return {
        success: true,
        data: {
          result: {
            total: 1,
            model_rows: [
              {
                ...channel(11),
                provider_model: 'provider-model',
                billing_mode: 'token',
                customer_models: ['customer'],
                detail_filter: { channel_id: 11 },
                usage: { ...usage, completed_images: 2 },
              },
            ],
          },
        },
      }
    }
    return {
      success: true,
      data: {
        billing_report: {
          version,
          published_days: 1,
          expected_days: 30,
          as_of: 1,
        },
        result: {
          channels: [],
          models: [],
          total: 3,
          totals: {
            usage: {
              ...usage,
              requests: 30,
              seconds: '13',
              completed_images: 2,
            },
            known_original_amount: 400,
            known_reference_amount: 160,
          },
          url_channels: [
            {
              ...urlChannel(11, url),
              reference_amount: null,
              known_reference_amount: 80,
              usage: { ...usage, requests: 7 },
            },
            urlChannel(12, url),
            urlChannel(11, otherUrl),
          ],
        },
      },
    }
  })
  view()
  expect(await screen.findByText(/1 of 30 days published/)).toBeVisible()
  const total = screen
    .getByRole('heading', { name: 'Total' })
    .closest('section')
  expect(total).toHaveTextContent('Requests30')
  const metrics = total?.querySelectorAll('dl > div')
  expect(
    Array.from(
      metrics ?? [],
      (metric) => metric.querySelector('dt')?.textContent
    )
  ).toEqual([
    'Requests',
    'List price',
    'Discounted reference amount',
    'Input tokens',
    'Output tokens',
    'Images',
    'Billable seconds',
  ])
  expect(
    screen.getByText('List price', { selector: 'dt' }).nextElementSibling
  ).toHaveTextContent(formatCustomerStatementQuota(400))
  expect(
    screen.getByText('Discounted reference amount', { selector: 'dt' })
      .nextElementSibling
  ).toHaveTextContent(formatCustomerStatementQuota(160))
  expect(
    screen.getByText('Images', { selector: 'dt' }).nextElementSibling
  ).toHaveTextContent(/^2$/)
  expect(total).not.toHaveTextContent('Known subtotal')
  expect(
    screen.getByText('Billable seconds', { selector: 'dt' }).nextElementSibling
  ).toHaveTextContent('13')
  const table = screen.getByRole('table')
  expect(
    within(table).getByRole('columnheader', {
      name: 'Discounted reference amount',
    })
  ).toBeVisible()
  await userEvent.click(within(table).getByRole('button', { name: 'Requests' }))
  await waitFor(() =>
    expect(mocks.summary).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: 'requests', page: 1 }),
      expect.any(AbortSignal)
    )
  )
  const summary = within(await screen.findByRole('table'))
  const row = summary
    .getAllByRole('row', { name: /Channel 11/ })
    .find((candidate) => within(candidate).queryByRole('cell', { name: url }))
  if (!row) throw new Error('Expected selected URL and channel row')
  expect(row).toHaveTextContent('Unknown')
  expect(row).toHaveTextContent('Known subtotal:')
  const action = within(row).getByRole('button', { name: 'View details' })
  action.focus()
  await userEvent.keyboard('{Enter}')
  expect(
    screen.getByRole('heading', { name: 'Channel 11 · #11' })
  ).toBeVisible()
  const detailTable = await screen.findByRole('table')
  expect(within(detailTable).getByText(/Channel 11/)).toBeVisible()
  expect(screen.queryByText(/Channel 12/)).not.toBeInTheDocument()
  expect(within(detailTable).getAllByRole('row')).toHaveLength(2)
  expect(detailTable).toHaveTextContent('Images · Known subtotal: 2')
  expect(
    screen.queryByRole('button', { name: 'Expand models' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('combobox', { name: 'Channel' })
  ).not.toBeInTheDocument()
  await userEvent.selectOptions(
    screen.getByRole('combobox', { name: 'Data status' }),
    'partial'
  )
  await userEvent.type(
    screen.getByRole('textbox', { name: 'Provider model' }),
    'provider-model'
  )
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(mocks.summary).toHaveBeenCalledWith(
      expect.objectContaining({
        level: 'url_models',
        channel_id: 11,
        data_status: 'partial',
        search: 'provider-model',
        report_version: version,
        url_key: url,
      }),
      expect.any(AbortSignal)
    )
  )
  await userEvent.click(
    await screen.findByRole('button', { name: 'View records' })
  )
  await waitFor(() =>
    expect(mocks.details).toHaveBeenCalledWith(
      expect.objectContaining({
        channel_id: 11,
        report_version: version,
        url_key: url,
        model_name: 'provider-model',
        billing_mode: 'token',
      })
    )
  )
  await userEvent.click(screen.getByRole('button', { name: 'Close details' }))
  expect(screen.queryByRole('button', { name: 'Export summary' })).toBeNull()
  await userEvent.click(
    screen.getByRole('button', { name: 'Export and download' })
  )
  expect(mocks.exports).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  await waitFor(() =>
    expect(mocks.exports).toHaveBeenCalledWith(
      expect.objectContaining({
        channel_id: 11,
        report_version: version,
        url_key: url,
      })
    )
  )
  await userEvent.keyboard('{Escape}')
  await userEvent.click(screen.getByRole('button', { name: 'Back' }))
  expect(screen.getByRole('table')).toHaveTextContent(otherUrl)
  expect(
    screen.getByRole('columnheader', { name: 'Requests' })
  ).toHaveAttribute('aria-sort', 'descending')
  expect(screen.getAllByText('Channel 11')).toHaveLength(2)
})
