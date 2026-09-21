import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { beforeAll, afterAll, beforeEach, expect, it, vi } from 'vitest'

import { formatCustomerStatementQuota, resolveShanghaiMonth } from '../../lib'
import type { ProviderUrlGroupSummary } from '../../types'
import { UpstreamReconciliationView } from '../upstream-reconciliation'
import {
  seedUpstreamPages,
  projectUpstreamPage,
  type SummaryFixture,
} from './upstream-page-fixture'

const api = vi.hoisted(() => ({
  summary: vi.fn(),
  init: vi.fn(),
  details: vi.fn(),
  export: vi.fn(),
  save: vi.fn(),
  name: vi.fn(),
}))
vi.mock('../../api', () => ({
  getAdminUpstreamReconciliation: api.summary,
  postAdminUpstreamDiscountInit: api.init,
  getAdminUpstreamDetails: api.details,
  putAdminUpstreamDiscount: api.save,
  putAdminUpstreamURLName: api.name,
}))
vi.mock('../../export-api', () => ({ createUpstreamExport: api.export }))
vi.mock('../../export-jobs-drawer', () => ({
  ExportJobsDrawer: (props: { open: boolean }) =>
    props.open ? <div role='dialog' aria-label='Export jobs' /> : null,
}))
vi.mock('react-i18next', async (original) => ({
  ...(await original<typeof import('react-i18next')>()),
  useTranslation: () => ({
    t: (key: string, vars?: Record<string, unknown>) =>
      key.replaceAll(/{{(\w+)}}/g, (_, name) => String(vars?.[name] ?? name)),
  }),
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

const period = resolveShanghaiMonth('2026-09')
const usage = {
  requests: 2,
  billable_calls: 0,
  input_tokens: 20,
  output_tokens: 10,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
}

function upstreamGroup(): ProviderUrlGroupSummary {
  return {
    url_key: 'https://upstream.example',
    display_name: 'https://upstream.example',
    channel_ids: [11, 12],
    channel_count: 2,
    model_count: 1,
    usage,
    original_amount: 3500,
    reference_amount: 3100,
    reference_known: true,
    discount_pending_channels: 0,
    channels: [11, 12].map((id) => ({
      channel_id: id,
      channel_name: `Channel ${id}`,
      discount: { value: '1', version: 1, source: 'database' },
      usage,
      models: [],
      original_amount: id === 11 ? 1000 : 2500,
      reference_amount: id === 11 ? 1000 : 2100,
    })),
  }
}

function renderGroup(
  group: ProviderUrlGroupSummary,
  month = '2026-09',
  initialize = false
) {
  const period = resolveShanghaiMonth(month)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  const data = {
    result: { url_groups: [group], data_quality: group.data_quality },
    generated_at: period.start_timestamp,
  }
  seedUpstreamPages(client, period, data)
  if (initialize) {
    client.removeQueries({ queryKey: ['billing-upstream-initialize'] })
  }
  api.summary.mockImplementation((params) => {
    const source = client.getQueryData<SummaryFixture>([
      'upstream-test-source',
      period.start_timestamp,
    ])
    if (!source) throw new Error('Missing upstream test fixture')
    return Promise.resolve(projectUpstreamPage(source, params))
  })
  render(
    <QueryClientProvider client={client}>
      <UpstreamReconciliationView
        month={month}
        period={period}
        onMonthChange={vi.fn()}
      />
    </QueryClientProvider>
  )
  return client
}

// jsdom lacks pointer capture used by the real drawer; keep the modal and
// focus behavior real while supplying only the missing browser primitive.
const pointerCapture = Object.getOwnPropertyDescriptor(
  Element.prototype,
  'setPointerCapture'
)
beforeAll(() => {
  Object.defineProperty(Element.prototype, 'setPointerCapture', {
    configurable: true,
    value: () => {},
  })
})
afterAll(() => {
  if (pointerCapture) {
    Object.defineProperty(
      Element.prototype,
      'setPointerCapture',
      pointerCapture
    )
  } else {
    Reflect.deleteProperty(Element.prototype, 'setPointerCapture')
  }
})
beforeEach(() => {
  vi.resetAllMocks()
  api.init.mockResolvedValue({ success: true, data: { outcomes: [] } })
  api.details.mockResolvedValue({
    success: true,
    data: { result: { items: [], total: 0, page: 1, page_size: 50 } },
  })
  api.export.mockResolvedValue({})
  api.name.mockResolvedValue({ success: true })
})

it('shows the server-provided upstream totals above both channels', () => {
  renderGroup(upstreamGroup())
  expect(
    screen.getByText(formatCustomerStatementQuota(3500))
  ).toBeInTheDocument()
  expect(
    screen.getByText(formatCustomerStatementQuota(3100))
  ).toBeInTheDocument()
})

it('keeps incomplete upstream totals unknown instead of adding known channels', () => {
  const group = upstreamGroup()
  group.original_amount = undefined
  group.reference_amount = undefined
  group.reference_known = false
  renderGroup(group)
  expect(screen.getByText('Unknown')).toBeInTheDocument()
  expect(screen.getByText('Incomplete')).toBeInTheDocument()
})

it('opens and exports all channels of an upstream without a channel or model restriction', async () => {
  const group = upstreamGroup()
  renderGroup(group)
  const cardHeader = screen
    .getByText(group.display_name, { selector: '[data-slot="card-title"]' })
    .closest<HTMLElement>('[data-slot="card-header"]')
  if (!cardHeader) throw new Error('Upstream card header is missing')
  fireEvent.click(
    within(cardHeader).getByRole('button', { name: 'View details' })
  )
  await waitFor(() => expect(api.details).toHaveBeenCalled())
  expect(api.details.mock.lastCall?.[0]).toMatchObject({
    ...period,
    url_key: group.url_key,
  })
  expect(api.details.mock.lastCall?.[0].channel_id).toBeUndefined()
  fireEvent.click(
    screen.getByRole('button', { name: 'Export all matching details' })
  )
  await waitFor(() => expect(api.export).toHaveBeenCalled())
  expect(
    await screen.findByRole('dialog', { name: 'Export jobs' })
  ).toBeVisible()
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Upstream evidence details' })
    ).not.toBeInTheDocument()
  )
  expect(api.export.mock.lastCall?.[0]).toMatchObject({
    url_key: group.url_key,
    end_timestamp: period.end_timestamp + 1,
  })
  expect(api.export.mock.lastCall?.[0].channel_id).toBeUndefined()
  expect(api.export.mock.lastCall?.[0].model_name).toBeUndefined()
})

it('initializes the entire month before showing amounts, independent of visible channels', async () => {
  const group = upstreamGroup()
  group.channels = Array.from({ length: 11 }, (_, index) => ({
    ...group.channels[0],
    channel_id: index + 1,
  }))
  group.channel_count = 11
  let complete: ((value: unknown) => void) | undefined
  api.init.mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve
      })
  )
  renderGroup(group, '2026-09', true)
  expect(
    screen.queryByRole('button', { name: 'Export our statement' })
  ).not.toBeInTheDocument()
  await waitFor(() =>
    expect(api.init).toHaveBeenCalledExactlyOnceWith({
      period_start: period.start_timestamp,
      end_timestamp: period.end_timestamp + 1,
    })
  )
  await act(async () => {
    complete?.({ success: true, data: { counts: { exists: 11 } } })
  })
  expect(
    await screen.findByRole('button', { name: 'Export our statement' })
  ).toBeVisible()
})

