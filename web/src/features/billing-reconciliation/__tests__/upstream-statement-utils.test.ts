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

import type { ProviderUrlGroupSummary } from '../types'
import {
  buildUpstreamReconciliationCsv,
  upstreamDiscountLabel,
  upstreamDiscountSourceLabel,
} from '../upstream-reconciliation-utils'
import {
  billingDataQualityReasons,
  formatStatementUsage,
} from '../upstream-statement-utils'

const groups: ProviderUrlGroupSummary[] = [
  {
    url_key: 'https://api.example.com',
    display_name: 'https://api.example.com',
    base_url: 'https://api.example.com',
    channel_ids: [18, 19],
    channel_count: 2,
    model_count: 1,
    usage: {
      requests: 3,
      billable_calls: 2,
      input_tokens: 245,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      output_tokens: 385,
    },
    models: [
      {
        provider_model: 'shared-model',
        billing_mode: 'token',
        usage: {
          requests: 3,
          billable_calls: 0,
          input_tokens: 245,
          cache_read_tokens: 0,
          cache_write_tokens: 0,
          output_tokens: 385,
        },
        data_quality: { status: 'complete' },
        original_amount: 3000,
        channels: [
          {
            channel_id: 18,
            channel_name: '=unsafe-channel',
            provider_model: 'shared-model',
            customer_models: ['@display-model'],
            billing_mode: 'token',
            usage: {
              requests: 1,
              billable_calls: 0,
              input_tokens: 245,
              cache_read_tokens: 0,
              cache_write_tokens: 0,
              output_tokens: 385,
            },
            discount: { value: '0.8', version: 3, source: 'database' },
            data_quality: { status: 'complete' },
            detail_filter: { start_timestamp: 1, end_timestamp: 2 },
            original_amount: 1000,
            reference_amount: 800,
          },
          {
            channel_id: 19,
            channel_name: 'pending-channel',
            provider_model: 'shared-model',
            customer_models: ['@display-model'],
            billing_mode: 'token',
            usage: {
              requests: 2,
              billable_calls: 0,
              input_tokens: 0,
              cache_read_tokens: 0,
              cache_write_tokens: 0,
              output_tokens: 0,
            },
            discount: null,
            data_quality: { status: 'partial' },
            detail_filter: { start_timestamp: 1, end_timestamp: 2 },
            original_amount: 2000,
          },
        ],
      },
    ],
    channel_discounts: [],
    reference_known: false,
    discount_pending_channels: 1,
    data_quality: { status: 'partial' },
  },
]

