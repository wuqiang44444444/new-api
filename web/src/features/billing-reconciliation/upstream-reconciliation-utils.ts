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
  ProviderDiscount,
  ProviderUrlGroupSummary,
  UpstreamDetailEvent,
} from './types'

export function upstreamUrlGroupLabel(
  group: ProviderUrlGroupSummary,
  t: TFunction
): string {
  if (group.custom_name) return group.custom_name
  if (!group.unidentified) {
    return group.base_url || group.url_key
  }
  if (group.deleted) {
    return `${t('Deleted channel')} · #${group.channel_ids.join(', #')}`
  }
  return group.display_name
}

// 卡片标题下的辅助行：已识别组显示安全基础 URL（设置名称后仍保留便于辨认），
// 无法安全识别的组继续显示删除/保留提示，名称不能掩盖删除或地址不明状态。
export function upstreamUrlGroupSubtitle(
  group: ProviderUrlGroupSummary,
  t: TFunction
): string {
  if (group.unidentified) {
    const hints =
      group.deleted && group.custom_name
        ? [`${t('Deleted channel')} · #${group.channel_ids.join(', #')}`]
        : []
    hints.push(t('Unidentified URL — kept per channel'))
    return hints.join(' · ')
  }
  return group.base_url || group.url_key
}

// 折扣展示：0.8 → "0.8（八折）"；null → 待填写；1 → 明确无折扣。
// 统一综合系数不区分协议折扣和充值折扣。
export function upstreamDiscountLabel(
  discount: ProviderDiscount | null,
  t: TFunction
): string {
  if (!discount) return t('Pending')
  const value = Number(discount.value)
  if (!Number.isFinite(value) || value <= 0) return t('Pending')
  const factor = `×${trimDiscountZeros(value)}`
  if (value === 1) return t('No discount (×1)')
  const percent = trimDiscountZeros((1 - value) * 100)
  return t('{{factor}} ({{percent}}% off)', { factor, percent })
}

export function upstreamDiscountSourceLabel(
  discount: ProviderDiscount | null,
  t: TFunction
): string {
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
    case 'task_refunded_hold':
      return t('Customer-refunded task hold')
    case 'task_call':
      return t('Task settlement row')
    case 'channel_test':
      return t('Channel test')
    default:
      return event
  }
}