it('keeps amounts and export unavailable when period initialization fails', async () => {
  api.init.mockResolvedValue({
    success: false,
    message: 'initialization failed',
  })
  renderGroup(upstreamGroup(), '2026-09', true)
  await waitFor(() =>
    expect(
      screen.getByText('Unable to load upstream reconciliation')
    ).toBeVisible()
  )
  expect(
    screen.queryByRole('button', { name: 'Export our statement' })
  ).not.toBeInTheDocument()
})

it.each(['', 'Retired supplier'])(
  'shows the deleted-channel marker once with alias %s',
  (name) => {
    const group = upstreamGroup()
    group.unidentified = true
    group.deleted = true
    group.custom_name = name
    renderGroup(group)
    // The select may repeat its option label; only inspect the actual card header.
    const title = name || 'Deleted channel · #11, #12'
    const header = screen
      .getByText(title, { selector: '[data-slot="card-title"]' })
      .closest('[data-slot="card-header"]')
    if (!header) throw new Error('Upstream card header is missing')
    expect(header.textContent?.match(/Deleted channel/g)).toHaveLength(1)
  }
)

it('opens channel and model evidence in a named dialog and closes with Escape', async () => {
  const user = userEvent.setup()
  const group = upstreamGroup()
  group.channels = group.channels.slice(0, 1)
  group.channels[0].models = [
    {
      provider_model: 'image-model',
      customer_models: ['public-image'],
      billing_mode: 'token',
      usage,
      detail_filter: {
        start_timestamp: period.start_timestamp,
        end_timestamp: period.end_timestamp,
        channel_id: 11,
      },
    },
  ]
  renderGroup(group)
  await user.click(screen.getByRole('button', { name: 'Expand models' }))
  const triggers = screen.getAllByRole('button', { name: 'View details' })
  await user.click(triggers[1])
  expect(
    await screen.findByRole('dialog', { name: 'Upstream evidence details' })
  ).toBeVisible()
  await waitFor(() =>
    expect(api.details.mock.lastCall?.[0]).toMatchObject({
      channel_id: 11,
      url_key: group.url_key,
    })
  )
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(triggers[1]).toHaveFocus()
  await user.click(triggers[2])
  const dialog = await screen.findByRole('dialog', {
    name: 'Upstream evidence details',
  })
  expect(
    within(dialog).getByText(/2026-09.*Channel 11.*image-model/)
  ).toBeInTheDocument()
  await waitFor(() =>
    expect(api.details.mock.lastCall?.[0]).toMatchObject({
      channel_id: 11,
      model_name: 'image-model',
      billing_mode: 'token',
    })
  )
  await user.click(
    within(dialog).getByRole('button', { name: 'Close details' })
  )
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})

