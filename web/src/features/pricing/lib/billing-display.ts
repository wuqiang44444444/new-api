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
  type ParsedTaskTier,
  type ParsedTier,
  type RequestCondition,
  type RequestRuleGroup,
  type RequestRuleTrace,
  type TaskTierCondition,
  type TierCondition,
} from './billing-expr'

/** 当前前端支持的投影版本；不一致时按投影缺失处理。 */
export const SUPPORTED_BILLING_DISPLAY_VERSION = 2

const TOKEN_DISPLAY_UNIT = 'usd_per_million_tokens'
/** 任务 USD 投影单位：单价按各声明字段自身单位（token 字段为每百万）。 */
export const TASK_DISPLAY_UNIT = 'usd_per_usage_unit'

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

function isUsableProjectionWithUnit(
  projection: BillingDisplayProjection | null | undefined,
  unit: string
): projection is BillingDisplayProjection {
  return (
    !!projection &&
    projection.display_version === SUPPORTED_BILLING_DISPLAY_VERSION &&
    projection.unit === unit &&
    projection.status === 'exact' &&
    (projection.tiers?.length ?? 0) > 0
  )
}

export function isUsableBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): projection is BillingDisplayProjection {
  return isUsableProjectionWithUnit(projection, TOKEN_DISPLAY_UNIT)
}

export function isUsableTaskBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): projection is BillingDisplayProjection {
  return isUsableProjectionWithUnit(projection, TASK_DISPLAY_UNIT)
}

/**
 * 把任务投影档位映射为展示组件已消费的 ParsedTaskTier 形状。条件从结构化
 * usage 叶子提取，无法展平为 AND 等值叶子的条件保留原文；金额一律来自
 * 投影，绝不本地解析表达式。
 */
export function taskTiersFromBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): ParsedTaskTier[] {
  if (!isUsableTaskBillingDisplay(projection)) return []
  return (projection.tiers ?? []).map((tier) =>
    taskTierFromBillingDisplay(tier, projection)
  )
}

function taskTierFromBillingDisplay(
  tier: BillingDisplayTier,
  projection: BillingDisplayProjection
): ParsedTaskTier {
  return {
    label: tier.label,
    conditions: usageConditionsFromBillingRule(tier.condition),
    conditionText: tier.condition_text,
    conditionTree: tier.condition,
    constant: (tier.constant ?? 0) + (projection.constant_charge ?? 0),
    hasConstant: tier.has_constant || projection.constant_charge != null,
    unitPrices: { ...tier.unit_prices },
  }
}

/**
 * 任务投影的可证明条件倍率场景（每档单价已乘以该分支倍率）。公开价格没有
 * 结算 traces，卡片范围与详情说明都以场景集合为准。
 */
export function taskScenarioTiersFromBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): { matched: boolean; tiers: ParsedTaskTier[] }[] {
  if (!isUsableTaskBillingDisplay(projection)) return []
  return (projection.scenarios ?? []).map((scenario) => ({
    matched: scenario.matched,
    tiers: scenario.tiers.map((tier) =>
      taskTierFromBillingDisplay(tier, projection)
    ),
  }))
}

/** token 投影的可证明条件倍率场景，形状与任务场景一致。 */
export function tokenScenarioTiersFromBillingDisplay(
  projection: BillingDisplayProjection | null | undefined
): { matched: boolean; tiers: ParsedTier[] }[] {
  if (!isUsableBillingDisplay(projection)) return []
  return (projection.scenarios ?? []).map((scenario) => ({
    matched: scenario.matched,
    tiers: scenario.tiers.map((tier) => ({
      ...tierFromBillingDisplay(tier),
      constantCharge: (tier.constant ?? 0) + (projection.constant_charge ?? 0),
      hasConstant: tier.has_constant || projection.constant_charge != null,
    })),
  }))
}

function usageConditionsFromBillingRule(
  rule: BillingDisplayRule | undefined
): TaskTierCondition[] {
  const flattened = flattenUsageEqualityLeaves(rule)
  return flattened ?? []
}

function flattenUsageEqualityLeaves(
  rule: BillingDisplayRule | undefined
): TaskTierCondition[] | null {
  if (!rule) return null
  if (rule.text_only || rule.op === 'or' || rule.op === 'not') return null
  if (!rule.op) {
    if (rule.source === 'usage' && rule.compare_op === '==') {
      return [{ field: rule.path ?? '', value: rule.value ?? '' }]
    }
    return null
  }
  if (rule.op === 'and') {
    const conditions: TaskTierCondition[] = []
    for (const child of rule.children ?? []) {
      const nested = flattenUsageEqualityLeaves(child)
      if (!nested) return null
      conditions.push(...nested)
    }
    return conditions
  }
  return null
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
    hasConstant: tier.has_constant || projection.constant_charge != null,
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
    hasConstant: tier.has_constant,
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
  // 条件倍率树与单价单位无关：token 与任务 USD 投影都可读取。
  if (
    !isUsableBillingDisplay(projection) &&
    !isUsableTaskBillingDisplay(projection)
  ) {
    return []
  }
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
    rule.source === 'token' ||
    // 任务用量字段条件不是请求条件，只保留规范化原文。
    rule.source === 'usage'
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
