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
import { createInstance } from 'i18next'
import { expect, it } from 'vitest'

import type { ProviderUrlGroupSummary, ProviderUrlModelSummary } from '../types'
import {
  buildUpstreamUrlStatementCsv,
  upstreamUrlGroupLabel,
} from '../upstream-url-statement-utils'

const i18n = createInstance()
await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
const t = i18n.t

const usage = {
  requests: 2,
  billable_calls: 1,
  input_tokens: 100,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  output_tokens: 20,
}

const channelRow = (channelId: number, name: string) => ({
  channel_id: channelId,
  channel_name: name,
  provider_model: 'model-a',
  customer_models: ['customer-a'],
  billing_mode: 'token' as const,
  usage,
  detail_filter: {
    start_timestamp: 1000,
    end_timestamp: 2000,
    channel_id: channelId,
    model_name: 'customer-a',
    billing_mode: 'token' as const,
  },
})

const modelRow: ProviderUrlModelSummary = {
  provider_model: 'model-a',
  billing_mode: 'token',
  usage,
  channels: [channelRow(21, 'alpha'), channelRow(22, 'beta')],
}

const identifiedGroup: ProviderUrlGroupSummary = {
  url_key: 'https://api.example.com',
  display_name: 'https://api.example.com',
  base_url: 'https://api.example.com',
  channel_ids: [21, 22],
  channel_count: 2,
  model_count: 1,
  usage,
  models: [modelRow],
}

const unidentifiedGroup: ProviderUrlGroupSummary = {
  url_key: 'channel:24',
  display_name: 'delta',
  unidentified: true,
  deleted: true,
  channel_ids: [24],
  channel_count: 1,
  model_count: 1,
  usage,
  models: [
    {
      provider_model: 'ghost-model',
      provider_model_fallback: true,
      billing_mode: 'unknown',
      usage,
      channels: [channelRow(24, 'Channel #24')],
    },
  ],
}

it('labels identified groups by URL and unidentified groups per channel', () => {
  expect(upstreamUrlGroupLabel(identifiedGroup, t)).toBe(
    'https://api.example.com'
  )
  expect(upstreamUrlGroupLabel(unidentifiedGroup, t)).toContain(
    'Deleted channel'
  )
})

it('exports one CSV row per channel leaf without parent totals', () => {
  const csv = buildUpstreamUrlStatementCsv({
    groups: [identifiedGroup, unidentifiedGroup],
    generatedAt: 1788192200,
    month: '2026-09',
    t,
  })
  const lines = csv.split('\n')
  // header + 2 channel rows of the merged group + 1 channel row of the
  // unidentified group; URL and model parent totals are not exported.
  expect(lines).toHaveLength(4)
  expect(csv).toContain('https://api.example.com')
  expect(csv).toContain('Current channel base URL')
  expect(csv).toContain('alpha')
  expect(csv).toContain('beta')
  expect(csv).toContain('Unidentified URL — kept per channel')
})
