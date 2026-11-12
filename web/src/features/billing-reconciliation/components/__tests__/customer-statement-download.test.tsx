import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  cleanup,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { useState } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import type { CustomerExportJobView } from '../../export-api'
import { MyBilling } from '../../my-billing'
import type { BillingEnvelope, CustomerStatement } from '../../types'
import { CustomerStatementView } from '../customer-statement'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const period = { start_timestamp: 1788192000, end_timestamp: 1790783999 }
const usage = {
  requests: 2,
  billable_calls: 0,
  refunded_calls: 0,
  input_tokens: 1000,
  cache_read_tokens: 900,
  cache_write_tokens: 0,
  output_tokens: 20,
  gross_quota: 100,
  refund_quota: 180,
  net_quota: -80,
}
function statementEnvelope(): BillingEnvelope<CustomerStatement> {
  return {
    period: {
      ...period,
      period_start: period.start_timestamp,
      timezone: 'Asia/Shanghai',
    },
    generated_at: 1789488000,
    data_version: period.end_timestamp,
    data_source: 'main_database+log_database',
    filters: {},
    result: {
      recorded_original_amounts: true,
      user_id: 7,
      username: 'customer',
      display_name: '',
      dimension: 'api_key',
      current_balance: 123,
      summary: { ...usage },
      original_quota: -100,
      discount_quota: -20,
      data_quality: { status: 'partial', input_tokens_unavailable_requests: 1 },
      groups: [
        {
          id: 4,
          name: 'key-a',
          usage,
          original_quota: -100,
          discount_quota: -20,
          models: [
            {
              model_name: 'public-a',
              billing_mode: 'token',
              usage,
              original_quota: -100,
              discount_ratio: 1,
              contract_discount_ratio: 0.8,
              price_versions: 1,
              detail_filter: { ...period, token_id: 4 },
            },
          ],
        },
      ],
    },
  }
}
const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
  vi.clearAllMocks()
  vi.mocked(api.get).mockReset()
})

function MyBillingWithFilter() {
  const [modelKeyId, setModelKeyId] = useState<number>()
  return (
    <MyBilling
      month='2026-09'
      onMonthChange={vi.fn()}
      modelKeyId={modelKeyId}
      onModelKeyChange={setModelKeyId}
    />
  )
}

async function mount(
  options: {
    envelope?: BillingEnvelope<CustomerStatement>
    isAdmin?: boolean
    userId?: number
    selfPage?: boolean
    language?: string
  } = {}
) {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({
    lng: options.language ?? 'en',
    nsSeparator: false,
    resources: { en: { translation: {} }, zh },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  clients.push(client)
  if (options.envelope) {
    client.setQueryData(
      [
        'billing-customer-reconciliation',
        options.isAdmin === true,
        options.userId,
        'api_key',
        period.start_timestamp,
        period.end_timestamp,
        null, // No selected statement version.
      ],
      options.envelope
    )
  }
  const queuedExport: CustomerExportJobView = {
    job_id: 'cex_test',
    user_id: options.isAdmin ? 1 : 7,
    target_user_id: options.userId ?? 7,
    job_type: 'statement_summary',
    status: 'queued',
    filters: { ...period, timezone: 'Asia/Shanghai', field_version: 21 },
    progress: { scanned: 0, matched: 0, written: 0, files: 0 },
    cancel_requested: false,
    created_at: period.start_timestamp,
  }
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: queuedExport },
  })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        {options.selfPage ? (
          <MyBillingWithFilter />
        ) : (
          <CustomerStatementView
            isAdmin={options.isAdmin === true}
            userId={options.userId}
            dimension='api_key'
            period={period}
            onDimensionChange={vi.fn()}
          />
        )}
      </QueryClientProvider>
    </I18nextProvider>
  )
  return { view, i18n, client, queuedExport }
}

async function openExportDrawer() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Export and download' }))
  return screen.findByRole('dialog', { name: 'Export and download' })
}

it('submits a server-side statement export with the frozen month and language', async () => {
  await mount({ envelope: statementEnvelope() })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(api.post).not.toHaveBeenCalled()
  const exportCSV = await screen.findByRole('button', { name: 'Export CSV' })
  await user.click(exportCSV)
  await vi.waitFor(() => expect(api.post).toHaveBeenCalled())
  expect(api.post).toHaveBeenCalledWith('/api/billing/exports', {
    job_type: 'statement_summary',
    start_timestamp: period.start_timestamp,
    end_timestamp: period.end_timestamp + 1,
    language: 'en',
  })
  expect(
    screen.getByRole('dialog', { name: 'Export and download' })
  ).toBeVisible()
})

