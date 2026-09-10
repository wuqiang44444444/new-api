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
/**
 * 读取后端 billing_display 投影的唯一入口。金额展示一律从这里取得已解释
 * 的单价与条件；投影缺失、版本不支持或整体 opaque 时返回空结果，由调用
 * 方显示「暂无法展开」，绝不回退本地正则猜价。
 */
import type {
  BillingDisplayProjection,
  BillingDisplayRule,
  BillingDisplayTier,
} from '../types'
import {
  BILLING_PRICING_VARS,
  type ParsedTier,
  type RequestCondition,
  type RequestRuleGroup,
  type RequestRuleTrace,
  type TierCondition,
} from './billing-expr'

/** 当前前端支持的投影版本；不一致时按投影缺失处理。 */
export const SUPPORTED_BILLING_DISPLAY_VERSION = 2

const TIER_CONDITION_VARS = new Set(['p', 'c', 'len'])

const TIME_COMPARE_OP_TO_MODE: Record<string, string> = {
  '==': 'eq',
  '>=': 'gte',
  '<': 'lt',
}

const COMPARE_OP_TO_MODE: Record<string, string> = {
  '==': 'eq',
  '!=': 'neq',
  '>': 'gt',
  '>=': 'gte',
  '<': 'lt',
  '<=': 'lte',
}

export function isUsableBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): projection is BillingDisplayProjection {
  return (
    !!projection &&
    projection.display_version === SUPPORTED_BILLING_DISPLAY_VERSION &&
    projection.unit === 'usd_per_million_tokens' &&
    projection.status === 'exact' &&
    (projection.tiers?.length ?? 0) > 0
  )
}

/**
 * 把投影档位映射为展示组件已消费的 ParsedTier 形状。单价来自投影的
 * unit_prices（已吸收固定乘除）；映射不了的变量被忽略，绝不猜价。
 */
export function tiersFromBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): ParsedTier[] {
  if (!isUsableBillingDisplay(projection)) return []
  return (projection.tiers ?? []).map((tier) => ({
    ...tierFromBillingDisplay(tier),
    constantCharge: (tier.constant ?? 0) + (projection.constant_charge ?? 0),
  }))
}

/** Select an already projected price branch from recorded facts, never from the current clock. */
export function recordedBillingScenario(
  projection: BillingDisplayProjection | null | undefined,
  traces: RequestRuleTrace[] | null | undefined
): NonNullable<BillingDisplayProjection['scenarios']>[number] | undefined {
  if (!isUsableBillingDisplay(projection) || projection.rules?.length !== 1) {
    return undefined
  }
  const matches =
    traces?.filter((trace) => trace.cond === projection.rules?.[0].text) ?? []
  if (matches.length !== 1) return undefined
  return projection.scenarios?.find(
    (scenario) => scenario.matched === matches[0].matched
  )
}

function tierFromBillingDisplay(tier: BillingDisplayTier): ParsedTier {
  const parsed: ParsedTier = {
    label: tier.label,
    conditions: [],
    conditionText: tier.condition_text,
    conditionTree: tier.condition,
    constantCharge: tier.constant,
  }
  for (const variable of BILLING_PRICING_VARS) {
    if (!variable.field) continue
    const value = tier.unit_prices[variable.key]
    if (value !== undefined && Number.isFinite(value)) {
      parsed[variable.field] = value
    }
  }
  const conditions: TierCondition[] = []
  for (const condition of tier.conditions ?? []) {
    if (!TIER_CONDITION_VARS.has(condition.var)) continue
    conditions.push({
      var: condition.var as TierCondition['var'],
      op: condition.op as TierCondition['op'],
      value: condition.value,
    })
  }
  parsed.conditions = conditions
  return parsed
}

/**
 * 把投影条件倍率树映射为 RequestRuleGroup。仅当整棵树能展平为
 * AND 连接的已结构化叶子时给出结构化条件；否则保留规范化原文，
 * 绝不静默丢弃条件的另一半分支。
 */
export function ruleGroupsFromBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): RequestRuleGroup[] {
  if (!isUsableBillingDisplay(projection)) return []
  return (projection.rules ?? []).map((rule) =>
    ruleGroupFromBillingDisplay(rule)
  )
}

function ruleGroupFromBillingDisplay(
  rule: BillingDisplayRule
): RequestRuleGroup {
  const multiplierText =
    rule.fallback != null && rule.fallback !== 1
      ? `${rule.multiplier} : ${rule.fallback}`
      : `${rule.multiplier}`
  const flattened = flattenAndLeaves(rule)
  if (flattened) {
    return {
      conditions: flattened,
      multiplier: multiplierText,
      conditionText: rule.text,
    }
  }
  return {
    conditions: [],
    multiplier: multiplierText,
    conditionText: rule.text,
  }
}

function flattenAndLeaves(rule: BillingDisplayRule): RequestCondition[] | null {
  if (rule.op === 'and' && rule.children?.length) {
    const conditions: RequestCondition[] = []
    for (const child of rule.children) {
      const nested = flattenAndLeaves(child)
      if (!nested) return null
      conditions.push(...nested)
    }
    return conditions
  }
  if (rule.op) return null
  const leaf = structuredLeafCondition(rule)
  return leaf ? [leaf] : null
}

function structuredLeafCondition(
  rule: BillingDisplayRule
): RequestCondition | null {
  if (
    rule.text_only ||
    !rule.source ||
    rule.source === 'text' ||
    rule.source === 'token'
  ) {
    return null
  }
  if (rule.source === 'time') {
    const mode = TIME_COMPARE_OP_TO_MODE[rule.compare_op ?? '']
    if (!mode) return null
    return {
      source: 'time',
      timeFunc: (rule.time_func ?? 'hour') as 'hour',
      timezone: rule.timezone ?? 'UTC',
      mode,
      value: rule.value ?? '',
      rangeStart: '',
      rangeEnd: '',
    }
  }
  const mode = COMPARE_OP_TO_MODE[rule.compare_op ?? '']
  if (!mode) return null
  if (mode === 'neq' && rule.value === '') {
    return {
      source: rule.source,
      path: rule.path ?? '',
      mode: 'exists',
      value: '',
    }
  }
  return {
    source: rule.source,
    path: rule.path ?? '',
    mode,
    value: rule.value ?? '',
  }
}
