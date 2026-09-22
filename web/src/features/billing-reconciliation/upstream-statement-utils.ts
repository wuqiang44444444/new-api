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

import type {
  BillingDataQuality,
  ProviderUrlChannelGroupSummary,
  ProviderUrlChannelModelSummary,
} from './types'

export function upstreamModelLabel(
  model: Pick<
    ProviderUrlChannelModelSummary,
    'provider_model' | 'provider_model_fallback' | 'customer_models'
  >
): string {
  const customerModels = model.customer_models.join(' / ') || '—'
  const providerModel = model.provider_model_fallback
    ? '—'
    : model.provider_model
  return `${customerModels} - ${providerModel}`
}

export function billingDataQualityLabel(
  quality: BillingDataQuality | undefined,
  t: TFunction
) {
  if (!quality || quality.status === 'unavailable') return t('Unavailable')
  if (quality.usage_without_amount_rows) return t('Pricing evidence incomplete')
  if (
    quality.unavailable_requests ||
    quality.unknown_billing_mode_requests ||
    quality.provider_model_fallback_rows
  ) {
    return t('Record details incomplete')
  }
  if (
    quality.input_tokens_unavailable_requests ||
    quality.seconds_unavailable_rows
  ) {
    return t('Usage incomplete')
  }
  if (quality.auxiliary_charge_rows) return t('Price restore blocked')
  if (quality.missing_historical_price_rows) return t('Price evidence missing')
  if (
    quality.cache_read_unavailable_requests ||
    quality.cache_write_unavailable_requests
  ) {
    return t('Cache metering notes')
  }
  return t('Usage recorded')
}

export type BillingEvidenceEntry = { filter: string; text: string }

// Evidence lines are grouped the way operators read them: money that cannot be
// rebuilt, money that is settled but misses a usage dimension, and other
// row-level gaps. Confirmed accounting results stay in their own block so
// positive notes never read as failures.
export type BillingEvidenceGroup = {
  key: 'amount_gap' | 'usage_gap' | 'other_gap'
  title: string
  entries: BillingEvidenceEntry[]
}