it('uses the shared drawer and submits admin exports for the selected customer', async () => {
  await mount({ envelope: statementEnvelope(), isAdmin: true, userId: 7 })
  const drawer = await openExportDrawer()
  expect(within(drawer).getByText('customer · #7')).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
  fireEvent.click(within(drawer).getByRole('button', { name: 'Export CSV' }))
  await vi.waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/billing/admin/customer-exports',
      {
        job_type: 'statement_summary',
        start_timestamp: period.start_timestamp,
        end_timestamp: period.end_timestamp + 1,
        language: 'en',
      },
      { params: { user_id: 7 } }
    )
  )
  expect(
    await within(drawer).findByRole('heading', { name: 'This export' })
  ).toBeVisible()
  const user = userEvent.setup()
  await user.click(
    within(drawer).getByRole('combobox', { name: 'Export content' })
  )
  await user.click(await screen.findByRole('option', { name: 'Usage records' }))
  await user.click(within(drawer).getByRole('button', { name: 'Export CSV' }))
  await vi.waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/billing/admin/customer-exports',
      expect.objectContaining({ job_type: 'usage_logs' }),
      { params: { user_id: 7 } }
    )
  )
})

it('does not offer usage-record export for the customer view', async () => {
  await mount({ envelope: statementEnvelope() })
  const drawer = await openExportDrawer()
  await userEvent.click(
    within(drawer).getByRole('combobox', { name: 'Export content' })
  )
  expect(
    await screen.findByRole('option', { name: 'Statement summary' })
  ).toBeVisible()
  expect(screen.queryByRole('option', { name: 'Usage records' })).toBeNull()
})

it('groups month and export controls with the My billing heading and status above the totals', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { versions: [] },
    },
  })
  const envelope = {
    ...statementEnvelope(),
    billing_report: {
      version: 'report-september',
      published_days: 29,
      incomplete_days: 1,
      expected_days: 30,
      estimated: true,
      as_of: period.end_timestamp,
    },
  }
  await mount({ envelope, selfPage: true })

  const heading = screen.getByRole('heading', {
    name: /^My billing$/,
  })
  const header = heading.closest('header')
  if (!header) throw new Error('Missing billing header')
  const pageHeader = within(header)
  expect(pageHeader.getByLabelText('Billing month')).toBeTruthy()
  expect(
    pageHeader.getByRole('button', { name: 'Export and download' })
  ).toBeTruthy()
  expect(pageHeader.queryByRole('button', { name: 'Export jobs' })).toBeNull()
  expect(
    await pageHeader.findByRole('button', { name: /Version history/ })
  ).toBeTruthy()
  expect(pageHeader.getByText('Database authoritative')).toBeTruthy()
  expect(
    pageHeader.getByText(
      /Partial data.*29 of 30 days published.*Days with incomplete data · 1/
    )
  ).toBeTruthy()
  expect(pageHeader.getByText('Incomplete usage details')).toBeTruthy()
  expect(pageHeader.getByText(/^Data as of/)).toBeTruthy()
  expect(pageHeader.queryByText(/Estimated usage or pricing/)).toBeNull()
  expect(pageHeader.queryByText('Current balance')).toBeNull()

  const historyButton = await pageHeader.findByRole('button', {
    name: 'Version history (0)',
  })
  const user = userEvent.setup()
  await user.click(historyButton)
  const history = screen.getByRole('dialog', { name: 'Version history' })
  expect(within(history).getByText('No version history')).toBeVisible()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('dialog', { name: 'Version history' })).toBeNull()
  expect(historyButton).toHaveFocus()
})

