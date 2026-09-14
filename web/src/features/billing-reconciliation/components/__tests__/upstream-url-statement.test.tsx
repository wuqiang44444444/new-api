/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
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
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import * as billingApi from '../../api'
import type { ProviderUrlGroupSummary, ProviderUrlSummary } from '../../types'
import { UpstreamUrlStatementView } from '../upstream-url-statement'

const period = { start_timestamp: 1788192000, end_timestamp: 1790783999 }
const usage = {
  requests: 2,
  billable_calls: 0,
  input_tokens: 300,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  output_tokens: 40,
}

const channelRow = (channelId: number, name: string) => ({
  channel_id: channelId,
  channel_name: name,
  provider_model: 'model-a',
  customer_models: ['customer-a'],
  billing_mode: 'token' as const,
  usage,
  detail_filter: {
    ...period,
    channel_id: channelId,
    model_name: 'customer-a',
    billing_mode: 'token' as const,
  },
})

const mergedGroup: ProviderUrlGroupSummary = {
  url_key: 'https://api.example.com',
  display_name: 'https://api.example.com',
  base_url: 'https://api.example.com',
  channel_ids: [21, 22],
  channel_count: 2,
  model_count: 1,
  usage,
  models: [
    {
      provider_model: 'model-a',
      billing_mode: 'token',
      usage,
      channels: [channelRow(21, 'alpha'), channelRow(22, 'beta')],
    },
  ],
}

const unidentifiedGroup: ProviderUrlGroupSummary = {
  url_key: 'channel:24',
  display_name: 'delta',
  unidentified: true,
  channel_ids: [24],
  channel_count: 1,
  model_count: 1,
  usage,
  models: [
    {
      provider_model: 'model-a',
      provider_model_fallback: true,
      billing_mode: 'unknown',
      usage,
      channels: [channelRow(24, 'delta')],
    },
  ],
}

const result = (groups: ProviderUrlGroupSummary[]): ProviderUrlSummary => ({
  url_groups: groups,
  data_quality: { status: 'complete' },
})

