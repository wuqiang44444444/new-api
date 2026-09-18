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
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

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
})

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
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: { job_id: 'cex_test' } },
  })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        {options.selfPage ? (
          <MyBilling month='2026-09' onMonthChange={vi.fn()} />
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
  return { view, i18n, client }
}

it('submits a server-side statement export with the frozen month and language', async () => {
  await mount({ envelope: statementEnvelope() })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(api.post).not.toHaveBeenCalled()
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Confirm export and download',
    })
  )
  await vi.waitFor(() => expect(api.post).toHaveBeenCalled())
  expect(api.post).toHaveBeenCalledWith('/api/billing/exports', {
    job_type: 'statement_summary',
    start_timestamp: period.start_timestamp,
    end_timestamp: period.end_timestamp + 1,
    language: 'en',
  })
  expect(toast.success).toHaveBeenCalled()
})

it('lets the admin download the selected customer statement and export usage records', async () => {
  await mount({ envelope: statementEnvelope(), isAdmin: true, userId: 7 })
  expect(
    screen.getByRole('button', { name: 'Export usage records' })
  ).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(api.post).not.toHaveBeenCalled()
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Confirm export and download',
    })
  )
  await vi.waitFor(() => expect(api.post).toHaveBeenCalled())
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
})

it('does not offer usage-record export for the customer view', async () => {
  await mount({ envelope: statementEnvelope() })
  expect(
    screen.queryByRole('button', { name: 'Export usage records' })
  ).toBeNull()
})

it('renders discount combinations with tier and factor and reconciling totals', async () => {
  const envelope = statementEnvelope()
  envelope.result.discount_combinations = [
    {
      group_id: 4,
      model_name: 'public-a',
      billing_mode: 'token',
      group_ratio: 0.5,
      contract_applicable: 'yes',
      contract_name: 'annual',
      contract_id_known: true,
      contract_id: 5,
      contract_version: 2,
      contract_ratio: 0.3,
      usage: { ...usage, net_quota: -80 },
      original_known: true,
      original_quota: -533,
      discount_quota: -453,
    },
    {
      group_id: 4,
      model_name: 'public-a',
      billing_mode: 'token',
      group_ratio: 0.5,
      contract_applicable: 'no',
      usage: { ...usage, net_quota: 0 },
      original_known: false,
    },
  ]
  await mount({ envelope })
  expect(screen.getByText('Discount breakdown')).toBeTruthy()
  expect(screen.getByText('annual')).toBeTruthy()
  // 组合原价未知时显示不可用，不冒充完整金额。
  expect(
    screen.getAllByText(
      'Cannot estimate: reason not recorded in this historical statement'
    ).length
  ).toBeGreaterThanOrEqual(1)
  expect(screen.getByText('85% off')).toBeTruthy()
  expect(screen.getByText('×0.15')).toBeTruthy()
  expect(screen.getAllByText('50% off').length).toBeGreaterThanOrEqual(2)
})

it('keeps equal-ratio billing groups and exclusive sources visible as separate rows', async () => {
  const envelope = statementEnvelope()
  envelope.result.discount_combinations = [
    { group_name: 'historical-a', group_ratio_source: 'group' as const },
    {
      group_name: 'historical-b',
      group_ratio_source: 'user_exclusive' as const,
    },
  ].map((group) => ({
    ...group,
    group_id: 4,
    model_name: 'public-a',
    billing_mode: 'token',
    group_ratio: 0.5,
    contract_applicable: 'no' as const,
    usage,
    original_known: false,
  }))
  await mount({ envelope })
  expect(screen.getByText('historical-a')).toBeTruthy()
  expect(screen.getByText('historical-b')).toBeTruthy()
  expect(screen.getByText('Group Ratio')).toBeTruthy()
  expect(screen.getByText('User Exclusive Ratio')).toBeTruthy()
})

it('offers only the version-aware statement download on My billing', async () => {
  vi.spyOn(window, 'open').mockImplementation(() => null)
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
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { url: 'https://example.test/file' } },
  })
  await mount({ envelope, selfPage: true })
  const buttons = screen.getAllByRole('button', { name: 'Export and download' })
  expect(buttons).toHaveLength(1)
  fireEvent.click(buttons[0])
  expect(api.post).not.toHaveBeenCalled()
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Confirm export and download',
    })
  )
  await vi.waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/billing/statement/self/versions/frozen-version/download',
      expect.anything()
    )
  )
  expect(api.post).not.toHaveBeenCalled()
})

