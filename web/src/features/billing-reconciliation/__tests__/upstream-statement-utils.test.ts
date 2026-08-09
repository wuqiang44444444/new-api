/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { TFunction } from 'i18next'
import { describe, expect, it } from 'vitest'

import type { ProviderChannelSummary } from '../types'
import {
  billingDataQualityReasons,
  buildUpstreamStatementCsv,
  filterProviderChannels,
  formatStatementUsage,
} from '../upstream-statement-utils'

const channels: ProviderChannelSummary[] = [
  {
    channel_id: 18,
    channel_name: '=unsafe-channel',
    usage: {
      requests: 1,
      billable_calls: 0,
      input_tokens: 245,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      output_tokens: 385,
    },
    models: [
      {
        channel_id: 18,
        channel_name: '=unsafe-channel',
        provider_model: '@unsafe-model',
        billing_mode: 'token',
        usage: {
          requests: 1,
          billable_calls: 0,
          input_tokens: 245,
          cache_read_tokens: 0,
          cache_write_tokens: 0,
          output_tokens: 385,
        },
        discount: { value: '1', version: 1, source: 'default' },
        data_quality: { status: 'complete' },
        detail_filter: { start_timestamp: 1, end_timestamp: 2 },
      },
      {
        channel_id: 18,
        channel_name: '=unsafe-channel',
        provider_model: 'other-model',
        billing_mode: 'per_call',
        usage: {
          requests: 2,
          billable_calls: 2,
          input_tokens: 0,
          cache_read_tokens: 0,
          cache_write_tokens: 0,
          output_tokens: 0,
        },
        discount: { value: '1', version: 1, source: 'default' },
        data_quality: { status: 'partial' },
        detail_filter: { start_timestamp: 1, end_timestamp: 2 },
      },
    ],
    data_quality: { status: 'partial' },
  },
]

describe('upstream statement utilities', () => {
  it('filters model child rows without changing authoritative channel totals', () => {
    const filtered = filterProviderChannels(channels, {
      channel: '18',
      model: '@unsafe-model',
    })

    expect(filtered).toHaveLength(1)
    expect(filtered[0]?.models).toHaveLength(1)
    expect(filtered[0]?.usage.input_tokens).toBe(245)
  })

  it('exports one CSV row per visible model and neutralizes formulas', () => {
    const t = ((key: string) => key) as TFunction
    const csv = buildUpstreamStatementCsv({
      channels,
      generatedAt: 1_700_000_000,
      month: '2026-08',
      t,
    })
    const lines = csv.split('\n')

    expect(lines).toHaveLength(3)
    expect(lines[1]).toContain("'=unsafe-channel")
    expect(lines[1]).toContain("'@unsafe-model")
    expect(lines[1]).toContain('245,0,0,385,1,0,Complete')
    expect(lines[2]).toContain('other-model,Per-call billing')
  })

  it('neutralizes signed and whitespace-prefixed spreadsheet formulas', () => {
    const t = ((key: string) => key) as TFunction
    const source = channels[0]
    const model = source?.models[0]
    if (!source || !model) throw new Error('missing statement fixture')
    const csv = buildUpstreamStatementCsv({
      channels: [
        {
          ...source,
          channel_name: ' +SUM(A1:A2)',
          models: [{ ...model, provider_model: '-2+3' }],
        },
      ],
      generatedAt: 1_700_000_000,
      month: '2026-08',
      t,
    })

    expect(csv).toContain("' +SUM(A1:A2)")
    expect(csv).toContain("'-2+3")
  })

  it('keeps small usage exact and compacts large Chinese usage values', () => {
    expect(formatStatementUsage(245, 'zhCN')).toBe('245')
    expect(formatStatementUsage(12_500_000, 'zhCN')).toBe('1,250万')
    expect(formatStatementUsage(132_000_000, 'zh-CN')).toBe('1.32亿')
    expect(formatStatementUsage(null, 'zh-CN')).toBe('—')
  })

  it('explains each partial-data counter with its record count', () => {
    const t = ((key: string, options?: { count?: number }) =>
      key.replace('{{count}}', String(options?.count ?? ''))) as TFunction
    const reasons = billingDataQualityReasons(
      {
        status: 'partial',
        unknown_billing_mode_requests: 7,
        provider_model_fallback_rows: 280,
      },
      t
    )

    expect(reasons).toEqual([
      '7 records do not contain a frozen billing mode.',
      '280 records do not contain the Provider model identity; the customer model is shown instead.',
    ])
  })
})