describe('upstream reconciliation utilities', () => {
  it('exports one CSV row per channel leaf with discount value and version', () => {
    const t = ((key: string) => key) as TFunction
    const csv = buildUpstreamReconciliationCsv({
      groups,
      generatedAt: 1_700_000_000,
      month: '2026-08',
      period: { start_timestamp: 1_782_835_200, end_timestamp: 1_785_513_599 },
      t,
    })
    const lines = csv.split('\n')

    expect(lines).toHaveLength(3)
    expect(lines[0]).toContain('Period start (Asia/Shanghai)')
    expect(lines[1]).toContain('2026-07-01 00:00:00 +08:00')
    expect(lines[1]).toContain('2026-07-31 23:59:59 +08:00')
    expect(lines[1]).toContain("'=unsafe-channel")
    expect(lines[1]).toContain("'@display-model - shared-model")
    expect(lines[1]).toContain('0.8,3')
    expect(lines[1]).toContain(',0.00160000,')
    expect(lines[2]).toContain('Pending,')
    expect(lines[2]).toContain(',Incomplete,')
  })

  it('neutralizes signed and whitespace-prefixed spreadsheet formulas', () => {
    const t = ((key: string) => key) as TFunction
    const source = structuredClone(groups)
    const model = source[0]?.models[0]
    const leaf = model?.channels[0]
    if (!model || !leaf) throw new Error('missing statement fixture')
    leaf.channel_name = ' +SUM(A1:A2)'
    leaf.customer_models = ['-display']
    leaf.provider_model = '-2+3'
    const csv = buildUpstreamReconciliationCsv({
      groups: source,
      generatedAt: 1_700_000_000,
      month: '2026-08',
      period: { start_timestamp: 1_782_835_200, end_timestamp: 1_785_513_599 },
      t,
    })

    expect(csv).toContain("' +SUM(A1:A2)")
    expect(csv).toContain("'-display - -2+3")
  })

  it('exports missing historical cache writes as unrecorded', () => {
    const source = structuredClone(groups)
    const model = source[0]?.models[0]
    const leaf = model?.channels[0]
    if (!model || !leaf) throw new Error('Missing model fixture')
    leaf.data_quality = {
      status: 'partial',
      cache_write_unavailable_requests: 1,
    }
    const csv = buildUpstreamReconciliationCsv({
      groups: source,
      generatedAt: 1700000000,
      month: '2026-09',
      period: { start_timestamp: 1_782_835_200, end_timestamp: 1_785_513_599 },
      t: ((key: string) => key) as TFunction,
    })
    expect(csv).toContain('Not recorded')
    expect(
      billingDataQualityReasons(
        leaf.data_quality,
        ((key: string) => key) as TFunction
      )
    ).toContain(
      '{{count}} channel test records have no recorded cache write usage.'
    )
  })

  it('distinguishes default no-discount from configured and inherited discounts', () => {
    const t = ((key: string, options?: Record<string, string>) =>
      Object.entries(options ?? {}).reduce(
        (text, [name, value]) => text.replaceAll(`{{${name}}}`, value),
        key
      )) as TFunction
    expect(
      upstreamDiscountLabel(
        { channel_id: 1, channel_name: 'a', discount: null },
        t
      )
    ).toBe('Pending')
    expect(
      upstreamDiscountLabel(
        {
          channel_id: 1,
          channel_name: 'a',
          discount: { value: '0.8', version: 1, source: 'database' },
        },
        t
      )
    ).toBe('×0.8 (20% off)')
    expect(
      upstreamDiscountLabel(
        {
          channel_id: 1,
          channel_name: 'a',
          discount: { value: '1', version: 1, source: 'database' },
        },
        t
      )
    ).toBe('No discount (×1)')
    expect(
      upstreamDiscountSourceLabel(
        {
          channel_id: 1,
          channel_name: 'a',
          discount: {
            value: '0.8',
            version: 1,
            source: 'previous_period',
            source_period: 1782979200,
          },
        },
        t
      )
    ).toContain('Inherited from')
    expect(
      upstreamDiscountSourceLabel(
        { channel_id: 1, channel_name: 'a', discount: null },
        t
      )
    ).toBe('Pending manual fill')
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

  it('shows coefficient 1 with a default source for unconfigured channels', () => {
    const t = ((key: string) => key) as TFunction
    const status = {
      channel_id: 1,
      channel_name: 'a',
      discount: { value: '1', version: 0, source: 'default' as const },
    }
    expect(upstreamDiscountLabel(status, t)).toBe('No discount (×1)')
    expect(upstreamDiscountSourceLabel(status, t)).toBe(
      'Default coefficient (×1)'
    )
  })

  it('preserves all eight decimal places of a frozen coefficient', () => {
    const source = structuredClone(groups)
    const discount = source[0]?.models[0]?.channels[0]?.discount
    if (!discount) throw new Error('missing discount fixture')
    discount.value = '0.12345678'
    const csv = buildUpstreamReconciliationCsv({
      groups: source,
      generatedAt: 1700000000,
      month: '2026-09',
      period: { start_timestamp: 1788278400, end_timestamp: 1790870399 },
      t: ((key: string) => key) as TFunction,
    })
    expect(csv).toContain('0.12345678,3')
    expect(csv).toContain('Currency,quota_per_unit,currency_rate')
  })
})

it('exports signed refund amounts as numeric CSV cells', () => {
  const source = structuredClone(groups)
  const leaf = source[0]?.models[0]?.channels[0]
  if (!leaf) throw new Error('missing channel fixture')
  leaf.original_amount = -500000
  leaf.reference_amount = -400000
  const csv = buildUpstreamReconciliationCsv({
    groups: source,
    generatedAt: 1700000000,
    month: '2026-09',
    period: { start_timestamp: 1788278400, end_timestamp: 1790870399 },
    t: ((key: string) => key) as TFunction,
  })
  expect(csv).toContain(',-1.00000000,-0.80000000,')
  expect(csv).not.toContain("'-1.00000000")
})
