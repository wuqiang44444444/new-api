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

import {
  upstreamDiscountLabel,
  upstreamDiscountSourceLabel,
} from '../upstream-reconciliation-utils'
import {
  billingAccountingEntries,
  billingDataQualityReasons,
  billingEvidenceGroups,
  cacheMeterUnavailableLabel,
  formatStatementUsage,
} from '../upstream-statement-utils'

describe('upstream reconciliation utilities', () => {
  it('distinguishes default no-discount from configured and inherited discounts', () => {
    const t = ((key: string, options?: Record<string, string>) =>
      Object.entries(options ?? {}).reduce(
        (text, [name, value]) => text.replaceAll(`{{${name}}}`, value),
        key
      )) as TFunction
    expect(upstreamDiscountLabel(null, t)).toBe('Pending')
    expect(
      upstreamDiscountLabel({ value: '0.8', version: 1, source: 'database' }, t)
    ).toBe('×0.8 (20% off)')
    expect(
      upstreamDiscountLabel({ value: '1', version: 1, source: 'database' }, t)
    ).toBe('No discount (×1)')
    expect(
      upstreamDiscountSourceLabel(
        {
          value: '0.8',
          version: 1,
          source: 'previous_period',
          source_period: 1782979200,
        },
        t
      )
    ).toContain('Inherited from')
    expect(upstreamDiscountSourceLabel(null, t)).toBe('Pending manual fill')
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
        usage_without_amount_rows: 2,
        test_priced_rows: 6,
        auxiliary_charge_rows: 1,
        cache_read_unavailable_requests: 3,
        seconds_unavailable_rows: 4,
        seconds_value_missing_rows: 1,
        input_tokens_unavailable_requests: 5,
        unknown_billing_mode_requests: 7,
        provider_model_fallback_rows: 280,
      },
      t
    )

    expect(reasons).toEqual([
      '2 channel tests did not save the pricing evidence needed to rebuild their amount; usage is retained. The breakdown below partitions these tests.',
      '1 records include tool surcharges; their official price cannot be fully restored yet.',
      '3 billing records lack cache read details.',
      '1 per-second records lack the billable duration used at the time; their amounts cannot be recalculated from usage.',
      '3 per-second records lack the recorded billing unit.',
      '5 records have no confirmed total input usage.',
      '7 records do not contain a frozen billing mode.',
      '280 records do not contain the Provider model identity; the customer model is shown instead.',
      '6 channel tests are priced and included in the amount total.',
      'The remaining 6 priced tests read the amount directly from saved pricing records.',
    ])
  })

  it('shows coefficient 1 with a default source for unconfigured channels', () => {
    const t = ((key: string) => key) as TFunction
    expect(
      upstreamDiscountLabel(
        { value: '1', version: 0, source: 'default' as const },
        t
      )
    ).toBe('No discount (×1)')
    expect(
      upstreamDiscountSourceLabel(
        { value: '1', version: 0, source: 'default' as const },
        t
      )
    ).toBe('Default coefficient (×1)')
  })
})

it('distinguishes optional unreported meters, historical evidence and mixed aggregates', () => {
  const t = ((key: string, options?: { count?: number }) =>
    key.replace('{{count}}', String(options?.count ?? ''))) as TFunction
  const q = {
    status: 'partial' as const,
    cache_read_unavailable_requests: 3,
    cache_read_unreported_requests: 1,
    cache_write_unavailable_requests: 2,
    cache_write_unreported_requests: 2,
  }
  expect(cacheMeterUnavailableLabel(q, 'read', t)).toBe(
    'Cache metering incomplete'
  )
  expect(cacheMeterUnavailableLabel(q, 'write', t)).toBe(
    'Upstream meter not provided'
  )
  expect(
    cacheMeterUnavailableLabel({ status: 'complete' }, 'read', t)
  ).toBeUndefined()
  expect(billingDataQualityReasons(q, t)).toEqual([
    '2 billing records lack cache read details.',
    '2 responses did not provide a usable cache write meter; this does not imply a failed request.',
    '1 responses did not provide a usable cache read meter; this does not imply a failed request.',
  ])
})