it('shows loading and retryable errors without claiming there are no matching rows', async () => {
  const user = userEvent.setup()
  let rejectRequest: (error: Error) => void = () => {
    throw new Error('Request was not started')
  }
  api.details.mockImplementationOnce(
    () =>
      new Promise((_resolve, reject) => {
        rejectRequest = reject
      })
  )
  renderGroup(upstreamGroup())
  await user.click(screen.getAllByRole('button', { name: 'View details' })[0])
  const dialog = await screen.findByRole('dialog', {
    name: 'Upstream evidence details',
  })
  expect(within(dialog).getByRole('status')).toHaveTextContent(
    'Loading upstream details...'
  )
  await act(async () => {
    rejectRequest(new Error('Example request failed'))
  })
  expect(
    await within(dialog).findByText('Example request failed')
  ).toBeInTheDocument()
  expect(
    within(dialog).queryByText('No evidence rows match the current filters.')
  ).not.toBeInTheDocument()
  await user.click(within(dialog).getByRole('button', { name: 'Retry' }))
  expect(
    await within(dialog).findByText(
      'No evidence rows match the current filters.'
    )
  ).toBeInTheDocument()
})

it('refreshes the saved channel coefficient in every model row without client-side repricing', async () => {
  const user = userEvent.setup()
  const group = upstreamGroup()
  group.channels = group.channels.slice(0, 1)
  group.channels[0].models = [
    {
      provider_model: 'image-model',
      customer_models: ['public-image'],
      billing_mode: 'token',
      usage,
      original_amount: 1000,
      reference_amount: 1000,
      detail_filter: {
        start_timestamp: period.start_timestamp,
        end_timestamp: period.end_timestamp,
        channel_id: 11,
      },
    },
  ]
  renderGroup(group)
  await user.click(screen.getByRole('button', { name: 'Expand models' }))
  expect(
    screen.getByText('Inherited channel discount: No discount (×1)')
  ).toBeInTheDocument()
  const updated = structuredClone(group)
  updated.channels[0].discount = {
    value: '0.8',
    version: 2,
    source: 'database',
  }
  updated.channels[0].reference_amount = 800
  updated.channels[0].models[0].reference_amount = 800
  api.save.mockResolvedValue({ success: true })
  api.summary.mockImplementation((params) =>
    Promise.resolve(
      projectUpstreamPage(
        {
          result: { url_groups: [updated] },
          generated_at: period.start_timestamp,
        },
        params
      )
    )
  )
  await user.click(screen.getByRole('button', { name: 'Edit' }))
  await user.clear(screen.getByRole('textbox', { name: 'Discount percent' }))
  await user.type(
    screen.getByRole('textbox', { name: 'Discount percent' }),
    '80'
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(
    await screen.findByText('Inherited channel discount: ×0.8 (20% off)')
  ).toBeInTheDocument()
  expect(api.save).toHaveBeenCalledWith({
    period_start: period.start_timestamp,
    channel_id: 11,
    discount: '0.8',
    expected_version: 1,
  })
  expect(screen.getAllByText(formatCustomerStatementQuota(800))).toHaveLength(2)
})

it('scopes record notes to the selected upstream and restores overall notes for all URLs', async () => {
  const user = userEvent.setup()
  const complete = {
    ...upstreamGroup(),
    data_quality: { status: 'complete' as const },
  }
  const partial = {
    ...upstreamGroup(),
    url_key: 'https://partial.example',
    display_name: 'https://partial.example',
    data_quality: {
      status: 'partial' as const,
      cache_read_unavailable_requests: 3,
    },
  }
  const client = renderGroup(complete)
  act(() =>
    seedUpstreamPages(client, period, {
      result: {
        url_groups: [complete, partial],
        data_quality: partial.data_quality,
      },
      generated_at: period.start_timestamp,
    })
  )
  const reason = '3 billing records lack cache read details.'
  expect(await screen.findByText(reason)).toBeVisible()
  await user.click(screen.getByRole('combobox', { name: 'Upstream base URL' }))
  await user.click(screen.getByRole('option', { name: complete.url_key }))
  expect(screen.queryByText(reason)).not.toBeInTheDocument()
  await user.click(screen.getByRole('combobox', { name: 'Upstream base URL' }))
  await user.click(screen.getByRole('option', { name: partial.url_key }))
  expect(screen.getByText(reason)).toBeVisible()
  await user.click(screen.getByRole('combobox', { name: 'Upstream base URL' }))
  await user.click(screen.getByRole('option', { name: 'All upstream URLs' }))
  expect(screen.getByText(reason)).toBeVisible()
})

it.each([false, true])(
  'shows a saved fallback-group name in the filter without masking state (deleted=%s)',
  async (deleted) => {
    const user = userEvent.setup()
    const group = {
      ...upstreamGroup(),
      url_key: 'channel:24',
      display_name: 'Channel #24',
      channel_ids: [24],
      unidentified: true,
      deleted,
      custom_name: 'Legacy supplier',
    }
    renderGroup(group)
    expect(
      screen.getByText('Legacy supplier', {
        selector: '[data-slot="card-title"]',
      })
    ).toBeVisible()
    await user.click(
      screen.getByRole('combobox', { name: 'Upstream base URL' })
    )
    const option = screen.getByRole('option', { name: /Legacy supplier/ })
    expect(option).toHaveTextContent('Unidentified URL — kept per channel')
    if (deleted) expect(option).toHaveTextContent('Deleted channel · #24')
    await user.click(option)
    expect(
      (
        screen.getByRole('combobox', {
          name: 'Upstream base URL',
        }) as HTMLInputElement
      ).value
    ).toContain('Legacy supplier')
    expect(
      screen.getByText('Legacy supplier', {
        selector: '[data-slot="card-title"]',
      })
    ).toBeVisible()
    const header = screen
      .getByText('Legacy supplier', { selector: '[data-slot="card-title"]' })
      .closest<HTMLElement>('[data-slot="card-header"]')
    if (!header) throw new Error('Missing upstream card header')
    await user.click(
      within(header).getByRole('button', { name: 'View details' })
    )
    await waitFor(() =>
      expect(api.details).toHaveBeenCalledWith(
        expect.objectContaining({ url_key: 'channel:24' })
      )
    )
    expect(
      within(
        screen.getByRole('dialog', { name: 'Upstream evidence details' })
      ).getByText(/2026-09 · Legacy supplier/)
    ).toBeVisible()
  }
)

it.each(['A', '中', '😀'])(
  'accepts 255 Unicode code points and rejects 256 for upstream names (%s)',
  async (character) => {
    const user = userEvent.setup()
    renderGroup(upstreamGroup())
    await user.click(screen.getByRole('button', { name: 'Add upstream name' }))
    const input = screen.getByRole('textbox', { name: 'Upstream name' })
    await user.click(input)
    await user.paste(`  ${character.repeat(255)}  `)
    expect(input).toHaveValue(`  ${character.repeat(255)}  `)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(api.name).toHaveBeenCalledWith({
        url_key: upstreamGroup().url_key,
        name: character.repeat(255),
      })
    )
    await user.click(
      await screen.findByRole('button', { name: 'Add upstream name' })
    )
    await user.click(screen.getByRole('textbox', { name: 'Upstream name' }))
    await user.paste(character.repeat(256))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(toast.error).toHaveBeenCalledWith('The upstream name is too long.')
    expect(api.name).toHaveBeenCalledTimes(1)
  }
)

