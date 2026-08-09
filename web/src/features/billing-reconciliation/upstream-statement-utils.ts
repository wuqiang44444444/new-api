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

import { billingModeLabel } from './lib'
import type {
  BillingDataQuality,
  ProviderChannelSummary,
  ProviderModelSummary,
} from './types'

export type UpstreamStatementFilters = {
  channel: string
  model: string
}

export function filterProviderChannels(
  channels: ProviderChannelSummary[],
  filters: UpstreamStatementFilters
) {
  return channels.flatMap((channel) => {
    if (
      filters.channel !== 'all' &&
      String(channel.channel_id) !== filters.channel
    ) {
      return []
    }
    const models =
      filters.model === 'all'
        ? channel.models
        : channel.models.filter(
            (model) => model.provider_model === filters.model
          )
    if (models.length === 0) return []
    return [{ ...channel, models }]
  })
}

export function billingDataQualityLabel(
  quality: BillingDataQuality | undefined,
  t: TFunction
) {
  if (quality?.status === 'partial') return t('Partial data')
  if (quality?.status === 'unavailable') return t('Unavailable')
  return t('Complete')
}

export function billingDataQualityReasons(
  quality: BillingDataQuality | undefined,
  t: TFunction
) {
  if (!quality || quality.status === 'complete') return []
  const reasons: string[] = []
  if (quality.unavailable_requests) {
    reasons.push(
      t('{{count}} records are missing readable billing metadata.', {
        count: quality.unavailable_requests,
      })
    )
  }
  if (quality.unknown_billing_mode_requests) {
    reasons.push(
      t('{{count}} records do not contain a frozen billing mode.', {
        count: quality.unknown_billing_mode_requests,
      })
    )
  }
  if (quality.provider_model_fallback_rows) {
    reasons.push(
      t(
        '{{count}} records do not contain the Provider model identity; the customer model is shown instead.',
        { count: quality.provider_model_fallback_rows }
      )
    )
  }
  if (quality.missing_historical_price_rows) {
    reasons.push(
      t(
        '{{count}} records are missing historical price or discount snapshots.',
        { count: quality.missing_historical_price_rows }
      )
    )
  }
  return reasons
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

type UpstreamStatementCsvOptions = {
  channels: ProviderChannelSummary[]
  generatedAt: number
  month: string
  t: TFunction
}

export function buildUpstreamStatementCsv(
  options: UpstreamStatementCsvOptions
) {
  const headers = [
    options.t('Billing month'),
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
    options.t('Data quality'),
    'generated_at',
  ]
  const lines = [headers.map(escapeCsvCell).join(',')]
  for (const channel of options.channels) {
    for (const model of channel.models) {
      lines.push(
        upstreamStatementCsvRow(options, channel, model)
          .map(escapeCsvCell)
          .join(',')
      )
    }
  }
  return lines.join('\n')
}

function escapeCsvCell(value: string | number) {
  let text = String(value)
  if (typeof value === 'string' && /^[\t\r ]*[=+@-]/.test(text)) {
    text = `'${text}`
  }
  if (!/[",\n]/.test(text)) return text
  return `"${text.replaceAll('"', '""')}"`
}

function upstreamStatementCsvRow(
  options: UpstreamStatementCsvOptions,
  channel: ProviderChannelSummary,
  model: ProviderModelSummary
): Array<string | number> {
  return [
    options.month,
    channel.channel_name,
    channel.channel_id,
    model.provider_model,
    options.t(billingModeLabel(model.billing_mode)),
    model.usage.input_tokens,
    model.usage.cache_read_tokens,
    model.usage.cache_write_tokens,
    model.usage.output_tokens,
    model.usage.requests,
    model.usage.billable_calls,
    billingDataQualityLabel(model.data_quality, options.t),
    new Date(options.generatedAt * 1000).toISOString(),
  ]
}

export function downloadUpstreamStatementCsv(
  options: UpstreamStatementCsvOptions
) {
  const csv = buildUpstreamStatementCsv(options)
  const blob = new Blob([`\uFEFF${csv}`], {
    type: 'text/csv;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `upstream-usage-statement-${options.month}.csv`
  document.body.append(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