it('cancels the export confirmation without submitting a task', async () => {
  await mount({ envelope: statementEnvelope(), selfPage: true })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Cancel',
    })
  )
  expect(api.post).not.toHaveBeenCalled()
  expect(screen.getAllByRole('button', { name: 'Export jobs' })).toHaveLength(1)
  expect(
    screen.queryByRole('button', { name: 'Export statement details' })
  ).toBeNull()
})

it('downloads a completed reusable export without submitting another task', async () => {
  vi.spyOn(window, 'open').mockImplementation(() => null)
  await mount({ envelope: statementEnvelope() })
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: { job_id: 'cex_ready', status: 'succeeded' } },
  })
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { files: [{ url: 'https://example.test/bill.csv' }] },
    },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Confirm export and download',
    })
  )
  await vi.waitFor(() =>
    expect(window.open).toHaveBeenCalledWith(
      'https://example.test/bill.csv',
      '_blank',
      'noopener'
    )
  )
  expect(api.post).toHaveBeenCalledTimes(1)
  expect(api.get).toHaveBeenCalledWith(
    '/api/billing/exports/cex_ready/download'
  )
})

it('lets the customer choose statement details within the combined confirmation', async () => {
  await mount({ envelope: statementEnvelope() })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  const user = userEvent.setup()
  await user.click(screen.getByRole('combobox', { name: 'Export content' }))
  await user.click(
    await screen.findByRole('option', { name: 'Statement details' })
  )
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Confirm export and download',
    })
  )
  await vi.waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/billing/exports',
      expect.objectContaining({ job_type: 'statement_details' })
    )
  )
})

it('keeps the confirmation available for retry after a submission failure', async () => {
  await mount({ envelope: statementEnvelope() })
  vi.mocked(api.post).mockRejectedValueOnce(
    new Error('Export service is busy.')
  )
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  fireEvent.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Confirm export and download',
    })
  )
  await vi.waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith('Export service is busy.')
  )
  expect(
    within(screen.getByRole('alertdialog'))
      .getByRole('button', { name: 'Confirm export and download' })
      .hasAttribute('disabled')
  ).toBe(false)
})

it('disables confirmation while the export submission is pending', async () => {
  await mount({ envelope: statementEnvelope() })
  let complete: (value: unknown) => void = () => undefined
  vi.mocked(api.post).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve
      })
  )
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  const confirm = within(screen.getByRole('alertdialog')).getByRole('button', {
    name: 'Confirm export and download',
  })
  fireEvent.click(confirm)
  await vi.waitFor(() => expect(confirm.hasAttribute('disabled')).toBe(true))
  fireEvent.click(confirm)
  expect(api.post).toHaveBeenCalledTimes(1)
  complete({
    data: { success: true, data: { job_id: 'cex_pending', status: 'queued' } },
  })
  await vi.waitFor(() => expect(toast.success).toHaveBeenCalled())
})

it('explains missing historical contract facts instead of showing unavailable amounts', async () => {
  const envelope = statementEnvelope()
  envelope.result.discount_combinations = [
    {
      group_id: 4,
      model_name: 'public-a',
      billing_mode: 'token',
      group_ratio: 1,
      contract_applicable: 'unknown',
      usage,
      original_known: false,
    },
  ]
  Object.assign(envelope.result.discount_combinations[0], {
    estimate_reasons: ['missing_contract'],
  })
  await mount({ envelope })
  expect(
    screen.getAllByText(
      'Cannot estimate: historical contract status not recorded'
    ).length
  ).toBe(2)
  expect(
    screen.getByText('Unknown: historical contract status not recorded')
  ).toBeTruthy()
  expect(screen.queryByText('Unavailable')).toBeNull()
})

it.each([
  ['missing_contract', '无法估算：历史合同状态未记录'],
  ['missing_group', '无法估算：历史分组倍率未记录'],
  ['invalid_facts', '估算失败：计费记录异常'],
  ['auxiliary_charge', '无法估算：附加费历史换算依据未核实'],
  ['amount_out_of_range', '估算失败：金额超出范围'],
  ['combination_limit', '无法估算：合并行缺少折扣明细'],
])(
  'shows the specific Chinese estimate reason for %s',
  async (reason, message) => {
    const envelope = statementEnvelope()
    envelope.result.discount_combinations = [
      {
        group_id: 4,
        model_name: 'public-a',
        billing_mode: 'token',
        group_ratio: 1,
        contract_applicable: 'unknown',
        usage,
        original_known: false,
        estimate_reasons: [reason],
      },
    ]
    await mount({ envelope, language: 'zh', isAdmin: true, userId: 7 })
    expect(screen.getAllByText(message)).toHaveLength(2)
    expect(screen.queryByText('无法展示')).toBeNull()
  }
)

