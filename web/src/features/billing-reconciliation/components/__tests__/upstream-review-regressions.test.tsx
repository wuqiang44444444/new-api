import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { resolveShanghaiMonth } from '../../lib'
import type { ProviderUrlGroupSummary } from '../../types'
import { channelRowKey } from '../../upstream-statement-utils'
import { UpstreamDataStatus } from '../upstream-data-status'
import { UpstreamReconciliationView } from '../upstream-reconciliation'
import { UpstreamReconciliationTable } from '../upstream-reconciliation-table'
import { seedUpstreamPages, projectUpstreamPage } from './upstream-page-fixture'

const api = vi.hoisted(() => ({
  save: vi.fn(),
  init: vi.fn(),
  summary: vi.fn(),
  saveName: vi.fn(),
}))
vi.mock('../../api', () => ({
  putAdminUpstreamDiscount: api.save,
  postAdminUpstreamDiscountInit: api.init,
  getAdminUpstreamReconciliation: api.summary,
  putAdminUpstreamURLName: api.saveName,
}))
vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({
    t: (key: string, vars?: Record<string, unknown>) =>
      key.replaceAll(/{{(\w+)}}/g, (_, name) => String(vars?.[name] ?? name)),
  }),
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))
const usage = {
  requests: 1,
  billable_calls: 0,
  input_tokens: 2,
  output_tokens: 3,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
}
const leaf = {
  provider_model: 'unmapped',
  provider_model_fallback: true,
  billing_mode: 'token' as const,
  customer_models: ['unmapped'],
  usage,
  detail_filter: { start_timestamp: 1, end_timestamp: 2, channel_id: 11 },
}
const group: ProviderUrlGroupSummary = {
  url_key: 'https://billing.example',
  display_name: 'Upstream',
  channel_ids: [11],
  channel_count: 1,
  model_count: 1,
  usage,
  reference_known: true,
  discount_pending_channels: 0,
  channels: [
    {
      channel_id: 11,
      channel_name: 'Channel',
      discount: { value: '0.8', version: 1, source: 'database' },
      usage,
      models: [leaf],
    },
  ],
}
beforeEach(() => {
  vi.clearAllMocks()
  api.summary.mockImplementation((params) =>
    Promise.resolve(
      projectUpstreamPage(
        { result: { url_groups: [group] }, generated_at: 1 },
        params
      )
    )
  )
})
it('separates known money from missing usage and makes evidence notes clickable', async () => {
  const { rerender } = render(
    <UpstreamDataStatus
      originalAmount={100}
      quality={{ status: 'partial', cache_read_unavailable_requests: 1 }}
    />
  )
  expect(screen.getByText('Original amount complete')).toBeInTheDocument()
  const status = screen.getByRole('button', { name: /^Cache metering notes:/ })
  expect(status).toHaveAttribute('aria-expanded', 'false')
  await userEvent.click(status)
  expect(status).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByText('Evidence gap categories')).toBeVisible()
  await userEvent.click(status)
  rerender(
    <UpstreamDataStatus
      usageOnly
      quality={{ status: 'complete', usage_without_amount_rows: 1 }}
    />
  )
  expect(
    screen.getByText('Channel tests only; amounts pending')
  ).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: /^Pricing evidence incomplete:/ })
  ).toBeInTheDocument()
  expect(
    screen.queryByText('Original amount incomplete')
  ).not.toBeInTheDocument()
})
it('shows test-only coverage and distinguishes absent cache readings from explicit zero', async () => {
  const source = structuredClone(group)
  const channel = source.channels[0]
  channel.data_quality = {
    status: 'partial',
    usage_without_amount_rows: 2,
    cache_read_unavailable_requests: 1,
  }
  channel.models[0].data_quality = {
    status: 'partial',
    usage_without_amount_rows: 2,
    cache_read_unavailable_requests: 1,
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  seedUpstreamPages(
    client,
    {
      start_timestamp: 1,
      end_timestamp: resolveShanghaiMonth('2026-09').end_timestamp,
    },
    { result: { url_groups: [source] }, generated_at: 1 }
  )
  render(
    <QueryClientProvider client={client}>
      <UpstreamReconciliationTable
        group={source}
        periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
        periodStart={1}
        expandedChannels={new Set([channelRowKey(source.url_key, channel)])}
        onToggleChannel={vi.fn()}
        onViewDetails={vi.fn()}
      />
    </QueryClientProvider>
  )
  for (const button of screen.getAllByRole('button', {
    name: /^Pricing evidence incomplete:/,
  })) {
    await userEvent.click(button)
    expect(
      screen.getByText(
        '2 channel tests did not save the pricing evidence needed to rebuild their amount; usage is retained. The breakdown below partitions these tests.'
      )
    ).toBeVisible()
    await userEvent.click(button)
  }
  const rows = screen.getAllByRole('row')
  expect(within(rows[1]).getAllByRole('cell')[4]).toHaveTextContent(
    'Historical meter unconfirmed'
  )
  expect(within(rows[2]).getAllByRole('cell')[4]).toHaveTextContent(
    'Historical meter unconfirmed'
  )
  expect(within(rows[2]).getAllByRole('cell')[5]).toHaveTextContent('0')
  expect(
    screen.getAllByRole('button', {
      name: /Pricing evidence incomplete:.*did not save the pricing evidence/,
    })
  ).toHaveLength(2)
})
it('shows priced channel tests as reference amount coverage', async () => {
  const source = structuredClone(group)
  const channel = source.channels[0]
  channel.data_quality = {
    status: 'partial',
    test_priced_rows: 3,
    auxiliary_charge_rows: 1,
  }
  channel.models[0].data_quality = {
    status: 'partial',
    test_priced_rows: 3,
    auxiliary_charge_rows: 1,
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  seedUpstreamPages(
    client,
    {
      start_timestamp: 1,
      end_timestamp: resolveShanghaiMonth('2026-09').end_timestamp,
    },
    { result: { url_groups: [source] }, generated_at: 1 }
  )
  render(
    <QueryClientProvider client={client}>
      <UpstreamReconciliationTable
        group={source}
        periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
        periodStart={1}
        expandedChannels={new Set([channelRowKey(source.url_key, channel)])}
        onToggleChannel={vi.fn()}
        onViewDetails={vi.fn()}
      />
    </QueryClientProvider>
  )
  const statuses = screen.getAllByRole('button', {
    name: /Price restore blocked:.*tool surcharges/,
  })
  expect(statuses).toHaveLength(2)
  for (const button of statuses) {
    await userEvent.click(button)
    expect(
      screen.getByText(
        '3 channel tests are priced and included in the amount total.'
      )
    ).toBeVisible()
    await userEvent.click(button)
  }
})
it('clears the discount editor when switching to a cached month', () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  for (const month of ['2026-08', '2026-09']) {
    const period = resolveShanghaiMonth(month)
    seedUpstreamPages(client, period, {
      result: { url_groups: [group] },
      generated_at: period.start_timestamp,
    })
  }
  const view = (month: string) => (
    <QueryClientProvider client={client}>
      <UpstreamReconciliationView
        month={month}
        period={resolveShanghaiMonth(month)}
        onMonthChange={vi.fn()}
      />
    </QueryClientProvider>
  )
  const rendered = render(view('2026-08'))
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
  fireEvent.change(screen.getByLabelText('Discount percent'), {
    target: { value: '70' },
  })
  rendered.rerender(view('2026-09'))
  expect(screen.queryByLabelText('Discount percent')).not.toBeInTheDocument()
  expect(api.save).not.toHaveBeenCalled()
  expect(
    screen.getByText(/Asia\/Shanghai, 2026-09-01 00:00:00 \+08:00/)
  ).toBeInTheDocument()
})
it('keeps channel totals on the parent row and fallback identity on leaf detail links', () => {
  const onViewDetails = vi.fn()
  const client = new QueryClient()
  seedUpstreamPages(client, resolveShanghaiMonth('2026-09'), {
    result: { url_groups: [group] },
    generated_at: 1,
  })
  render(
    <QueryClientProvider client={client}>
      <UpstreamReconciliationTable
        expandedChannels={
          new Set([channelRowKey(group.url_key, group.channels[0])])
        }
        group={group}
        onToggleChannel={vi.fn()}
        onViewDetails={onViewDetails}
        periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
        periodStart={resolveShanghaiMonth('2026-09').start_timestamp}
      />
    </QueryClientProvider>
  )
  const links = screen.getAllByRole('button', { name: 'View details' })
  // 渠道父行只带账期、URL 分组与渠道 ID。
  fireEvent.click(links[0])
  expect(onViewDetails).toHaveBeenLastCalledWith({
    urlKey: group.url_key,
    channelId: 11,
    channelName: 'Channel #11',
  })
  // 模型叶子行在其上增加 Provider 模型、fallback 身份和计费方式。
  fireEvent.click(links[1])
  expect(onViewDetails).toHaveBeenLastCalledWith({
    urlKey: group.url_key,
    channelId: 11,
    channelName: 'Channel #11',
    providerModel: 'unmapped',
    providerModelFallback: true,
    billingMode: 'token',
  })
})

it.each([
  { seconds: '6.5', missing: 0, display: '6.5' },
  { seconds: '0', missing: 0, display: '0' },
  { seconds: '6.5', missing: 1, display: 'Known subtotal: 6.5' },
  { seconds: null, missing: 1, display: 'Not recorded' },
])(
  'keeps seconds distinct from missing evidence: $display ($missing missing)',
  ({ seconds, missing, display }) => {
    const timed = structuredClone(group)
    timed.usage = { ...usage, seconds, seconds_unavailable_rows: missing }
    timed.channels[0].usage = timed.usage
    timed.channels[0].models[0].billing_mode = 'per_second'
    timed.channels[0].models[0].usage = timed.usage
    const client = new QueryClient()
    seedUpstreamPages(client, resolveShanghaiMonth('2026-09'), {
      result: { url_groups: [timed] },
      generated_at: 1,
    })
    render(
      <QueryClientProvider client={client}>
        <UpstreamReconciliationTable
          expandedChannels={
            new Set([channelRowKey(timed.url_key, timed.channels[0])])
          }
          group={timed}
          onToggleChannel={vi.fn()}
          onViewDetails={vi.fn()}
          periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
          periodStart={resolveShanghaiMonth('2026-09').start_timestamp}
        />
      </QueryClientProvider>
    )
    expect(
      screen.getByRole('columnheader', { name: 'Billable seconds' })
    ).toBeInTheDocument()
    for (const row of screen
      .getAllByRole('row')
      .filter((row) => within(row).queryAllByRole('cell').length === 14)) {
      expect(within(row).getAllByRole('cell')[8]).toHaveTextContent(display)
    }
  }
)

it('keeps a known subtotal separate from an incomplete channel total', () => {
  const source = structuredClone(group)
  source.original_amount = undefined
  source.reference_amount = undefined
  source.reference_known = false
  source.known_original_amount = 1562400
  source.known_reference_amount = 1562400
  const channel = source.channels[0]
  channel.original_amount = undefined
  channel.reference_amount = undefined
  channel.known_original_amount = 1562400
  channel.known_reference_amount = 1562400
  channel.data_quality = { status: 'partial', usage_without_amount_rows: 324 }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  seedUpstreamPages(
    client,
    {
      start_timestamp: 1,
      end_timestamp: resolveShanghaiMonth('2026-09').end_timestamp,
    },
    { result: { url_groups: [source] }, generated_at: 1 }
  )
  render(
    <QueryClientProvider client={client}>
      <UpstreamReconciliationTable
        group={source}
        periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
        periodStart={1}
        expandedChannels={new Set()}
        onToggleChannel={vi.fn()}
        onViewDetails={vi.fn()}
      />
    </QueryClientProvider>
  )
  expect(screen.getByText('Original amount incomplete')).toBeInTheDocument()
  expect(screen.queryByText('Original amount complete')).not.toBeInTheDocument()
  expect(screen.getAllByText(/^Known subtotal:/)).toHaveLength(2)
})

it('reports a deduplicated denominator and does not present priced tests as issues', async () => {
  render(
    <UpstreamDataStatus
      quality={{
        status: 'partial',
        evidence_coverage: { rows: 3920, gap_rows: 750 },
        usage_without_amount_rows: 750,
        cache_write_unavailable_requests: 538,
        legacy_test_cache_write_rows: 538,
        test_priced_rows: 3,
      }}
    />
  )
  expect(
    screen.queryByText(
      '750 of 3,920 billing records have incomplete information'
    )
  ).not.toBeInTheDocument()
  await userEvent.click(
    screen.getByRole('button', { name: /^Pricing evidence incomplete:/ })
  )
  expect(
    screen.getByText('750 of 3,920 billing records have incomplete information')
  ).toBeVisible()
  expect(
    screen.getByText('Amount and usage information complete: 3,170 records')
  ).toBeVisible()
  expect(
    screen.getByText(
      'Except for the mutually exclusive channel-test breakdown, categories may overlap on the same record; do not add these counts together.'
    )
  ).toBeVisible()
  expect(
    screen.getAllByText(
      '750 channel tests did not save the pricing evidence needed to rebuild their amount; usage is retained. The breakdown below partitions these tests.'
    )
  ).toHaveLength(1)
  expect(
    screen.getAllByText(
      '3 channel tests are priced and included in the amount total.'
    )
  ).toHaveLength(1)
  expect(
    screen.getByText(
      'Of the unpriced tests above, 538 also lack cache write details; these are the same records.'
    )
  ).toBeVisible()
  expect(
    screen.queryByText('538 billing records lack cache write details.')
  ).not.toBeInTheDocument()
  expect(
    screen
      .getAllByRole('listitem')
      .some((item) =>
        item.textContent?.includes('included in the amount total')
      )
  ).toBe(false)
})

it('explains the incomplete record total with mutually exclusive billing reasons', async () => {
  render(
    <UpstreamDataStatus
      quality={{
        status: 'partial',
        evidence_coverage: {
          rows: 5381,
          gap_rows: 628,
          amount_gap_rows: 619,
          usage_gap_rows: 9,
          other_gap_rows: 0,
        },
      }}
    />
  )
  await userEvent.click(screen.getByRole('button'))
  expect(
    screen.getByText('628 of 5,381 billing records have incomplete information')
  ).toBeVisible()
  expect(
    screen.getByText('Amount and usage information complete: 4,753 records')
  ).toBeVisible()
  expect(
    screen.getByText('Amounts that cannot be calculated yet: 619 records')
  ).toBeVisible()
  expect(
    screen.getByText('Amount available, usage details missing: 9 records')
  ).toBeVisible()
  expect(
    screen.queryByText(/other billing details missing/)
  ).not.toBeInTheDocument()
})

it('shows verified billing seconds and refunded holds as accounting notes, not missing information', async () => {
  render(
    <UpstreamDataStatus
      quality={{
        status: 'complete',
        recovered_billing_seconds_rows: 593,
        refunded_task_hold_rows: 38,
      }}
      originalAmount={100}
    />
  )
  await userEvent.click(screen.getByRole('button'))
  expect(
    screen.getByText(
      '593 records have billable seconds verified against their original settlement.'
    )
  ).toBeVisible()
  expect(
    screen.getByText(
      '38 failed-task holds were refunded to customers; no successful-task seconds are counted.'
    )
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /incomplete/i })
  ).not.toBeInTheDocument()
})