it('opens a global evidence category and preserves it in pagination, request search and export', async () => {
  const group = upstreamGroup()
  group.data_quality = {
    status: 'partial',
    usage_without_amount_rows: 2,
    test_amount_pending_reasons: { missing_cache_write: 2 },
  }
  api.details.mockResolvedValue({
    success: true,
    data: { result: { items: [], total: 51, page: 1, page_size: 50 } },
  })
  renderGroup(group)
  const summary = screen
    .getByText('Reconciliation evidence')
    .closest('[role="alert"]') as HTMLElement
  fireEvent.click(
    within(summary).getByRole('button', {
      name: /View records: 2 tests did not save cache-write/,
    })
  )
  await waitFor(() =>
    expect(api.details).toHaveBeenCalledWith(
      expect.objectContaining({
        evidence_filter: 'test:missing_cache_write',
        page: 1,
      })
    )
  )
  expect(api.details.mock.lastCall?.[0].url_key).toBeUndefined()
  const dialog = await screen.findByRole('dialog', {
    name: 'Upstream evidence details',
  })
  fireEvent.click(within(dialog).getByRole('button', { name: 'Next' }))
  await waitFor(() =>
    expect(api.details.mock.lastCall?.[0]).toMatchObject({
      evidence_filter: 'test:missing_cache_write',
      page: 2,
    })
  )
  fireEvent.change(
    within(dialog).getByRole('textbox', { name: 'Our request ID' }),
    { target: { value: 'req-cache' } }
  )
  await waitFor(() =>
    expect(api.details.mock.lastCall?.[0]).toMatchObject({
      evidence_filter: 'test:missing_cache_write',
      request_id: 'req-cache',
      page: 1,
    })
  )
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Export all matching details' })
  )
  await waitFor(() =>
    expect(api.export).toHaveBeenCalledWith(
      expect.objectContaining({
        evidence_filter: 'test:missing_cache_write',
        request_id: 'req-cache',
      })
    )
  )
})