it('keeps the net charge visible when an older server supplies reverse-calculated amounts', async () => {
  const envelope = statementEnvelope()
  Object.assign(envelope.result, { recorded_original_amounts: undefined })
  await mount({ envelope, selfPage: true })

  expect(screen.getAllByText('Not recorded').length).toBeGreaterThanOrEqual(2)
  expect(screen.queryByText('-$0.0002')).toBeNull()
  expect(screen.getAllByText('-$0.00016').length).toBeGreaterThan(0)
  const modelBill = screen.getByRole('region', { name: 'Model billing' })
  expect(
    within(modelBill).queryByRole('columnheader', { name: 'List price' })
  ).toBeNull()
  expect(
    within(modelBill).getByRole('columnheader', { name: 'Net settled amount' })
  ).toBeVisible()
  const summary = within(
    screen.getByRole('table', { name: 'Billing by API Key' })
  )
  const total = within(summary.getByRole('row', { name: /^Total/ }))
  expect(total.getAllByRole('cell', { name: 'Not recorded' })).toHaveLength(2)
  expect(total.getByRole('cell', { name: '-$0.00016' })).toBeVisible()
})

it('shows historical contract and group discounts per model even without recorded prices', async () => {
  const envelope = statementEnvelope()
  envelope.result.recorded_original_amounts = undefined
  const models = envelope.result.groups[0].models
  models[0].discount_ratio = 0.5
  models.push({
    ...models[0],
    model_name: 'public-b',
    multiple_contract_discounts: true,
    discount_ratio: undefined,
  })
  await mount({ envelope, selfPage: true, language: 'zh' })

  const modelBill = within(screen.getByRole('region', { name: '模型账单' }))
  expect(
    modelBill.getByRole('columnheader', { name: '合同折扣' })
  ).toBeVisible()
  expect(
    modelBill.getByRole('columnheader', { name: '分组折扣/倍率' })
  ).toBeVisible()
  const single = within(modelBill.getByRole('row', { name: /public-a/ }))
  expect(single.getByRole('cell', { name: /8折.*×0.8/ })).toBeVisible()
  expect(single.getByRole('cell', { name: /5折.*×0.5/ })).toBeVisible()
  const mixed = within(modelBill.getByRole('row', { name: /public-b/ }))
  expect(mixed.getByRole('cell', { name: '多种折扣' })).toBeVisible()
  expect(mixed.getByRole('cell', { name: '未记录' })).toBeVisible()
})

