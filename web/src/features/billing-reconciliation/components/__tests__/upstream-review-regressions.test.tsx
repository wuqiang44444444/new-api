import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { ProviderUrlGroupSummary } from '../../types'
import { resolveShanghaiMonth } from '../../lib'
import { UpstreamReconciliationView } from '../upstream-reconciliation'
import { UpstreamReconciliationTable } from '../upstream-reconciliation-table'

const api = vi.hoisted(() => ({ save: vi.fn(), init: vi.fn(), summary: vi.fn() }))
vi.mock('../../api', () => ({ putAdminUpstreamDiscount: api.save,
  postAdminUpstreamDiscountInit: api.init, getAdminUpstreamReconciliation: api.summary }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, vars?: Record<string, unknown>) =>
  key.replace(/{{(\w+)}}/g, (_, name) => String(vars?.[name] ?? name)) }) }))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() } }))
const usage = { requests: 1, billable_calls: 0, input_tokens: 2, output_tokens: 3, cache_read_tokens: 0, cache_write_tokens: 0 }
const group: ProviderUrlGroupSummary = {
  url_key: 'https://billing.example', display_name: 'Upstream', channel_ids: [11], channel_count: 1, model_count: 1,
  usage, reference_known: true, discount_pending_channels: 0,
  channel_discounts: [{ channel_id: 11, channel_name: 'Channel', discount: { value: '0.8', version: 1, source: 'database' } }],
  models: [{ provider_model: 'unmapped', provider_model_fallback: true, billing_mode: 'token', usage,
    channels: [{ channel_id: 11, channel_name: 'Channel', provider_model: 'unmapped', provider_model_fallback: true,
      customer_models: ['unmapped'], billing_mode: 'token', usage, detail_filter: { start_timestamp: 1, end_timestamp: 2 }, discount: null }] }],
}
beforeEach(() => { vi.clearAllMocks() })
it('clears the discount editor when switching to a cached month', () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  for (const month of ['2026-08', '2026-09']) {
    const period = resolveShanghaiMonth(month)
    client.setQueryData(['billing-upstream-reconciliation', period.start_timestamp, period.end_timestamp],
      { result: { url_groups: [group] }, generated_at: period.start_timestamp })
  }
  const view = (month: string) => <QueryClientProvider client={client}><UpstreamReconciliationView
    month={month} period={resolveShanghaiMonth(month)} onMonthChange={vi.fn()} /></QueryClientProvider>
  const rendered = render(view('2026-08'))
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
  fireEvent.change(screen.getByLabelText('Discount percent'), { target: { value: '70' } })
  rendered.rerender(view('2026-09'))
  expect(screen.queryByLabelText('Discount percent')).not.toBeInTheDocument()
  expect(api.save).not.toHaveBeenCalled()
  expect(screen.getByText(/Asia\/Shanghai, 2026-09-01 00:00:00 \+08:00/)).toBeInTheDocument()
})
it('preserves fallback model identity in both model and channel detail links', () => {
  const onViewDetails = vi.fn()
  render(<UpstreamReconciliationTable groups={[group]} expandedGroups={new Set([group.url_key])}
    expandedModels={new Set([`${group.url_key}|unmapped|token|1`])} onToggleGroup={vi.fn()}
    onToggleModel={vi.fn()} onViewDetails={onViewDetails} />)
  const links = screen.getAllByRole('button', { name: 'View details' })
  fireEvent.click(links[1])
  expect(onViewDetails).toHaveBeenLastCalledWith({ urlKey: group.url_key, providerModel: 'unmapped', providerModelFallback: true, billingMode: 'token' })
  fireEvent.click(links[2])
  expect(onViewDetails).toHaveBeenLastCalledWith({ urlKey: group.url_key, channelId: 11, providerModel: 'unmapped', providerModelFallback: true, billingMode: 'token' })
})