it('shows no discount and zero savings when the recorded final factor is one', async () => {
  const envelope = statementEnvelope()
  envelope.result.discount_combinations = [
    {
      group_id: 4,
      model_name: 'public-a',
      billing_mode: 'token',
      group_ratio: 1,
      contract_applicable: 'no',
      usage,
      original_known: true,
      original_quota: -80,
      discount_quota: 0,
    },
  ]
  await mount({ envelope })
  const row = screen.getByText('No discount (×1)').closest('tr')
  expect(row).not.toBeNull()
  if (!row) throw new Error('Discount row not found')
  expect(within(row).getByText('No contract discount applied')).toBeTruthy()
  expect(within(row).queryByText(/Cannot estimate|Estimate failed/)).toBeNull()
  expect(within(row).getByText('$0')).toBeTruthy()
})

it('uses the group factor for historical rows without a contract and labels the contract briefly', async () => {
  const envelope = statementEnvelope()
  envelope.result.discount_combinations = [
    {
      group_id: 4,
      model_name: 'public-a',
      billing_mode: 'token',
      group_ratio: 0.5,
      contract_applicable: 'unrecorded',
      usage,
      original_known: true,
      original_quota: -160,
      discount_quota: -80,
    },
  ]
  await mount({ envelope, language: 'zh' })
  expect(screen.getAllByText('未记录').length).toBeGreaterThan(0)
  expect(screen.queryByText('合同优惠：未记录')).toBeNull()
  expect(screen.queryByText(/未知：历史合同状态/)).toBeNull()
  expect(screen.getAllByText('×0.5')).toHaveLength(2)
})

it('shows estimated savings amounts for expanded models instead of discount factors', async () => {
  const envelope = statementEnvelope()
  const model = envelope.result.groups[0].models[0]
  Object.assign(model, { discount_quota: 0 })
  model.original_quota = model.usage.net_quota
  await mount({ envelope })
  fireEvent.click(screen.getByRole('button', { name: 'Expand models' }))
  const row = screen.getByText('public-a').closest('tr')
  if (!row) throw new Error('Model row missing')
  const cells = within(row).getAllByRole('cell')
  expect(cells[4]).toHaveTextContent('$0')
  expect(cells[4]).not.toHaveTextContent('×')
  expect(
    screen.getByRole('columnheader', { name: 'Estimated savings' })
  ).toBeTruthy()
})

it('shows a factor of one once in each discount column', async () => {
  const envelope = statementEnvelope()
  envelope.result.discount_combinations = [
    {
      group_id: 4,
      model_name: 'public-a',
      billing_mode: 'token',
      group_ratio: 1,
      contract_applicable: 'yes',
      contract_ratio: 1,
      usage,
      original_known: true,
      original_quota: -80,
      discount_quota: 0,
    },
  ]
  await mount({ envelope })
  const row = screen.getByText('No discount (×1)').closest('tr')
  if (!row) throw new Error('Combination row missing')
  const cells = within(row).getAllByRole('cell')
  expect(within(cells[1]).getAllByText('×1')).toHaveLength(1)
  expect(within(cells[2]).getAllByText('×1')).toHaveLength(1)
})

it('shows known zero savings across cards, totals, API Keys and models without a contract', async () => {
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
  await mount({ envelope })
  fireEvent.click(screen.getByRole('button', { name: 'Expand models' }))
  expect(
    screen.queryByText(
      /Cannot estimate|Estimate failed|historical contract status not recorded/
    )
  ).toBeNull()
  // Combination, combination total, card, Key, and expanded model.
  expect(screen.getAllByText('$0', { exact: true })).toHaveLength(5)
  expect(screen.getByText('No discount (×1)')).toBeTruthy()
})

it('explains summary aggregation and per-call billing events before export', async () => {
  await mount({ envelope: statementEnvelope() })
  fireEvent.click(screen.getByRole('button', { name: 'Export and download' }))
  expect(
    await screen.findByText(
      'One row per API Key and model: requests, net amount, estimated list price and savings.'
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