it('switches model billing from the API key name or any cell in its row', async () => {
  const envelope = statementEnvelope()
  const group = envelope.result.groups[0]
  envelope.result.groups.push({
    ...group,
    id: 5,
    name: 'key-b',
    models: [{ ...group.models[0], model_name: 'public-b' }],
  })
  await mount({ envelope, selfPage: true })
  const user = userEvent.setup()
  const modelBill = screen.getByRole('region', { name: 'Model billing' })
  expect(within(modelBill).getByText('public-a')).toBeVisible()
  expect(within(modelBill).getByText('public-b')).toBeVisible()

  const summary = within(
    screen.getByRole('table', { name: 'Billing by API Key' })
  )
  expect(summary.queryByRole('columnheader', { name: 'Actions' })).toBeNull()
  const trigger = summary.getByRole('button', { name: 'key-a #4' })
  expect(trigger).toHaveAttribute('aria-pressed', 'false')
  trigger.focus()
  await user.keyboard('{Enter}')

  expect(screen.queryByRole('dialog')).toBeNull()
  expect(trigger).toHaveAttribute('aria-pressed', 'true')
  const modelHeading = within(modelBill).getByRole('heading')
  expect(modelHeading).toHaveFocus()
  const headingRow = modelHeading.parentElement
  if (!headingRow) throw new Error('Missing model billing heading row')
  expect(headingRow).toHaveClass('flex', 'flex-wrap', 'items-center')
  expect(within(headingRow).getByText(/key-a #4/)).toBeVisible()
  expect(
    within(headingRow).getByRole('button', { name: 'Clear filters' })
  ).toBeVisible()
  expect(within(modelBill).queryByText('public-b')).toBeNull()
  const detailHref = within(modelBill)
    .getByRole('button', { name: 'View details' })
    .getAttribute('href')
  const detailSearch = new URL(detailHref ?? '', 'http://localhost')
    .searchParams
  expect(detailSearch.get('billingModelKeyId')).toBe('4')

  const otherSummaryRow = summary.getByRole('row', { name: /key-b/ })
  await user.click(within(otherSummaryRow).getByRole('cell', { name: '2' }))
  expect(within(modelBill).getByText(/key-b #5/)).toBeVisible()
  expect(within(modelBill).getByText('public-b')).toBeVisible()
  expect(within(modelBill).queryByText('public-a')).toBeNull()
  expect(
    within(otherSummaryRow).getByRole('button', { name: 'key-b #5' })
  ).toHaveAttribute('aria-pressed', 'true')

  await user.click(
    within(modelBill).getByRole('button', { name: 'Clear filters' })
  )
  expect(within(modelBill).getByRole('heading')).toHaveFocus()
  expect(within(modelBill).getByText('public-a')).toBeVisible()
  expect(within(modelBill).getByText('public-b')).toBeVisible()
})

it('keeps the API key summary total for the full bill when model billing is filtered', async () => {
  const envelope = statementEnvelope()
  const group = envelope.result.groups[0]
  envelope.result.groups.push({
    ...group,
    id: 5,
    name: 'key-b',
    models: [{ ...group.models[0], model_name: 'public-b' }],
  })
  envelope.result.summary = { ...usage, requests: 4, net_quota: -160 }
  envelope.result.original_quota = -200
  envelope.result.discount_quota = -40
  await mount({ envelope, selfPage: true })

  const summary = within(
    screen.getByRole('table', { name: 'Billing by API Key' })
  )
  const row = summary.getByRole('row', { name: /key-b/ })
  await userEvent.click(within(row).getByRole('cell', { name: '2' }))

  const total = within(summary.getByRole('row', { name: /^Total/ }))
  expect(total.getByRole('rowheader', { name: 'Total' })).toBeVisible()
  expect(total.getAllByRole('cell').map((cell) => cell.textContent)).toEqual([
    '4',
    '-$0.0004',
    '-$0.00008',
    '-$0.00032',
  ])
})

it('shows model totals and details directly on the bill for a single API key', async () => {
  const envelope = statementEnvelope()
  const model = envelope.result.groups[0].models[0]
  model.original_quota = model.usage.net_quota
  model.discount_quota = 0
  envelope.billing_report = {
    version: 'published-report',
    as_of: period.end_timestamp,
    published_days: 30,
    expected_days: 30,
  }
  await mount({ envelope, selfPage: true })

  const modelBill = screen.getByRole('region', { name: 'Model billing' })
  const row = within(modelBill).getByRole('row', { name: /public-a/ })
  expect(within(row).getByText('$0', { exact: true })).toBeVisible()
  expect(within(row).getAllByText('-$0.00016')).toHaveLength(2)
  const action = within(row).getByRole('button', { name: 'View details' })
  const params = new URL(action.getAttribute('href') ?? '', 'http://localhost')
    .searchParams
  expect(params.get('model')).toBe('public-a')
  expect(params.get('tokenId')).toBe('4')
  expect(params.get('billingReportVersion')).toBe('published-report')
  expect(params.get('billingView')).toBe('events')
  expect(params.has('billingModelKeyId')).toBe(false)
  expect(screen.queryByRole('button', { name: 'View models' })).toBeNull()
  const summary = within(
    screen.getByRole('table', { name: 'Billing by API Key' })
  )
  const keyRow = within(summary.getByRole('row', { name: /key-a/ }))
  await userEvent.click(keyRow.getByRole('cell', { name: '2' }))
  expect(within(modelBill).getByText(/key-a #4/)).toBeVisible()
  expect(within(modelBill).getByRole('heading')).toHaveFocus()
  expect(screen.queryByText('Discount breakdown')).toBeNull()
})

it.each([false, true])(
  'downloads the displayed version without creating a new export (admin: %s)',
  async (isAdmin) => {
    const versionPath = isAdmin
      ? '/api/billing/admin/customer-statement-versions'
      : '/api/billing/statement/self/versions'
    const envelope = {
      ...statementEnvelope(),
      billing_version: {
        id: 3,
        draft_public_id: 'frozen-version',
        status: 'confirmed',
        version_number: 1,
        user_id: 7,
        period_start: period.start_timestamp,
        period_end_exclusive: period.end_timestamp + 1,
        confirmed_at: 1791000000,
        created_at: 1791000000,
        updated_at: 1791000000,
        public_reason: '',
        corrects_version_id: null,
        quota_per_unit: 500000,
        currency: 'USD',
        currency_rate: 1,
      },
    }
    vi.mocked(api.get).mockImplementation(async (url) => {
      let data: unknown = { url: 'https://example.test/file' }
      if (url.endsWith('version-month-status')) {
        data = {
          current_version: envelope.billing_version,
          versions: [envelope.billing_version],
        }
      }
      if (url === '/api/billing/exports') data = { items: [], total: 0 }
      return { data: { success: true, data } }
    })
    await mount({
      envelope,
      isAdmin,
      userId: isAdmin ? 7 : undefined,
      selfPage: !isAdmin,
    })
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {})
    const buttons = screen.getAllByRole('button', {
      name: 'Export and download',
    })
    expect(buttons).toHaveLength(1)
    expect(
      screen.queryByRole('button', { name: 'Download confirmed version' })
    ).toBeNull()
    fireEvent.click(buttons[0])
    expect(api.post).not.toHaveBeenCalled()
    const drawer = screen.getByRole('dialog', { name: 'Export and download' })
    expect(
      within(drawer).getByRole('heading', {
        name: 'September 2026 statement · v1',
      })
    ).toBeTruthy()
    const user = userEvent.setup()
    await user.click(
      within(drawer).getByRole('combobox', { name: 'Export content' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Statement details' })
    )
    fireEvent.click(
      within(drawer).getByRole('button', {
        name: 'Download CSV',
      })
    )
    await vi.waitFor(() =>
      expect(api.get).toHaveBeenCalledWith(
        `${versionPath}/frozen-version/download`,
        { params: { role: 'detail_csv' } }
      )
    )
    await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(1))
    expect(api.post).not.toHaveBeenCalled()
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: /Version history/ }))
    const versions = screen.getByRole('dialog', { name: 'Version history' })
    expect(
      within(versions).getByRole('button', {
        name: 'Download confirmation note',
      })
    ).toBeTruthy()
  }
)

it('opens and closes the export drawer without submitting a task', async () => {
  await mount({ envelope: statementEnvelope(), selfPage: true })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(
    screen.getByRole('dialog', { name: 'Export and download' })
  ).toBeTruthy()
  await userEvent.keyboard('{Escape}')
  expect(api.post).not.toHaveBeenCalled()
  expect(
    screen.queryByRole('dialog', { name: 'Export and download' })
  ).toBeNull()
})

it.each(['loading', 'failed'])(
  'keeps export history downloadable while the statement is %s',
  async (status) => {
    const expiresAt = Math.floor(Date.now() / 1000) + 600
    const file = {
      file_name: 'statement-summary.csv',
      url: 'https://example.test/statement-summary.csv',
      object_key: '',
      size_bytes: 100,
      line_count: 1,
      sha256: '',
      expires_at: expiresAt,
    }
    const job: CustomerExportJobView = {
      job_id: 'cex_existing',
      user_id: 7,
      target_user_id: 7,
      job_type: 'statement_summary',
      status: 'succeeded',
      filters: { ...period, timezone: 'Asia/Shanghai', field_version: 21 },
      progress: { scanned: 1, matched: 1, written: 1, files: 1 },
      cancel_requested: false,
      artifact: {
        files: [file],
        line_count: 1,
        size_bytes: 100,
        generated_at: period.start_timestamp,
        expires_at: expiresAt,
      },
      created_at: period.start_timestamp,
    }
    vi.mocked(api.get).mockImplementation(async (url) => {
      if (url === '/api/billing/statement/self') {
        if (status === 'loading') return new Promise(() => {})
        throw new Error('Statement request failed')
      }
      let data: unknown = { versions: [] }
      if (url === '/api/billing/exports') data = { items: [job], total: 1 }
      if (url === '/api/billing/exports/cex_existing/download') {
        data = { files: [file] }
      }
      return { data: { success: true, data } }
    })
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => {})
    await mount({ selfPage: true })
    if (status === 'failed') await screen.findByText('Statement request failed')
    fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
    const drawer = screen.getByRole('dialog', { name: 'Export and download' })
    expect(
      within(drawer).getByRole('button', { name: 'Export CSV' })
    ).toBeDisabled()
    fireEvent.click(
      await within(drawer).findByRole('button', { name: 'Download CSV' })
    )
    await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(1))
    expect(api.post).not.toHaveBeenCalled()
  }
)

it('pins the submitted export above history and clears the pin when history is reopened', async () => {
  const job: CustomerExportJobView = {
    job_id: 'cex_current',
    user_id: 7,
    target_user_id: 7,
    job_type: 'statement_summary',
    status: 'queued',
    filters: {
      ...period,
      end_timestamp: period.end_timestamp + 1,
      timezone: 'Asia/Shanghai',
      field_version: 21,
    },
    progress: { scanned: 0, matched: 0, written: 0, files: 0 },
    cancel_requested: false,
    created_at: period.start_timestamp,
  }
  vi.mocked(api.get).mockImplementation(async (url) => {
    let data: unknown = { versions: [] }
    if (url === '/api/billing/exports') data = { items: [job], total: 1 }
    if (url === '/api/billing/exports/cex_current') data = job
    return { data: { success: true, data } }
  })
  await mount({ envelope: statementEnvelope(), selfPage: true })
  vi.mocked(api.post).mockResolvedValueOnce({
    data: { success: true, data: job },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  const drawer = screen.getByRole('dialog', { name: 'Export and download' })
  expect(
    within(drawer).getByRole('heading', { name: 'September 2026 statement' })
  ).toBeTruthy()
  fireEvent.click(
    within(drawer).getByRole('button', {
      name: 'Export CSV',
    })
  )

  const current = await screen.findByRole('heading', { name: 'This export' })
  const section = current.closest('section')
  if (!section) throw new Error('Missing current export section')
  expect(within(section).getByRole('article')).toBeTruthy()
  expect(screen.getAllByRole('article')).toHaveLength(1)

  await userEvent.keyboard('{Escape}')
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(await screen.findByRole('article')).toBeTruthy()
  expect(screen.queryByRole('heading', { name: 'This export' })).toBeNull()
})

it('downloads a reusable single file from My billing and keeps the drawer open', async () => {
  const url = 'https://example.test/bill.csv'
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, 'click')
    .mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.href).toBe(url)
    })
  vi.mocked(api.get).mockImplementation(async (endpoint) => {
    let data: unknown = { versions: [] }
    if (endpoint === '/api/billing/exports') data = { items: [], total: 0 }
    if (endpoint === '/api/billing/exports/cex_ready/download') {
      data = { files: [{ url }] }
    }
    return { data: { success: true, data } }
  })
  await mount({ envelope: statementEnvelope(), selfPage: true })
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: { job_id: 'cex_ready', status: 'succeeded' } },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  const drawer = screen.getByRole('dialog', { name: 'Export and download' })
  fireEvent.click(within(drawer).getByRole('button', { name: 'Export CSV' }))
  await vi.waitFor(() => expect(click).toHaveBeenCalledTimes(1))
  expect(drawer).toBeVisible()
  expect(api.post).toHaveBeenCalledTimes(1)
  expect(api.get).toHaveBeenCalledWith(
    '/api/billing/exports/cex_ready/download'
  )
})

