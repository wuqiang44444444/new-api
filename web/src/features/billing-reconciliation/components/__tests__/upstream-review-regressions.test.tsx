import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { resolveShanghaiMonth } from '../../lib'
import { UpstreamDataStatus } from '../upstream-data-status'
import {
  AmountCell,
  UpstreamReconciliationTable,
} from '../upstream-reconciliation-table'
import {
  type ProviderUrlGroupSummary,
  seedUpstreamPages,
  projectUpstreamPage,
} from './upstream-page-fixture'

const api = vi.hoisted(() => ({
  summary: vi.fn(),
}))
vi.mock('../../api', () => ({
  getAdminUpstreamReconciliation: api.summary,
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
it('keeps the original amount complete when detail usage is unavailable', async () => {
  const { rerender } = render(
    <UpstreamDataStatus
      scope='detail'
      originalAmount={100}
      quality={{ status: 'partial', cache_read_unavailable_requests: 1 }}
    />
  )
  expect(screen.getByText('Original amount complete')).toBeInTheDocument()
  const status = screen.getByRole('button', { name: 'Original amount complete' })
  expect(status).toHaveAttribute('aria-expanded', 'false')
  await userEvent.click(status)
  expect(status).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByText('Usage and record details')).toBeVisible()
  expect(screen.queryByText('Amount unavailable reasons')).not.toBeInTheDocument()
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
    screen.getByRole('button', { name: 'Channel tests only; amounts pending' })
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
        channels={source.channels}
        periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
        periodStart={1}
        expandedChannels={new Set([String(channel.channel_id)])}
        onToggleChannel={vi.fn()}
        onViewDetails={vi.fn()}
      />
    </QueryClientProvider>
  )
  for (const button of screen.getAllByRole('button', {
    name: 'Original amount incomplete',
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
  const headers = screen.getAllByRole('columnheader')
  const readColumn = headers.indexOf(
    screen.getByRole('columnheader', { name: 'Cache read tokens' })
  )
  const writeColumn = headers.indexOf(
    screen.getByRole('columnheader', { name: 'Cache write tokens' })
  )
  expect(within(rows[1]).getAllByRole('cell')[readColumn]).toHaveTextContent(
    'Historical meter unconfirmed'
  )
  expect(within(rows[2]).getAllByRole('cell')[readColumn]).toHaveTextContent(
    'Historical meter unconfirmed'
  )
  expect(within(rows[2]).getAllByRole('cell')[writeColumn]).toHaveTextContent(
    '0'
  )
  expect(
    screen.getAllByRole('button', {
      name: 'Original amount incomplete',
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
        channels={source.channels}
        periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
        periodStart={1}
        expandedChannels={new Set([String(channel.channel_id)])}
        onToggleChannel={vi.fn()}
        onViewDetails={vi.fn()}
      />
    </QueryClientProvider>
  )
  const statuses = screen.getAllByRole('button', {
    name: 'Original amount incomplete',
  })
  expect(statuses).toHaveLength(2)
  for (const button of statuses) {
    await userEvent.click(button)
    expect(screen.getByText(/1 records include tool surcharges/)).toBeVisible()
    expect(
      screen.getByText(
        '3 channel tests are priced and included in the amount total.'
      )
    ).toBeVisible()
    await userEvent.click(button)
  }
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
        expandedChannels={new Set([String(group.channels[0].channel_id)])}
        channels={group.channels}
        onToggleChannel={vi.fn()}
        onViewDetails={onViewDetails}
        periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
        periodStart={resolveShanghaiMonth('2026-09').start_timestamp}
      />
    </QueryClientProvider>
  )
  const links = screen.getAllByRole('button', { name: 'View details' })
  // 渠道父行只带渠道 ID，模型叶子再增加模型和计费方式。
  fireEvent.click(links[0])
  expect(onViewDetails).toHaveBeenLastCalledWith({
    channelId: 11,
    channelName: 'Channel #11',
  })
  // 模型叶子行在其上增加 Provider 模型、fallback 身份和计费方式。
  fireEvent.click(links[1])
  expect(onViewDetails).toHaveBeenLastCalledWith({
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
          expandedChannels={new Set([String(timed.channels[0].channel_id)])}
          channels={timed.channels}
          onToggleChannel={vi.fn()}
          onViewDetails={vi.fn()}
          periodEnd={resolveShanghaiMonth('2026-09').end_timestamp}
          periodStart={resolveShanghaiMonth('2026-09').start_timestamp}
        />
      </QueryClientProvider>
    )
    const secondsColumn = screen
      .getAllByRole('columnheader')
      .indexOf(screen.getByRole('columnheader', { name: 'Billable seconds' }))
    expect(
      screen.getByRole('columnheader', { name: 'Billable seconds' })
    ).toBeInTheDocument()
    for (const row of screen
      .getAllByRole('row')
      .filter((row) => within(row).queryAllByRole('cell').length === 14)) {
      expect(within(row).getAllByRole('cell')[secondsColumn]).toHaveTextContent(
        display
      )
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
        channels={source.channels}
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
        evidence_coverage: { rows: 3920, gap_rows: 750, amount_gap_rows: 750 },
        usage_without_amount_rows: 750,
        cache_write_unavailable_requests: 538,
        legacy_test_cache_write_rows: 538,
        test_priced_rows: 3,
      }}
    />
  )
  expect(
    screen.queryByText(
      'Original amount pending: 750 records'
    )
  ).not.toBeInTheDocument()
  await userEvent.click(
    screen.getByRole('button', { name: 'Original amount incomplete' })
  )
  expect(
    screen.getByText('Original amount pending: 750 records')
  ).toBeVisible()
  expect(
    screen.getByText('Original amount available: 3,170 records')
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
    screen.queryByText(
      'Of the unpriced tests above, 538 also lack cache write details; these are the same records.'
    )
  ).not.toBeInTheDocument()
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

it('counts only missing originals even when the frozen report has usage gaps', async () => {
  render(
    <UpstreamDataStatus
      scope='report'
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
  await userEvent.click(screen.getByRole('button', {
    name: '619 of 5,381 records have an original amount pending',
  }))
  expect(
    screen.getByText('Original amount available: 4,762 records')
  ).toBeVisible()
  expect(
    screen.getByText('Original amount pending: 619 records')
  ).toBeVisible()
  expect(
    screen.queryByText('Amount available, usage details missing: 9 records')
  ).not.toBeInTheDocument()
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

it('hides unverifiable historical amounts while preserving verified ones', () => {
  render(
    <table>
      <tbody>
        <tr>
          <AmountCell value={5} unverified />
          <AmountCell value={10} />
        </tr>
      </tbody>
    </table>
  )
  const [unverified, recorded] = screen.getAllByRole('cell')
  expect(unverified).toHaveTextContent('Cannot calculate')
  expect(recorded).toHaveTextContent('$0.00002')
})
