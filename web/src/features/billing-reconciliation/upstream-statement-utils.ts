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

import type { BillingDataQuality, ProviderUrlChannelSummary } from './types'

export function upstreamModelLabel(
  model: Pick<
    ProviderUrlChannelSummary,
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
  if (quality.cache_write_unavailable_requests) {
    reasons.push(
      t('{{count}} channel test records have no recorded cache write usage.', {
        count: quality.cache_write_unavailable_requests,
      })
    )
  }
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