it('lets the customer choose statement details in the export drawer', async () => {
  await mount({ envelope: statementEnvelope() })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Export content' }))
  await user.click(
    await screen.findByRole('option', { name: 'Statement details' })
  )
  fireEvent.click(screen.getByRole('button', { name: 'Export CSV' }))
  await vi.waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/billing/exports',
      expect.objectContaining({ job_type: 'statement_details' })
    )
  )
})

it('keeps the drawer available for retry after a submission failure', async () => {
  await mount({ envelope: statementEnvelope(), isAdmin: true, userId: 7 })
  vi.mocked(api.post).mockRejectedValueOnce(
    new Error('Export service is busy.')
  )
  const drawer = await openExportDrawer()
  fireEvent.click(within(drawer).getByRole('button', { name: 'Export CSV' }))
  await vi.waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith('Export service is busy.')
  )
  expect(
    within(drawer).getByRole('button', { name: 'Export CSV' })
  ).toBeEnabled()
})

it('disables export while the export submission is pending', async () => {
  const { queuedExport } = await mount({
    envelope: statementEnvelope(),
    isAdmin: true,
    userId: 7,
  })
  let complete: (value: unknown) => void = () => undefined
  vi.mocked(api.post).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve
      })
  )
  const drawer = await openExportDrawer()
  const submit = within(drawer).getByRole('button', { name: 'Export CSV' })
  fireEvent.click(submit)
  await vi.waitFor(() => expect(submit).toBeDisabled())
  fireEvent.click(submit)
  expect(api.post).toHaveBeenCalledTimes(1)
  complete({
    data: { success: true, data: queuedExport },
  })
  expect(
    await within(drawer).findByRole('heading', { name: 'This export' })
  ).toBeVisible()
})