it('keeps channel and model scope when viewing evidence and returns focus on close', async () => {
  const user = userEvent.setup()
  const group = upstreamGroup()
  group.channels = group.channels.slice(0, 1)
  group.channels[0].models = [
    {
      provider_model: 'image-model',
      provider_model_fallback: true,
      customer_models: ['public-image'],
      billing_mode: 'token',
      usage,
      detail_filter: { ...period, channel_id: 11 },
      data_quality: {
        status: 'partial',
        evidence_coverage: { rows: 1, gap_rows: 1, amount_gap_rows: 1 },
      },
    },
  ]
  renderGroup(group)
  await user.click(screen.getByRole('button', { name: 'Expand models' }))
  const status = screen.getByRole('button', { name: /^Usage recorded:/ })
  await user.click(status)
  const trigger = screen.getByRole('button', {
    name: 'View records: Original amount cannot be calculated: 1 records',
  })
  await user.click(trigger)
  await waitFor(() =>
    expect(api.details.mock.lastCall?.[0]).toMatchObject({
      ...period,
      url_key: group.url_key,
      channel_id: 11,
      model_name: 'image-model',
      provider_model_fallback: true,
      billing_mode: 'token',
      evidence_filter: 'amount_gap',
    })
  )
  expect(
    screen.getByRole('heading', { name: 'Upstream evidence details' })
  ).toHaveFocus()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(status).toHaveFocus()
})

