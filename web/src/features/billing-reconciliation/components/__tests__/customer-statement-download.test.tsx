import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  cleanup,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { buildCustomerStatementCsv } from '../../customer-statement-csv'
import type { BillingEnvelope, CustomerStatement } from '../../types'
import { CustomerStatementView } from '../customer-statement'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

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
          name: '=formula,"key"\nname',
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
              data_quality: {
                status: 'partial',
                input_tokens_unavailable_requests: 1,
              },
            },
            {
              model_name: 'public-b',
              billing_mode: 'unknown',
              usage: { ...usage, net_quota: 0 },
              price_versions: 0,
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
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

async function mount(envelope?: BillingEnvelope<CustomerStatement>) {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  if (envelope) {
    client.setQueryData(
      [
        'billing-customer-reconciliation',
        false,
        undefined,
        'api_key',
        period.start_timestamp,
        period.end_timestamp,
      ],
      envelope
    )
  }
  const element = (nextPeriod = period) => (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <CustomerStatementView
          isAdmin={false}
          dimension='api_key'
          period={nextPeriod}
          onDimensionChange={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  const view = render(element())
  return { view, element, i18n, client }
}

it.each([
  [1, '$0.000002'],
  [-1, '-$0.000002'],
  [0, '$0'],
])(
  'keeps page and CSV net amounts equal for %s quota',
  async (quota, expected) => {
    const envelope = statementEnvelope()
    envelope.result.summary.net_quota = quota
    envelope.result.groups[0].usage = { ...usage, net_quota: quota }
    envelope.result.groups[0].models[0].usage = { ...usage, net_quota: quota }
    const { i18n } = await mount(envelope)
    expect(screen.getAllByText(expected).length).toBeGreaterThanOrEqual(2)
    fireEvent.click(screen.getByRole('button', { name: 'Expand models' }))
    expect(screen.getAllByText(expected).length).toBeGreaterThanOrEqual(3)
    expect(buildCustomerStatementCsv(envelope, i18n.t)).toContain(expected)
  }
)

it('downloads every model from the authenticated monthly response while rows stay collapsed', async () => {
  const envelope = statementEnvelope()
  const create = vi.fn((_blob: Blob) => 'blob:bill')
  const revoke = vi.fn()
  vi.stubGlobal(
    'URL',
    class extends URL {
      static createObjectURL = create
      static revokeObjectURL = revoke
    }
  )
  let filename = ''
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(
    function (this: HTMLAnchorElement) {
      filename = this.download
    }
  )
  const { i18n } = await mount(envelope)
  expect(screen.queryByText('public-a')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Download statement' }))
  expect(filename).toBe('my-billing-2026-09.csv')
  expect(revoke).toHaveBeenCalledWith('blob:bill')
  const blob = create.mock.calls[0][0] as Blob
  const bytes = await new Promise<ArrayBuffer>((resolve) => {
    const reader = new FileReader()
    reader.addEventListener(
      'load',
      () => resolve(reader.result as ArrayBuffer),
      { once: true }
    )
    reader.readAsArrayBuffer(blob)
  })
  expect(new Uint8Array(bytes).slice(0, 3)).toEqual(
    new Uint8Array([239, 187, 191])
  )
  const csv = new TextDecoder().decode(bytes)
  expect(csv).toBe(buildCustomerStatementCsv(envelope, i18n.t))
  expect(csv).toContain('public-a')
  expect(csv).toContain('public-b')
  expect(csv).toContain('"\'=formula,""key""\nname"')
  expect(csv).toContain('-80')
  expect(csv).toContain('Estimated list price')
  expect(csv).not.toContain('channel_id')
  expect(csv).not.toContain('provider_model')
  expect(api.get).not.toHaveBeenCalled()
})

it('keeps unavailable values blank and uses backend totals without summing model rows', async () => {
  const { i18n } = await mount(statementEnvelope())
  const envelope = statementEnvelope()
  envelope.result.summary.net_quota = -999
  const csv = buildCustomerStatementCsv(envelope, i18n.t)
  expect(csv).toContain(',-999,')
  expect(csv).toContain('public-a,Token billing,2,,900,0,20')
  expect(csv).toContain('public-b,Unknown billing mode,2,,,,,,,,,,')
  expect(csv).not.toContain('undefined')
  expect(csv).not.toContain('NaN')
})

it('disables downloads for an empty month and while loading a different month', async () => {
  vi.mocked(api.get).mockReturnValue(new Promise(() => undefined))
  const envelope = statementEnvelope()
  envelope.result.groups = []
  const { view, element } = await mount(envelope)
  expect(
    screen.getByRole('button', { name: 'Download statement' })
  ).toBeDisabled()
  view.rerender(
    element({ start_timestamp: 1790784000, end_timestamp: 1793462399 })
  )
  expect(
    screen.getByRole('button', { name: 'Download statement' })
  ).toBeDisabled()
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith('/api/billing/statement/self', {
      params: { start_timestamp: 1790784000, end_timestamp: 1793462399 },
    })
  )
})

it('does not offer a download when the statement request fails', async () => {
  vi.mocked(api.get).mockRejectedValue(new Error('Unavailable'))
  await mount()
  await screen.findByText('Unable to load customer billing')
  expect(
    screen.queryByRole('button', { name: 'Download statement' })
  ).toBeNull()
})

it('reports browser download failure instead of reporting success', async () => {
  vi.stubGlobal(
    'URL',
    class extends URL {
      static createObjectURL = vi.fn(() => {
        throw new Error('No blob support')
      })
    }
  )
  await mount(statementEnvelope())
  fireEvent.click(screen.getByRole('button', { name: 'Download statement' }))
  expect(toast.error).toHaveBeenCalledWith('Unable to export statement.')
  expect(toast.success).not.toHaveBeenCalled()
})