it('shows recorded savings amounts for model bill rows instead of discount factors', async () => {
  const envelope = statementEnvelope()
  const model = envelope.result.groups[0].models[0]
  Object.assign(model, { discount_quota: 0 })
  model.original_quota = model.usage.net_quota
  await mount({ envelope })
  const row = screen.getByText('public-a').closest('tr')
  if (!row) throw new Error('Model row missing')
  const cells = within(row).getAllByRole('cell')
  expect(cells[7]).toHaveTextContent('$0')
  expect(cells[7]).not.toHaveTextContent('×')
  expect(
    within(screen.getByRole('region', { name: 'Model billing' })).getByRole(
      'columnheader',
      { name: 'Savings' }
    )
  ).toBeTruthy()
})

it('keeps admin model discounts, net amounts and detail links without standalone breakdowns', async () => {
  const envelope = statementEnvelope()
  const knownUsage = {
    ...usage,
    gross_quota: 500000,
    refund_quota: 0,
    net_quota: 500000,
  }
  Object.assign(envelope.result, {
    summary: knownUsage,
    original_quota: 500000,
    discount_quota: 0,
    data_quality: { status: 'complete' },
    estimate_reasons: [],
  })
  const group = envelope.result.groups[0]
  Object.assign(group, {
    usage: knownUsage,
    original_quota: 500000,
    discount_quota: 0,
    estimate_reasons: [],
  })
  Object.assign(group.models[0], {
    usage: knownUsage,
    original_quota: 500000,
    discount_quota: 0,
    discount_ratio: 1,
    contract_discount_ratio: undefined,
    estimate_reasons: [],
  })
  envelope.result.discount_combinations = [
    {
      group_id: 4,
      model_name: 'public-a',
      billing_mode: 'token',
      group_ratio: 1,
      contract_applicable: 'unrecorded',
      usage: knownUsage,
      original_known: true,
      original_quota: 500000,
      discount_quota: 0,
    },
  ]
  await mount({ envelope, isAdmin: true, userId: 7 })
  expect(screen.queryByRole('button', { name: 'Fund movements' })).toBeNull()
  expect(
    screen.queryByRole('button', { name: 'Discount breakdown' })
  ).toBeNull()
  expect(
    within(
      screen.getByRole('table', { name: 'Billing by API Key' })
    ).getAllByText('$0', { exact: true })
  ).toHaveLength(2)
  const modelBill = within(
    screen.getByRole('region', { name: 'Model billing' })
  )
  expect(modelBill.getAllByText('$0', { exact: true })).toHaveLength(2)
  const row = within(modelBill.getByRole('row', { name: /public-a/ }))
  expect(row.getByText('×1')).toBeVisible()
  const netColumn = modelBill
    .getAllByRole('columnheader')
    .indexOf(
      modelBill.getByRole('columnheader', { name: 'Net settled amount' })
    )
  expect(row.getAllByRole('cell')[netColumn]).toHaveTextContent('$1')
  const href = row
    .getByRole('button', { name: 'View details' })
    .getAttribute('href')
  const details = new URL(href ?? '', 'http://localhost').searchParams
  expect(details.get('billingUserId')).toBe('7')
  expect(details.get('model')).toBe('public-a')
  expect(details.get('tokenId')).toBe('4')
})

it('explains summary aggregation and per-call billing events before export', async () => {
  await mount({ envelope: statementEnvelope(), isAdmin: true, userId: 7 })
  await openExportDrawer()
  expect(
    await screen.findByText(
      'Totals grouped by API Key, model, and billing mode.'
    )
  ).toBeTruthy()
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Export content' }))
  await user.click(
    await screen.findByRole('option', { name: 'Statement details' })
  )
  expect(
    await screen.findByText(
      'Per-call billing records: time, request ID, usage, amounts and discount factors. Charges and refunds are separate rows; refunds have negative net amounts.'
    )
  ).toBeTruthy()
})