it('pages upstream cards on the server and keeps scope-wide evidence totals', async () => {
  const user = userEvent.setup()
  const groups = Array.from({ length: 12 }, (_, i) => ({
    ...upstreamGroup(),
    url_key: `https://page-${i}.example`,
    display_name: `Supplier ${i}`,
    custom_name: `Supplier ${i}`,
    channels: [],
  }))
  const client = renderGroup(groups[0])
  act(() =>
    seedUpstreamPages(client, period, {
      result: {
        url_groups: groups,
        data_quality: {
          status: 'partial',
          evidence_coverage: {
            rows: 120,
            gap_rows: 12,
            amount_gap_rows: 12,
            usage_gap_rows: 0,
            other_gap_rows: 0,
          },
        },
      },
      generated_at: period.start_timestamp,
    })
  )
  expect(
    screen.queryByText('Supplier 10', { selector: '[data-slot="card-title"]' })
  ).not.toBeInTheDocument()
  await user.click(
    within(screen.getByRole('group', { name: 'Upstream pages' })).getByRole(
      'button',
      { name: 'Go to next page' }
    )
  )
  expect(
    await screen.findByText('Supplier 10', {
      selector: '[data-slot="card-title"]',
    })
  ).toBeVisible()
  expect(api.summary).toHaveBeenCalledWith(
    expect.objectContaining({ level: 'groups', page: 2, page_size: 10 }),
    expect.anything()
  )
  expect(
    screen.getByText('12 of 120 billing records have incomplete information')
  ).toBeVisible()
})

it('loads channel and model pages independently without preloading model lists', async () => {
  const user = userEvent.setup()
  const group = upstreamGroup()
  group.channels = Array.from({ length: 12 }, (_, i) => ({
    ...group.channels[0],
    channel_id: i + 1,
    channel_name: `Paged channel ${i + 1}`,
    models: Array.from({ length: 12 }, (_, n) => ({
      provider_model: `model-${n}`,
      billing_mode: 'token',
      customer_models: [],
      usage,
      detail_filter: { ...period, channel_id: i + 1 },
    })),
  }))
  group.channel_count = 12
  renderGroup(group)
  expect(
    api.summary.mock.calls.some(([params]) => params.level === 'models')
  ).toBe(false)
  await user.click(
    within(screen.getByRole('group', { name: 'Channel pages' })).getByRole(
      'button',
      { name: 'Go to next page' }
    )
  )
  expect(await screen.findByText('Paged channel 11')).toBeVisible()
  expect(
    screen.queryByText('Paged channel 1', { exact: true })
  ).not.toBeInTheDocument()
  expect(api.summary).toHaveBeenCalledWith(
    expect.objectContaining({
      level: 'channels',
      url_key: group.url_key,
      page: 2,
    }),
    expect.anything()
  )
  await user.click(screen.getAllByRole('button', { name: 'Expand models' })[0])
  expect(await screen.findByText('— - model-0')).toBeVisible()
  await user.click(
    within(screen.getByRole('group', { name: 'Model pages' })).getByRole(
      'button',
      { name: 'Go to next page' }
    )
  )
  expect(await screen.findByText('— - model-10')).toBeVisible()
  expect(
    screen.queryByText('— - model-0', { exact: true })
  ).not.toBeInTheDocument()
  expect(api.summary).toHaveBeenCalledWith(
    expect.objectContaining({
      level: 'models',
      url_key: group.url_key,
      channel_id: 11,
      page: 2,
      page_size: 10,
    }),
    expect.anything()
  )
})

it('queues one upstream summary without fetching every channel and model page', async () => {
  const user = userEvent.setup()
  const group = upstreamGroup()
  group.channel_count = 101
  renderGroup(group)
  api.summary.mockClear()
  await user.click(screen.getByRole('button', { name: 'Export our statement' }))
  await waitFor(() => expect(api.export).toHaveBeenCalledOnce())
  expect(api.export).toHaveBeenCalledWith({
    job_type: 'upstream_summary',
    start_timestamp: period.start_timestamp,
    end_timestamp: period.end_timestamp + 1,
    url_key: group.url_key,
  })
  expect(api.summary).not.toHaveBeenCalled()
  expect(screen.getByRole('dialog', { name: 'Export jobs' })).toBeVisible()
})

