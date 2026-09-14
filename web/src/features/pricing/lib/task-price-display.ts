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
import {
  resolveLocalizedText,
  type LocalizedTextValue,
} from '@/lib/localized-text'

import type {
  BillingDisplayRule,
  BillingUsageFieldSchema,
  BillingUsageSchema,
  PricingModel,
} from '../types'
import type { TaskTierCondition } from './billing-expr'
import {
  ruleGroupsFromBillingDisplay,
  taskTiersFromBillingDisplay,
} from './billing-display'

export function taskPriceLabel(
  description: LocalizedTextValue | undefined,
  field: string,
  language: string
): string {
  const localized =
    typeof description === 'object' && description
      ? { ...description, en: description.en?.trim() || field }
      : description
  return resolveLocalizedText(localized, language) || field
}

export function taskEnumLabel(
  definition: BillingUsageFieldSchema | undefined,
  value: string,
  language: string
): string {
  return taskPriceLabel(definition?.enumLabels?.[value], value, language)
}

export function taskPricingConditions(
  conditions: TaskTierCondition[],
  schema: BillingUsageSchema | undefined,
  language: string,
  t: (key: string) => string
): string {
  return conditions
    .map(({ field, value }) => {
      const definition = schema?.[field]
      const label = taskPriceLabel(definition?.description, field, language)
      if (definition?.type === 'boolean') {
        return `${label}: ${value === 'true' ? t('Yes') : t('No')}`
      }
      const optionLabel = taskEnumLabel(definition, value, language)
      return optionLabel !== value ? optionLabel : `${label}: ${optionLabel}`
    })
    .join(' · ')
}

const COMPARE_OP_SYMBOLS: Record<string, string> = {
  '==': '=',
  '!=': '≠',
  '>': '>',
  '>=': '≥',
  '<': '<',
  '<=': '≤',
}

/**
 * 详情档位的条件说明：优先使用可展平的 AND 等值条件；否定、或与数值比较
 * 从后端条件树结构化渲染并本地化，不再把不同分支简化成同一句“其他情况”。
 */
export function taskPricingConditionSummary(
  tier: {
    conditions: TaskTierCondition[]
    conditionTree?: BillingDisplayRule
  },
  schema: BillingUsageSchema | undefined,
  language: string,
  t: (key: string) => string,
  tierCount: number
): string {
  const flat = taskPricingConditions(tier.conditions, schema, language, t)
  if (flat) return flat
  const tree = renderTaskConditionTree(tier.conditionTree, schema, language, t)
  if (tree) return tree
  return t(tierCount > 1 ? 'Other cases' : 'All requests')
}

function renderTaskConditionTree(
  rule: BillingDisplayRule | undefined,
  schema: BillingUsageSchema | undefined,
  language: string,
  t: (key: string) => string
): string {
  if (!rule || rule.text_only) return ''
  if (rule.op === 'and' || rule.op === 'or') {
    const children = rule.children ?? []
    const parts = children.map((child) =>
      renderTaskConditionTree(child, schema, language, t)
    )
    if (parts.length === 0 || parts.some((part) => !part)) return ''
    return parts.join(rule.op === 'and' ? ' · ' : ` ${t('or')} `)
  }
  if (rule.op === 'not') {
    const child = (rule.children ?? [])[0]
    if (!child) return ''
    return renderInvertedLeaf(child, schema, language, t)
  }
  return renderTaskConditionLeaf(rule, schema, language, t)
}

function renderTaskConditionLeaf(
  rule: BillingDisplayRule,
  schema: BillingUsageSchema | undefined,
  language: string,
  t: (key: string) => string
): string {
  if (rule.text_only || rule.source !== 'usage') return ''
  const label = taskPriceLabel(
    schema?.[rule.path ?? '']?.description,
    rule.path ?? '',
    language
  )
  const op = rule.compare_op ?? '=='
  if (op === '==') {
    return renderUsageValueText(rule.path ?? '', rule.value ?? '', schema, language, t)
  }
  const symbol = COMPARE_OP_SYMBOLS[op]
  if (!symbol) return ''
  return `${label} ${symbol} ${rule.value ?? ''}`
}

function renderInvertedLeaf(
  rule: BillingDisplayRule,
  schema: BillingUsageSchema | undefined,
  language: string,
  t: (key: string) => string
): string {
  if (rule.text_only || rule.source !== 'usage' || (rule.compare_op ?? '==') !== '==') {
    return ''
  }
  const definition = schema?.[rule.path ?? '']
  const label = taskPriceLabel(definition?.description, rule.path ?? '', language)
  if (definition?.type === 'boolean') {
    return `${label}: ${rule.value === 'true' ? t('No') : t('Yes')}`
  }
  return `${label} ≠ ${taskEnumLabel(definition, rule.value ?? '', language)}`
}

function renderUsageValueText(
  field: string,
  value: string,
  schema: BillingUsageSchema | undefined,
  language: string,
  t: (key: string) => string
): string {
  const definition = schema?.[field]
  const label = taskPriceLabel(definition?.description, field, language)
  if (definition?.type === 'boolean') {
    return `${label}: ${value === 'true' ? t('Yes') : t('No')}`
  }
  const optionLabel = taskEnumLabel(definition, value, language)
  return optionLabel !== value
    ? `${label}: ${optionLabel}`
    : `${label}: ${optionLabel}`
}

export function hasSimpleTaskPricing(model: PricingModel): boolean {
  if (
    !model.billing_usage_schema ||
    model.billing_mode !== 'tiered_expr' ||
    !model.billing_expr
  ) {
    return false
  }
  // 与金额展示同源：只信任后端投影判定档位与条件倍率。
  if (ruleGroupsFromBillingDisplay(model.billing_display).length > 0) {
    return false
  }
  return taskTiersFromBillingDisplay(model.billing_display).length === 1
}
