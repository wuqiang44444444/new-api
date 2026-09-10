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
import type { TFunction } from 'i18next'

import type { BillingDisplayRule } from '../types'
import { BILLING_VARS } from './billing-expr'

/** Format the backend condition tree; never evaluate it against the browser clock. */
export function billingConditionText(
  rule: BillingDisplayRule,
  t: TFunction,
  language: string
): string {
  if (rule.op && rule.children?.length) {
    const children = rule.children.map((child) =>
      billingConditionText(child, t, language)
    )
    if (rule.op === 'not') return `${t('Not')} (${children[0]})`
    return `(${children.join(rule.op === 'and' ? ` ${t('And')} ` : ` ${t('Or')} `)})`
  }
  const operators: Record<string, string> = {
    '>=': '≥',
    '<=': '≤',
    '==': '=',
    '!=': '≠',
  }
  if (!rule.text_only && rule.source === 'token') {
    const number = Number(rule.value)
    let value = rule.value
    if (number >= 1_000_000) value = `${number / 1_000_000}M`
    else if (number >= 1000) value = `${number / 1000}K`
    const label =
      { p: t('Input'), c: t('Output'), len: t('Full input length') }[
        rule.path ?? ''
      ] ??
      t(
        BILLING_VARS.find((item) => item.key === rule.path)?.shortLabel ??
          rule.path ??
          ''
      )
    return `${label} ${operators[rule.compare_op ?? ''] ?? rule.compare_op} ${value}`
  }
  if (rule.text_only || rule.source !== 'time') return rule.text
  const labels: Record<string, string> = {
    weekday: t('Weekday'),
    hour: t('Hour'),
    minute: t('Minute'),
    month: t('Month'),
    day: t('Day'),
  }
  let value = rule.value ?? ''
  const numeric = Number(value)
  if (
    rule.time_func === 'weekday' &&
    Number.isInteger(numeric) &&
    numeric >= 0 &&
    numeric <= 6
  ) {
    value = new Intl.DateTimeFormat(language, {
      weekday: 'long',
      timeZone: 'UTC',
    }).format(new Date(Date.UTC(2026, 8, 6 + numeric)))
  } else if (rule.time_func === 'hour' && Number.isInteger(numeric)) {
    value = `${String(numeric).padStart(2, '0')}:00`
  }
  return `${labels[rule.time_func ?? ''] ?? rule.time_func} ${operators[rule.compare_op ?? ''] ?? rule.compare_op} ${value} (${rule.timezone || 'UTC'})`
}