describe('Unpriced channel test explanations', () => {
  it('explains the exclusive causes without claiming every row lacks price data', () => {
    const t = ((key: string, options?: { count?: number }) =>
      key.replace('{{count}}', String(options?.count ?? ''))) as TFunction
    const reasons = billingDataQualityReasons(
      {
        status: 'partial',
        usage_without_amount_rows: 22020,
        test_amount_pending_reasons: {
          missing_cache_write: 12926,
          missing_usage_semantic: 9065,
          estimated_usage: 29,
        },
      },
      t,
      false
    )
    expect(reasons).toHaveLength(4)
    expect(reasons.join(' ')).toContain(
      '12926 tests did not save cache-write usage'
    )
    expect(reasons.join(' ')).toContain('9065 tests lack the usage breakdown')
    expect(reasons.join(' ')).toContain('29 tests used estimated usage')
    expect(reasons.join(' ')).toContain('excluded from confirmed totals')
    expect(reasons.join(' ')).toContain('does not mean zero cost')
    expect(reasons.join(' ')).not.toContain('lack reliable pricing records')
  })
})

describe('Grouped reconciliation evidence', () => {
  it('groups amount gaps, usage gaps and confirmed results with closed arithmetic', () => {
    const t = ((key: string, options?: { count?: number | string }) =>
      key.replace('{{count}}', String(options?.count ?? ''))) as TFunction
    const quality = {
      status: 'partial' as const,
      evidence_coverage: {
        rows: 91_816,
        gap_rows: 14_646,
        amount_gap_rows: 8_194,
        usage_gap_rows: 6_452,
        other_gap_rows: 0,
      },
      usage_without_amount_rows: 8_193,
      test_amount_pending_reasons: {
        missing_cache_write: 7_939,
        estimated_usage: 245,
        missing_multimodal_usage: 9,
      },
      legacy_test_cache_write_rows: 7_947,
      cache_write_unavailable_requests: 14_332,
      seconds_task_link_missing_rows: 67,
      test_priced_rows: 14_907,
      test_recomputed_rows: 7_741,
      test_recorded_original_rows: 6_086,
    }

    const groups = billingEvidenceGroups(quality, t)
    expect(groups.map((group) => group.key)).toEqual([
      'amount_gap',
      'usage_gap',
    ])
    expect(groups[0].title).toBe('Amounts that cannot be calculated yet: 8,194 records')
    expect(groups[0].entries.map((entry) => entry.text)).toEqual([
      '8193 channel tests did not save the pricing evidence needed to rebuild their amount; usage is retained. The breakdown below partitions these tests.',
      '245 tests used estimated usage or fees. Their cost is pending and excluded from confirmed totals; this does not mean zero cost.',
      '7939 tests did not save cache-write usage required to recalculate their amount.',
      '9 tests lack the image or audio usage needed by their pricing rule.',
      'Of the unpriced tests above, 7947 also lack cache write details; these are the same records.',
    ])
    expect(groups[1].title).toBe(
      'Amount available, usage details missing: 6,452 records'
    )
    expect(groups[1].entries.map((entry) => entry.text)).toEqual([
      '6385 billing records lack cache write details.',
      '67 historical video records have no task link or recorded billing seconds.',
    ])

    const accounting = billingAccountingEntries(quality, t)
    expect(accounting.map((entry) => entry.text)).toEqual([
      '14907 channel tests are priced and included in the amount total.',
      'Of the priced tests, 7741 match their original settlement when recalculated from saved prices and usage.',
      'Of the priced tests, 6086 use the saved successful expression result with a group multiplier of 1.',
      'The remaining 1080 priced tests read the amount directly from saved pricing records.',
    ])
    // The breakdown has no dedicated evidence filter, so it must not render a
    // clickable link that would open the full priced-test view instead.
    expect(accounting[3].filter).toBe('')
  })

  it('omits group headings when the coverage partition is missing', () => {
    const t = ((key: string, options?: { count?: number }) =>
      key.replace('{{count}}', String(options?.count ?? ''))) as TFunction
    const groups = billingEvidenceGroups(
      {
        status: 'partial',
        evidence_coverage: { rows: 3_920, gap_rows: 750 },
        usage_without_amount_rows: 750,
      },
      t
    )
    expect(groups).toHaveLength(1)
    expect(groups[0].key).toBe('amount_gap')
    expect(groups[0].title).toBe('')
  })
})