it('keeps a failed upstream submission retryable', async () => {
  api.export.mockRejectedValueOnce(new Error('Queue is full'))
  renderGroup(upstreamGroup())
  const button = screen.getByRole('button', { name: 'Export our statement' })
  fireEvent.click(button)
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Queue is full'))
  expect(button).toBeEnabled()
  expect(
    screen.queryByRole('dialog', { name: 'Export jobs' })
  ).not.toBeInTheDocument()
  fireEvent.click(button)
  await waitFor(() => expect(api.export).toHaveBeenCalledTimes(2))
})

it('paginates URL choices and searches beyond the loaded options', async () => {
  const user = userEvent.setup()
  const groups = Array.from({ length: 25 }, (_, i) => ({
    ...upstreamGroup(),
    url_key: `https://supplier-${i}.example`,
    custom_name: `Supplier ${i}`,
    channels: [],
  }))
  const client = renderGroup(groups[0])
  act(() =>
    seedUpstreamPages(client, period, {
      result: { url_groups: groups },
      generated_at: 1,
    })
  )
  await user.click(screen.getByRole('combobox', { name: 'Upstream base URL' }))
  expect(
    screen.queryByRole('option', { name: /Supplier 24/ })
  ).not.toBeInTheDocument()
  await user.click(
    within(screen.getByRole('group', { name: 'Upstream URL pages' })).getByRole(
      'button',
      { name: 'Go to next page' }
    )
  )
  expect(
    await screen.findByRole('option', { name: /Supplier 24/ })
  ).toBeVisible()
  await user.type(
    screen.getByRole('combobox', { name: 'Upstream base URL' }),
    'Supplier 24'
  )
  await waitFor(() =>
    expect(
      api.summary.mock.calls.some(
        ([p]) =>
          p.level === 'options' && p.search === 'Supplier 24' && p.page === 1
      )
    ).toBe(true)
  )
  await user.click(await screen.findByRole('option', { name: /Supplier 24/ }))
  expect(
    await screen.findByText('Supplier 24', {
      selector: '[data-slot="card-title"]',
    })
  ).toBeVisible()
  expect(
    screen.queryByText('Supplier 0', { selector: '[data-slot="card-title"]' })
  ).not.toBeInTheDocument()
})

it('submits only the upstream whose export button was clicked', async () => {
  const user = userEvent.setup()
  const first = upstreamGroup()
  const second = {
    ...upstreamGroup(),
    url_key: 'https://second.example',
    display_name: 'https://second.example',
  }
  const client = renderGroup(first)
  await act(async () => {
    seedUpstreamPages(client, period, {
      result: { url_groups: [first, second] },
      generated_at: period.start_timestamp,
    })
  })
  await waitFor(() =>
    expect(
      screen.getAllByRole('button', { name: 'Export our statement' })
    ).toHaveLength(2)
  )
  const buttons = screen.getAllByRole('button', {
    name: 'Export our statement',
  })
  await user.click(buttons[1])
  expect(api.export).toHaveBeenCalledExactlyOnceWith({
    job_type: 'upstream_summary',
    start_timestamp: period.start_timestamp,
    end_timestamp: period.end_timestamp + 1,
    url_key: second.url_key,
  })
})

it.each([
  ['2026-09', '2026-10-01T00:00:00+08:00'],
  ['2026-02', '2026-03-01T00:00:00+08:00'],
  ['2024-02', '2024-03-01T00:00:00+08:00'],
  ['2026-12', '2027-01-01T00:00:00+08:00'],
])(
  'exports %s with an exclusive Shanghai month boundary',
  async (month, nextMonth) => {
    const user = userEvent.setup()
    const group = upstreamGroup()
    renderGroup(group, month)
    await user.click(
      screen.getByRole('button', { name: 'Export our statement' })
    )
    expect(api.export).toHaveBeenCalledExactlyOnceWith({
      job_type: 'upstream_summary',
      start_timestamp: Date.parse(`${month}-01T00:00:00+08:00`) / 1000,
      end_timestamp: Date.parse(nextMonth) / 1000,
      url_key: group.url_key,
    })
  }
)