// Unpriced channel tests dominate the amount gaps. The exclusive test reasons
// come first and partition their total; the remaining lines are separate
// row-level facts that may overlap the tests.
function billingAmountGapEntries(
  quality: BillingDataQuality,
  t: TFunction
): BillingEvidenceEntry[] {
  const reasons: BillingEvidenceEntry[] = []
  if (quality.usage_without_amount_rows) {
    reasons.push({
      filter: 'usage_without_amount_rows',
      text: t(
        '{{count}} channel tests did not save the pricing evidence needed to rebuild their amount; usage is retained. The breakdown below partitions these tests.',
        { count: quality.usage_without_amount_rows }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.estimated_usage) {
    reasons.push({
      filter: 'test:estimated_usage',
      text: t(
        '{{count}} tests used estimated usage or fees. Their cost is pending and excluded from confirmed totals; this does not mean zero cost.',
        { count: quality.test_amount_pending_reasons.estimated_usage }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_cache_write) {
    reasons.push({
      filter: 'test:missing_cache_write',
      text: t(
        '{{count}} tests did not save cache-write usage required to recalculate their amount.',
        { count: quality.test_amount_pending_reasons.missing_cache_write }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_cache_read) {
    reasons.push({
      filter: 'test:missing_cache_read',
      text: t(
        '{{count}} tests did not save cache-read usage required by their pricing rule.',
        { count: quality.test_amount_pending_reasons.missing_cache_read }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_cache_ttl) {
    reasons.push({
      filter: 'test:missing_cache_ttl',
      text: t(
        '{{count}} tests lack the cache-write duration breakdown required by their pricing rule.',
        { count: quality.test_amount_pending_reasons.missing_cache_ttl }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_multimodal_usage) {
    reasons.push({
      filter: 'test:missing_multimodal_usage',
      text: t(
        '{{count}} tests lack the image or audio usage needed by their pricing rule.',
        { count: quality.test_amount_pending_reasons.missing_multimodal_usage }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_usage_semantic) {
    reasons.push({
      filter: 'test:missing_usage_semantic',
      text: t(
        '{{count}} tests lack the usage breakdown needed to apply their pricing rule.',
        { count: quality.test_amount_pending_reasons.missing_usage_semantic }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.fixed_price_mismatch) {
    reasons.push({
      filter: 'test:fixed_price_mismatch',
      text: t(
        '{{count}} fixed-price tests have a recorded amount that differs from the saved unit price.',
        { count: quality.test_amount_pending_reasons.fixed_price_mismatch }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_price_fields) {
    reasons.push({
      filter: 'test:missing_price_fields',
      text: t(
        '{{count}} tests did not save the price fields needed to calculate an amount.',
        { count: quality.test_amount_pending_reasons.missing_price_fields }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.invalid_pricing_record) {
    reasons.push({
      filter: 'test:invalid_pricing_record',
      text: t(
        '{{count}} tests contain invalid or unsupported pricing records.',
        {
          count: quality.test_amount_pending_reasons.invalid_pricing_record,
        }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_cache_write_price) {
    reasons.push({
      filter: 'test:missing_cache_write_price',
      text: t(
        '{{count}} tests have cache writes but lack the historical cache-write price.',
        { count: quality.test_amount_pending_reasons.missing_cache_write_price }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_cache_read_price) {
    reasons.push({
      filter: 'test:missing_cache_read_price',
      text: t(
        '{{count}} tests have cache reads but lack the historical cache-read price.',
        { count: quality.test_amount_pending_reasons.missing_cache_read_price }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_expression_context) {
    reasons.push({
      filter: 'test:missing_expression_context',
      text: t(
        '{{count}} tests lack the original request parameters or pricing time required by their rule.',
        {
          count: quality.test_amount_pending_reasons.missing_expression_context,
        }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_group_ratio) {
    reasons.push({
      filter: 'test:missing_group_ratio',
      text: t(
        '{{count}} expression tests lack the group multiplier needed to verify their recorded amount.',
        { count: quality.test_amount_pending_reasons.missing_group_ratio }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.recorded_amount_mismatch) {
    reasons.push({
      filter: 'test:recorded_amount_mismatch',
      text: t(
        '{{count}} tests have a recorded amount that disagrees with the frozen rule and usage.',
        { count: quality.test_amount_pending_reasons.recorded_amount_mismatch }
      ),
    })
  }
  if (quality.test_amount_pending_reasons?.missing_tool_price) {
    reasons.push({
      filter: 'test:missing_tool_price',
      text: t(
        '{{count}} tests include tool fees without a complete historical fee breakdown.',
        { count: quality.test_amount_pending_reasons.missing_tool_price }
      ),
    })
  }
  if (quality.legacy_test_cache_read_rows) {
    reasons.push({
      filter: 'legacy_test_cache_read_rows',
      text: t(
        'Of the unpriced tests above, {{count}} also lack cache read details; these are the same records.',
        { count: quality.legacy_test_cache_read_rows }
      ),
    })
  }
  if (quality.legacy_test_cache_write_rows) {
    reasons.push({
      filter: 'legacy_test_cache_write_rows',
      text: t(
        'Of the unpriced tests above, {{count}} also lack cache write details; these are the same records.',
        { count: quality.legacy_test_cache_write_rows }
      ),
    })
  }
  if (quality.auxiliary_charge_rows) {
    reasons.push({
      filter: 'auxiliary_charge_rows',
      text: t(
        '{{count}} records include tool surcharges; their official price cannot be fully restored yet.',
        { count: quality.auxiliary_charge_rows }
      ),
    })
  }
  if (quality.missing_historical_price_rows) {
    reasons.push({
      filter: 'missing_historical_price_rows',
      text: t(
        '{{count}} records are missing historical price or discount snapshots.',
        { count: quality.missing_historical_price_rows }
      ),
    })
  }
  return reasons
}

// Amounts stay settled in this block; only a usage dimension is unverifiable.
function billingUsageGapEntries(
  quality: BillingDataQuality,
  t: TFunction
): BillingEvidenceEntry[] {
  const reasons: BillingEvidenceEntry[] = []
  const writeUnreported = quality.cache_write_unreported_requests ?? 0
  const writeHistorical =
    (quality.cache_write_unavailable_requests ?? 0) -
    writeUnreported -
    (quality.legacy_test_cache_write_rows ?? 0)
  if (writeHistorical > 0) {
    reasons.push({
      filter: 'cache_write_historical',
      text: t('{{count}} billing records lack cache write details.', {
        count: writeHistorical,
      }),
    })
  }
  const readUnreported = quality.cache_read_unreported_requests ?? 0
  const readHistorical =
    (quality.cache_read_unavailable_requests ?? 0) -
    readUnreported -
    (quality.legacy_test_cache_read_rows ?? 0)
  if (readHistorical > 0) {
    reasons.push({
      filter: 'cache_read_historical',
      text: t('{{count}} billing records lack cache read details.', {
        count: readHistorical,
      }),
    })
  }
  if (writeUnreported > 0) {
    reasons.push({
      filter: 'cache_write_unreported_requests',
      text: t(
        '{{count}} responses did not provide a usable cache write meter; this does not imply a failed request.',
        { count: writeUnreported }
      ),
    })
  }
  if (readUnreported > 0) {
    reasons.push({
      filter: 'cache_read_unreported_requests',
      text: t(
        '{{count}} responses did not provide a usable cache read meter; this does not imply a failed request.',
        { count: readUnreported }
      ),
    })
  }
  const missingTaskLinks = quality.seconds_task_link_missing_rows ?? 0
  if (missingTaskLinks > 0) {
    reasons.push({
      filter: 'seconds_task_link_missing_rows',
      text: t(
        '{{count}} historical video records have no task link or recorded billing seconds.',
        { count: missingTaskLinks }
      ),
    })
  }
  const secondsValueMissing = quality.seconds_value_missing_rows ?? 0
  if (secondsValueMissing - missingTaskLinks > 0) {
    reasons.push({
      filter: 'seconds_value_missing',
      text: t(
        '{{count}} per-second records lack the billable duration used at the time; their amounts cannot be recalculated from usage.',
        { count: secondsValueMissing - missingTaskLinks }
      ),
    })
  }
  const secondsGeneric =
    (quality.seconds_unavailable_rows ?? 0) - secondsValueMissing
  if (secondsGeneric > 0) {
    reasons.push({
      filter: 'seconds_unit_missing',
      text: t('{{count}} per-second records lack the recorded billing unit.', {
        count: secondsGeneric,
      }),
    })
  }
  if (quality.input_tokens_unavailable_requests) {
    reasons.push({
      filter: 'input_tokens_unavailable_requests',
      text: t('{{count}} records have no confirmed total input usage.', {
        count: quality.input_tokens_unavailable_requests,
      }),
    })
  }
  return reasons
}

// Row-level facts that are neither an amount nor a usage-dimension gap.
function billingOtherGapEntries(
  quality: BillingDataQuality,
  t: TFunction
): BillingEvidenceEntry[] {
  const reasons: BillingEvidenceEntry[] = []
  if (quality.unavailable_requests) {
    reasons.push({
      filter: 'unavailable_requests',
      text: t('{{count}} records are missing readable billing metadata.', {
        count: quality.unavailable_requests,
      }),
    })
  }
  if (quality.unknown_billing_mode_requests) {
    reasons.push({
      filter: 'unknown_billing_mode_requests',
      text: t('{{count}} records do not contain a frozen billing mode.', {
        count: quality.unknown_billing_mode_requests,
      }),
    })
  }
  if (quality.provider_model_fallback_rows) {
    reasons.push({
      filter: 'provider_model_fallback_rows',
      text: t(
        '{{count}} records do not contain the Provider model identity; the customer model is shown instead.',
        { count: quality.provider_model_fallback_rows }
      ),
    })
  }
  return reasons
}

// Titles need the coverage partition row; leaner payloads stay heading-less
// instead of showing a zero next to explained records.
function billingEvidenceGroupTitle(
  key: BillingEvidenceGroup['key'],
  count: number | undefined,
  t: TFunction
): string {
  if (count == null) return ''
  if (key === 'amount_gap') {
    return t('Amounts that cannot be calculated yet: {{count}} records', {
      count: count.toLocaleString(),
    })
  }
  if (key === 'usage_gap') {
    return t('Amount available, usage details missing: {{count}} records', {
      count: count.toLocaleString(),
    })
  }
  return t(
    'Amount available, other billing details missing: {{count}} records',
    {
      count: count.toLocaleString(),
    }
  )
}

export function billingEvidenceGroups(
  quality: BillingDataQuality | undefined,
  t: TFunction
): BillingEvidenceGroup[] {
  const coverage = quality?.evidence_coverage
  const groups: BillingEvidenceGroup[] = [
    {
      key: 'amount_gap',
      title: billingEvidenceGroupTitle(
        'amount_gap',
        coverage?.amount_gap_rows,
        t
      ),
      entries: quality ? billingAmountGapEntries(quality, t) : [],
    },
    {
      key: 'usage_gap',
      title: billingEvidenceGroupTitle(
        'usage_gap',
        coverage?.usage_gap_rows,
        t
      ),
      entries: quality ? billingUsageGapEntries(quality, t) : [],
    },
    {
      key: 'other_gap',
      title: billingEvidenceGroupTitle(
        'other_gap',
        coverage?.other_gap_rows,
        t
      ),
      entries: quality ? billingOtherGapEntries(quality, t) : [],
    },
  ]
  return groups.filter((group) => group.entries.length > 0)
}

export function billingDataQualityEntries(
  quality: BillingDataQuality | undefined,
  t: TFunction,
  includePricedTests = true
) {
  if (!quality) return []
  const entries = [
    ...billingAmountGapEntries(quality, t),
    ...billingUsageGapEntries(quality, t),
    ...billingOtherGapEntries(quality, t),
  ]
  if (includePricedTests) entries.push(...billingAccountingEntries(quality, t))
  return entries
}

export function escapeCsvCell(value: string | number) {
  let text = String(value)
  if (typeof value === 'string' && /^[\t\r ]*[=+@-]/.test(text)) {
    text = `'${text}`
  }
  if (!/[",\n]/.test(text)) return text
  return `"${text.replaceAll('"', '""')}"`
}

export function formatStatementUsage(value: number | null, language: string) {
  if (value == null) return '—'
  const normalizedLanguage = language.toLowerCase().replaceAll(/[-_]/g, '')
  let locale = language
  if (normalizedLanguage === 'zhcn') locale = 'zh-CN'
  if (normalizedLanguage === 'zhtw') locale = 'zh-TW'
  if (normalizedLanguage.startsWith('zh')) {
    if (Math.abs(value) >= 100_000_000) {
      return `${new Intl.NumberFormat(locale, {
        maximumFractionDigits: 2,
      }).format(value / 100_000_000)}亿`
    }
    if (Math.abs(value) >= 10_000) {
      return `${new Intl.NumberFormat(locale, {
        maximumFractionDigits: 2,
      }).format(value / 10_000)}万`
    }
  }
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: 2,
    notation: Math.abs(value) >= 10_000 ? 'compact' : 'standard',
  }).format(value)
}

export function channelRowKey(
  urlKey: string,
  channel: ProviderUrlChannelGroupSummary
) {
  return `${urlKey}|${channel.channel_id}`
}

// Shared by summary cells, detail cells and summary CSV. A missing optional
// meter is not proof of zero usage or of an unsupported caching capability.
export function cacheMeterUnavailableLabel(
  quality: BillingDataQuality | undefined,
  kind: 'read' | 'write',
  t: TFunction
): string | undefined {
  const total = quality?.[`cache_${kind}_unavailable_requests`] ?? 0
  if (!total) return undefined
  const unreported = quality?.[`cache_${kind}_unreported_requests`] ?? 0
  if (unreported === total) return t('Upstream meter not provided')
  if (unreported === 0) return t('Historical meter unconfirmed')
  return t('Cache metering incomplete')
}

// Confirmed results only: every line here is a positive verification, so the
// block stays separate from the gap groups. The priced-test total comes first
// and its breakdown partitions it (replayed, recorded, remaining direct).
export function billingAccountingEntries(
  quality: BillingDataQuality | undefined,
  t: TFunction
): BillingEvidenceEntry[] {
  if (!quality) return []
  const notes: BillingEvidenceEntry[] = []
  if (quality.test_priced_rows) {
    notes.push({
      filter: 'test_priced_rows',
      text: t(
        '{{count}} channel tests are priced and included in the amount total.',
        { count: quality.test_priced_rows }
      ),
    })
    if (quality.test_recomputed_rows) {
      notes.push({
        filter: 'test_recomputed_rows',
        text: t(
          'Of the priced tests, {{count}} match their original settlement when recalculated from saved prices and usage.',
          { count: quality.test_recomputed_rows }
        ),
      })
    }
    if (quality.test_recorded_original_rows) {
      notes.push({
        filter: 'test_recorded_original_rows',
        text: t(
          'Of the priced tests, {{count}} use the saved successful expression result with a group multiplier of 1.',
          { count: quality.test_recorded_original_rows }
        ),
      })
    }
    const directRows =
      quality.test_priced_rows -
      (quality.test_recomputed_rows ?? 0) -
      (quality.test_recorded_original_rows ?? 0)
    if (directRows > 0) {
      notes.push({
        filter: '',
        text: t(
          'The remaining {{count}} priced tests read the amount directly from saved pricing records.',
          { count: directRows }
        ),
      })
    }
  }
  if (quality.recovered_billing_seconds_rows) {
    notes.push({
      filter: 'recovered_billing_seconds_rows',
      text: t(
        '{{count}} records have billable seconds verified against their original settlement.',
        { count: quality.recovered_billing_seconds_rows }
      ),
    })
  }
  if (quality.refunded_task_hold_rows) {
    notes.push({
      filter: 'refunded_task_hold_rows',
      text: t(
        '{{count}} failed-task holds were refunded to customers; no successful-task seconds are counted.',
        { count: quality.refunded_task_hold_rows }
      ),
    })
  }
  return notes
}

export function billingSecondsSourceLabel(
  source: string | undefined,
  t: TFunction
): string {
  if (source === 'billing_parameters') {
    return t('Verified original billing parameters')
  }
  if (source === 'refunded_hold') return t('Customer-refunded task hold')
  return ''
}

export function billingDataQualityReasons(
  quality: BillingDataQuality | undefined,
  t: TFunction,
  includePricedTests = true
): string[] {
  return billingDataQualityEntries(quality, t, includePricedTests).map(
    (entry) => entry.text
  )
}

export function billingAccountingNotes(
  quality: BillingDataQuality | undefined,
  t: TFunction
): string[] {
  return billingAccountingEntries(quality, t).map((entry) => entry.text)
}