const envelope = (groups: ProviderUrlGroupSummary[]) => ({
  success: true,
  message: '',
  data: {
    period: {
      ...period,
      period_start: period.start_timestamp,
      timezone: 'Asia/Shanghai' as const,
    },
    filters: {},
    result: result(groups),
    generated_at: 1788192200,
    data_version: period.end_timestamp,
    data_source: 'database',
  },
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function renderView() {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const content = (month: string, currentPeriod: typeof period) => (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <UpstreamUrlStatementView
          month={month}
          period={currentPeriod}
          onMonthChange={(next) => view.rerender(content(next, nextPeriod))}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  const nextPeriod = {
    start_timestamp: period.end_timestamp + 1,
    end_timestamp: 1793462399,
  }
  const view = render(content('2026-09', period))
  return { view, client, nextPeriod }
}

it('merges channels by URL, filters by URL, and exports the filtered scope', async () => {
  const user = userEvent.setup()
  const api = vi
    .spyOn(billingApi, 'getAdminUpstreamUrlStatement')
    .mockImplementation(async (params) =>
      'url_key' in params
        ? envelope([mergedGroup])
        : envelope([mergedGroup, unidentifiedGroup])
    )
  await renderView()

  await screen.findByText('https://api.example.com')
  expect(screen.getByText('delta')).toBeTruthy()
  expect(
    screen.getByText(/Usage is grouped by each channel's current base URL/)
  ).toBeTruthy()

  await user.click(screen.getByRole('combobox', { name: 'Upstream base URL' }))
  await user.click(
    await screen.findByRole('option', { name: 'https://api.example.com' })
  )
  await waitFor(() =>
    expect(api).toHaveBeenCalledWith({
      ...period,
      url_key: 'https://api.example.com',
    })
  )
  await waitFor(() =>
    expect(screen.queryByText('Unidentified URL — kept per channel')).toBeNull()
  )

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
  // The CSV follows the filtered scope: alpha and beta rows are present, the
  // unidentified group filtered out is absent.
  expect(csv).toContain('https://api.example.com')
  expect(csv).toContain('alpha')
  expect(csv).not.toContain('Unidentified URL — kept per channel')
})

it('expands a URL group into models and channel detail rows', async () => {
  const user = userEvent.setup()
  vi.spyOn(billingApi, 'getAdminUpstreamUrlStatement').mockResolvedValue(
    envelope([mergedGroup])
  )
  await renderView()

  await screen.findByText('https://api.example.com')
  // The merged model row is visible without a pseudo detail entry.
  expect(screen.getByRole('columnheader', { name: 'Requests' })).toBeTruthy()
  expect(screen.getByText('model-a')).toBeTruthy()
  for (const label of ['https://api.example.com', 'model-a']) {
    const row = screen.getByText(label).closest('tr')
    if (!row) throw new Error('Missing usage row')
    expect(within(row).getAllByRole('cell')[2]?.textContent).toBe('2')
  }

  expect(screen.queryByRole('link', { name: /View details/ })).toBeNull()

  await user.click(screen.getByRole('button', { name: /Expand channels/ }))
  const alphaRow = screen.getByText('alpha').closest('tr')
  if (!alphaRow) throw new Error('Missing channel detail row')
  expect(within(alphaRow).getAllByRole('cell')[2]?.textContent).toBe('2')
  expect(
    within(alphaRow).getByRole('link', { name: /View details/ })
  ).toHaveProperty('href', expect.stringContaining('channel=21'))
  await user.click(screen.getByRole('button', { name: /Collapse channels/ }))
  expect(screen.queryByText('alpha')).toBeNull()
})

it('shows load failure and retries the current statement', async () => {
  const user = userEvent.setup()
  const api = vi
    .spyOn(billingApi, 'getAdminUpstreamUrlStatement')
    .mockRejectedValueOnce(new Error('Temporary outage'))
    .mockResolvedValue(envelope([mergedGroup]))
  await renderView()
  await screen.findByText('Temporary outage')
  expect(
    screen.queryByRole('button', { name: 'Export our statement' })
  ).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByText('https://api.example.com')
  expect(api).toHaveBeenCalledTimes(2)
})

it('shows an empty statement and disables export', async () => {
  vi.spyOn(billingApi, 'getAdminUpstreamUrlStatement').mockResolvedValue(
    envelope([])
  )
  await renderView()
  await screen.findByText('No upstream usage')
  expect(
    screen.getByRole('button', { name: 'Export our statement' })
  ).toHaveProperty('disabled', true)
})

it('switches billing periods without exporting the previous period while pending or failed', async () => {
  let rejectNext!: (reason: Error) => void
  const api = vi
    .spyOn(billingApi, 'getAdminUpstreamUrlStatement')
    .mockResolvedValueOnce(envelope([mergedGroup]))
    .mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectNext = reject
        })
    )
  const { nextPeriod } = await renderView()
  await screen.findByText('https://api.example.com')
  const month = document.querySelector('input[type="month"]')
  if (!month) throw new Error('Missing month input')
  fireEvent.change(month, { target: { value: '2026-10' } })
  await waitFor(() => expect(api).toHaveBeenLastCalledWith(nextPeriod))
  expect(screen.queryByText('https://api.example.com')).toBeNull()
  expect(
    screen.queryByRole('button', { name: 'Export our statement' })
  ).toBeNull()
  await act(async () => rejectNext(new Error('October unavailable')))
  await screen.findByText('October unavailable')
  expect(
    screen.queryByRole('button', { name: 'Export our statement' })
  ).toBeNull()
})

it('disables filtered export while its request is pending and after it fails', async () => {
  const user = userEvent.setup()
  let rejectFilter!: (reason: Error) => void
  vi.spyOn(billingApi, 'getAdminUpstreamUrlStatement').mockImplementation(
    (params) =>
      'url_key' in params
        ? new Promise((_, reject) => {
            rejectFilter = reject
          })
        : Promise.resolve(envelope([mergedGroup, unidentifiedGroup]))
  )
  await renderView()
  await screen.findByText('https://api.example.com')
  await user.click(screen.getByRole('combobox', { name: 'Upstream base URL' }))
  await user.click(
    await screen.findByRole('option', { name: 'https://api.example.com' })
  )
  expect(
    screen.getByRole('button', { name: 'Export our statement' })
  ).toHaveProperty('disabled', true)
  await act(async () => rejectFilter(new Error('Filtered scope unavailable')))
  await screen.findByText('Filtered scope unavailable')
  expect(
    screen.getByRole('button', { name: 'Export our statement' })
  ).toHaveProperty('disabled', true)
})

it('uses model identities rather than billing-mode row counts in its summary', async () => {
  const firstModel = mergedGroup.models[0]
  if (!firstModel) throw new Error('Missing model fixture')
  const multiMode = {
    ...mergedGroup,
    models: [
      ...mergedGroup.models,
      { ...firstModel, billing_mode: 'per_call' as const },
    ],
  }
  vi.spyOn(billingApi, 'getAdminUpstreamUrlStatement').mockResolvedValue(
    envelope([multiMode])
  )
  await renderView()
  expect(await screen.findByText(/1 URL groups · 1 models/)).toBeTruthy()
})
