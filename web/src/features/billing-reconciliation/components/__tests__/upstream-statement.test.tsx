import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import * as billingApi from '../../api'
import type { ProviderModelSummary, ProviderSummary } from '../../types'
import { UpstreamStatementView } from '../upstream-statement'

const period = { start_timestamp: 1788192000, end_timestamp: 1790783999 }
const usage = {
  requests: 1,
  billable_calls: 0,
  input_tokens: 45,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  output_tokens: 12,
}
const model: ProviderModelSummary = {
  channel_id: 18,
  channel_name: 'Channel A',
  provider_model: 'model-b',
  customer_models: ['customer-b'],
  billing_mode: 'token',
  usage,
  discount: {
    value: '1',
    version: 1,
    source: 'database',
    source_period: period.start_timestamp,
  },
  detail_filter: {
    ...period,
    channel_id: 18,
    model_name: 'customer-b',
    billing_mode: 'token',
  },
  data_quality: { status: 'complete' },
}
const filtered: ProviderSummary = {
  channels: [
    {
      channel_id: 18,
      channel_name: 'Channel A',
      usage,
      models: [model],
      data_quality: { status: 'complete' },
    },
  ],
  data_quality: { status: 'complete' },
}
const all: ProviderSummary = {
  channels: [
    {
      channel_id: 18,
      channel_name: 'Channel A',
      usage: { ...usage, requests: 2, input_tokens: 245 },
      models: [
        {
          ...model,
          provider_model: 'model-a',
          customer_models: ['customer-a'],
          usage: { ...usage, input_tokens: 200 },
        },
        model,
      ],
      data_quality: { status: 'partial', cache_write_unavailable_requests: 1 },
    },
  ],
  data_quality: { status: 'partial', cache_write_unavailable_requests: 1 },
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

it('queries selected model totals and exports the same scope while retaining filter choices', async () => {
  const user = userEvent.setup()
  const api = vi
    .spyOn(billingApi, 'getAdminUpstreamStatement')
    .mockImplementation(async (params) => ({
      success: true,
      message: '',
      data: {
        period: {
          ...period,
          period_start: period.start_timestamp,
          timezone: 'Asia/Shanghai' as const,
        },
        filters: {},
        result: 'model_name' in params ? filtered : all,
        generated_at: 1788192200,
        data_version: period.end_timestamp,
        data_source: 'database',
      },
    }))
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UpstreamStatementView
          month='2026-09'
          period={period}
          onMonthChange={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByText('customer-b - model-b')
  await user.click(screen.getByRole('combobox', { name: 'Model' }))
  await user.click(await screen.findByRole('option', { name: 'model-b' }))
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith({ ...period, model_name: 'model-b' })
  )
  await waitFor(() =>
    expect(screen.queryByText('customer-a - model-a')).toBeNull()
  )
  const parent = screen
    .getByText('Channel A', { selector: '.font-semibold' })
    .closest('tr')
  if (!parent) throw new Error('Missing channel total row')
  expect(within(parent).getByText('45')).toBeTruthy()
  expect(within(parent).getByText('Complete')).toBeTruthy()
  expect(screen.queryByText('245')).toBeNull()
  const createUrl = vi.fn<(blob: Blob) => string>(() => 'blob:statement')
  vi.stubGlobal(
    'URL',
    class extends URL {
      static createObjectURL = createUrl
      static revokeObjectURL = vi.fn()
    }
  )
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  await user.click(screen.getByRole('button', { name: 'Export our statement' }))
  const blob = createUrl.mock.calls[0]?.[0]
  if (!blob) throw new Error('No statement exported')
  const csv = await new Promise<string>((resolve) => {
    const reader = new FileReader()
    reader.addEventListener('load', () => resolve(String(reader.result)), {
      once: true,
    })
    reader.readAsText(blob)
  })
  expect(csv).toContain('customer-b - model-b')
  expect(csv).not.toContain('customer-a - model-a')
  await user.click(screen.getByRole('combobox', { name: 'Model' }))
  expect(await screen.findByRole('option', { name: 'model-a' })).toBeTruthy()
  view.unmount()
  client.clear()
})

it('keeps filters usable and prevents stale exports while selected data is pending or failed', async () => {
  const user = userEvent.setup()
  const response = {
    success: true,
    message: '',
    data: {
      period: {
        ...period,
        period_start: period.start_timestamp,
        timezone: 'Asia/Shanghai' as const,
      },
      filters: {},
      result: all,
      generated_at: 1788192200,
      data_version: period.end_timestamp,
      data_source: 'database' as const,
    },
  }
  let rejectSelected: (reason: Error) => void = () => {}
  const selected = new Promise<typeof response>((_resolve, reject) => {
    rejectSelected = reject
  })
  vi.spyOn(billingApi, 'getAdminUpstreamStatement').mockImplementation(
    (params) => ('channel_id' in params ? selected : Promise.resolve(response))
  )
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const view = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UpstreamStatementView
          month='2026-09'
          period={period}
          onMonthChange={vi.fn()}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  await screen.findByText('customer-b - model-b')
  await user.click(screen.getByRole('combobox', { name: 'Upstream channel' }))
  await user.click(await screen.findByRole('option', { name: 'Channel A' }))
  expect(
    screen
      .getByRole('button', { name: 'Export our statement' })
      .hasAttribute('disabled')
  ).toBe(true)
  expect(screen.queryByText('245')).toBeNull()
  expect(screen.getByRole('combobox', { name: 'Model' })).toBeTruthy()
  rejectSelected(new Error('Selected statement unavailable'))
  await screen.findByText('Selected statement unavailable')
  expect(
    screen
      .getByRole('button', { name: 'Export our statement' })
      .hasAttribute('disabled')
  ).toBe(true)
  await user.click(screen.getByRole('combobox', { name: 'Upstream channel' }))
  await user.click(await screen.findByRole('option', { name: 'All channels' }))
  await screen.findByText('customer-b - model-b')
  expect(
    screen
      .getByRole('button', { name: 'Export our statement' })
      .hasAttribute('disabled')
  ).toBe(false)
  view.unmount()
  client.clear()
})
