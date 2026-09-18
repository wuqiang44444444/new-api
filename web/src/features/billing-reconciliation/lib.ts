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
import { formatQuotaWithCurrency } from '@/lib/currency'

import type { BillingMode } from './types'

// Statements retain small charges and refunds without the balance UI's
// minimum nonzero display floor. Page and CSV must use the same precision.
export function formatCustomerStatementQuota(quota: number | null | undefined) {
  return formatQuotaWithCurrency(quota, {
    digitsLarge: 8,
    digitsSmall: 8,
    compact: false,
    abbreviate: false,
    minimumNonZero: Number.MIN_VALUE,
  })
}

export function currentShanghaiMonth() {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
  }).formatToParts(new Date())
  const year = parts.find((part) => part.type === 'year')?.value ?? '1970'
  const month = parts.find((part) => part.type === 'month')?.value ?? '01'
  return `${year}-${month}`
}

export function resolveShanghaiMonth(month: string) {
  const match = /^(\d{4})-(\d{2})$/.exec(month)
  if (!match) return resolveShanghaiMonth(currentShanghaiMonth())
  const year = Number(match[1])
  const monthNumber = Number(match[2])
  if (monthNumber < 1 || monthNumber > 12) {
    return resolveShanghaiMonth(currentShanghaiMonth())
  }
  const nextYear = monthNumber === 12 ? year + 1 : year
  const nextMonth = monthNumber === 12 ? 1 : monthNumber + 1
  const start = Date.parse(
    `${year}-${String(monthNumber).padStart(2, '0')}-01T00:00:00+08:00`
  )
  const nextStart = Date.parse(
    `${nextYear}-${String(nextMonth).padStart(2, '0')}-01T00:00:00+08:00`
  )
  return {
    start_timestamp: Math.floor(start / 1000),
    end_timestamp: Math.floor(nextStart / 1000) - 1,
  }
}

export function formatInteger(value: number | null | undefined) {
  if (value == null) return '—'
  return new Intl.NumberFormat().format(value)
}

export function billingModeLabel(mode: BillingMode) {
  if (mode === 'token') return 'Token billing'
  if (mode === 'per_call') return 'Per-call billing'
  if (mode === 'per_second') return 'Duration billing'
  return 'Unknown billing mode'
}

// 折扣展示语言（方案 1.2/确认决定）：主要显示“5 折、1.5 折”，同时保留
// ×0.5、×0.15。倍率大于 1 是加价，不称“折扣”；0 表示免费。
export function formatDiscountTier(
  ratio: number,
  t: (key: string, options: Record<string, string>) => string
): string {
  if (!Number.isFinite(ratio) || ratio <= 0) return ''
  if (ratio === 1) return '×1'
  if (ratio > 1) {
    return `×${trimTrailingZeros(ratio)}`
  }
  const tier = trimTrailingZeros(ratio * 10)
  return t('{{percent}}% off', {
    percent: trimTrailingZeros((1 - ratio) * 100),
    tier,
  })
}

export function formatDiscountFactor(ratio: number): string {
  if (!Number.isFinite(ratio) || ratio <= 0) return ''
  return `×${trimTrailingZeros(ratio)}`
}

function trimTrailingZeros(value: number): string {
  return String(Number(value.toFixed(6)))
}

// 最终折扣 G×C（1.1）：两个因子都已知时才给出乘积。
export function combinedDiscountFactor(
  groupRatio: number | null | undefined,
  contractRatio: number | null | undefined
): number | null {
  if (
    groupRatio == null ||
    !Number.isFinite(groupRatio) ||
    groupRatio <= 0 ||
    contractRatio == null ||
    !Number.isFinite(contractRatio) ||
    contractRatio <= 0
  ) {
    return null
  }
  return groupRatio * contractRatio
}
