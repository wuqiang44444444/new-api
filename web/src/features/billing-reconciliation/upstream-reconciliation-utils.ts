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

import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'

import { billingModeLabel } from './lib'
import type {
  ProviderChannelDiscountStatus,
  ProviderUrlChannelSummary,
  ProviderUrlGroupSummary,
  ProviderUrlModelSummary,
  UpstreamDetailEvent,
} from './types'
import {
  billingDataQualityLabel,
  escapeCsvCell,
  upstreamModelLabel,
} from './upstream-statement-utils'

export function upstreamUrlGroupLabel(
  group: ProviderUrlGroupSummary,
  t: TFunction
): string {
  if (!group.unidentified) return group.base_url ?? group.url_key
  if (group.deleted) {
    return `${t('Deleted channel')} · #${group.channel_ids.join(', #')}`
  }
  return group.display_name
}

// 折扣槽位展示：0.8 → "0.8（八折）"；null → 待填写；1 → 明确无折扣。
// 统一综合系数不区分协议折扣和充值折扣。
export function upstreamDiscountLabel(
  status: ProviderChannelDiscountStatus,
  t: TFunction
): string {
  if (!status.discount) return t('Pending')
  const value = Number(status.discount.value)
  if (!Number.isFinite(value) || value <= 0) return t('Pending')
  const factor = `×${trimDiscountZeros(value)}`
  if (value === 1) return t('No discount (×1)')
  const percent = trimDiscountZeros((1 - value) * 100)
  return t('{{factor}} ({{percent}}% off)', { factor, percent })
}

export function upstreamDiscountSourceLabel(
  status: ProviderChannelDiscountStatus,
  t: TFunction
): string {
  const discount = status.discount
  if (!discount) return t('Pending manual fill')
  if (discount.source === 'previous_period' && discount.source_period) {
    return t('Inherited from {{month}}', {
      month: periodMonthLabel(discount.source_period),
    })
  }
  if (discount.source === 'default') return t('Default coefficient (×1)')
  if (discount.source === 'migrated') {
    return t('Migrated from model-level discounts')
  }
  return t('Configured this month')
}

export function periodMonthLabel(periodStart: number): string {
  // 折扣账期是上海时区自然月；用当地日历日期取年月，避免浏览器时区偏移。
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
  }).formatToParts(new Date(periodStart * 1000))
  const year = parts.find((part) => part.type === 'year')?.value ?? '1970'
  const month = parts.find((part) => part.type === 'month')?.value ?? '01'
  return `${year}-${month}`
}

function trimDiscountZeros(value: number): string {
  return String(Number(value.toFixed(8)))
}

// 导出固定标注上海时区的账期实际起止时刻，便于与上游账单的切账规则核对。
export function formatShanghaiTimestamp(timestamp: number): string {
  const parts = new Intl.DateTimeFormat('sv-SE', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(timestamp * 1000))
  return `${parts} +08:00`
}

export function upstreamDetailEventLabel(
  event: UpstreamDetailEvent,
  t: TFunction
): string {
  switch (event) {
    case 'refund':
      return t('Customer refund')
    case 'call':
      return t('Settled call')
    case 'task_create':
      return t('Task hold')
    case 'task_adjustment':
      return t('Task settlement adjustment')
    case 'task_call':
      return t('Task settlement row')
    case 'channel_test':
      return t('Channel test')
    default:
      return event
  }
}

type UpstreamReconciliationCsvOptions = {
  groups: ProviderUrlGroupSummary[]
  generatedAt: number
  month: string
  period: { start_timestamp: number; end_timestamp: number }
  t: TFunction
}

// CSV rows are the per-channel leaf rows only, so parent URL and model totals
// are never double-counted next to their children. Amount columns carry the
// same local official-price and reference figures as the page, and each row
// records the discount value and version used, so later edits never rewrite
// what an export said.
export function buildUpstreamReconciliationCsv(
  options: UpstreamReconciliationCsvOptions
) {
  const headers = [
    options.t('Billing month'),
    options.t('Period start (Asia/Shanghai)'),
    options.t('Period end (Asia/Shanghai)'),
    options.t('Upstream base URL'),
    options.t('URL grouping'),
    options.t('Upstream channel'),
    'channel_id',
    options.t('Model'),
    options.t('Billing mode'),
    options.t('Input tokens'),
    options.t('Cache read tokens'),
    options.t('Cache write tokens'),
    options.t('Output tokens'),
    options.t('Requests'),
    options.t('Billable calls'),
    options.t('Original amount (local official price)'),
    options.t('Reference amount (after channel discount)'),
    options.t('Channel discount'),
    'discount_version',
    options.t('Data status'),
    'generated_at',
    options.t('Currency'),
    'quota_per_unit',
    'currency_rate',
  ]
  const lines = [headers.map(escapeCsvCell).join(',')]
  for (const group of options.groups) {
    for (const model of group.models) {
      for (const channel of model.channels) {
        lines.push(
          upstreamReconciliationCsvRow(options, group, model, channel)
            .map((value, index) =>
              (index === 15 || index === 16) && typeof value === 'number'
                ? value.toFixed(8)
                : escapeCsvCell(value)
            )
            .join(',')
        )
      }
    }
  }
  return lines.join('\n')
}

function upstreamReconciliationCsvRow(
  options: UpstreamReconciliationCsvOptions,
  group: ProviderUrlGroupSummary,
  model: ProviderUrlModelSummary,
  channel: ProviderUrlChannelSummary
): Array<string | number> {
  const grouping = group.unidentified
    ? options.t('Unidentified URL — kept per channel')
    : options.t('Current channel base URL')
  const { config, meta } = getCurrencyDisplay()
  const rate = meta.kind === 'tokens' ? 1 : meta.exchangeRate
  const amount = (quota: number) =>
    meta.kind === 'tokens' ? quota : (quota / config.quotaPerUnit) * rate
  return [
    options.month,
    formatShanghaiTimestamp(options.period.start_timestamp),
    formatShanghaiTimestamp(options.period.end_timestamp),
    group.unidentified ? '' : (group.base_url ?? group.url_key),
    grouping,
    channel.channel_name,
    channel.channel_id,
    upstreamModelLabel(channel),
    options.t(billingModeLabel(model.billing_mode)),
    channel.usage.input_tokens,
    channel.usage.cache_read_tokens,
    channel.data_quality?.cache_write_unavailable_requests
      ? options.t('Not recorded')
      : channel.usage.cache_write_tokens,
    channel.usage.output_tokens,
    channel.usage.requests,
    channel.usage.billable_calls,
    channel.original_amount == null
      ? options.t('Unknown')
      : amount(channel.original_amount),
    channel.reference_amount == null
      ? options.t('Incomplete')
      : amount(channel.reference_amount),
    channel.discount ? String(channel.discount.value) : options.t('Pending'),
    channel.discount?.version ?? '',
    billingDataQualityLabel(channel.data_quality, options.t),
    new Date(options.generatedAt * 1000).toISOString(),
    getCurrencyLabel(),
    config.quotaPerUnit,
    rate,
  ]
}

export function downloadUpstreamReconciliationCsv(
  options: UpstreamReconciliationCsvOptions
) {
  const csv = buildUpstreamReconciliationCsv(options)
  const blob = new Blob([`\uFEFF${csv}`], {
    type: 'text/csv;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `upstream-reconciliation-${options.month}.csv`
  document.body.append(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
