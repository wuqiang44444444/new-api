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
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import { TOKEN_UNIT_DIVISORS } from '../constants'
import type {
  BillingUsageSchema,
  BillingUsageUnit,
  PricingModel,
  TokenUnit,
} from '../types'
import {
  ruleGroupsFromBillingDisplay,
  taskScenarioTiersFromBillingDisplay,
  taskTiersFromBillingDisplay,
  tokenScenarioTiersFromBillingDisplay,
  tiersFromBillingDisplay,
} from './billing-display'
import {
  BILLING_PRICING_VARS,
  type BillingVar,
  type ParsedTaskTier,
  type ParsedTier,
} from './billing-expr'
import { getDisplayGroupRatio } from './model-helpers'
import { getTaskNumberFields } from './task-expr'

export type DynamicPriceOptions = {
  tokenUnit: TokenUnit
  showCurrencySymbol?: boolean
  showRechargePrice?: boolean
  priceRate?: number
  usdExchangeRate?: number
  groupRatioMultiplier?: number
  usageSchema?: BillingUsageSchema
}

export type DynamicPriceLabelKind = 'i18n' | 'schema'

export type DynamicPriceEntry = {
  key: string
  field: string
  label: string
  shortLabel: string
  /** `schema` labels are raw usage-field names and must not go through `t()`. */
  labelKind: DynamicPriceLabelKind
  value: number
  formatted: string
  formattedRange?: string
  unit: 'token' | BillingUsageUnit | 'request'
  variable?: BillingVar
  description?: string | Record<string, string>
}

export type CardExamplePrice = {
  label: string
  formatted: string
}

export type DynamicPricingTier = ParsedTier | ParsedTaskTier

export type DynamicPricingSummary = {
  tiers: DynamicPricingTier[]
  tier: DynamicPricingTier | null
  tierCount: number
  hasRequestRules: boolean
  isSpecialExpression: boolean
  rawExpression: string
  entries: DynamicPriceEntry[]
  primaryEntries: DynamicPriceEntry[]
  secondaryEntries: DynamicPriceEntry[]
  isTaskUsage: boolean
}

export function getTaskUsageQuantityUnitLabelKey(
  unit: BillingUsageUnit | undefined
): string {
  if (unit === 'second') return 's'
  if (unit === 'token') return 'token (unit)'
  if (unit === 'credit') return 'credit'
  return 'unit'
}

export function getTaskUsagePriceUnitLabelKey(
  unit: BillingUsageUnit | undefined
): string {
  if (unit === 'second') return 'second'
  if (unit === 'token') return '1M token'
  if (unit === 'credit') return 'credit'
  return 'unit'
}

export function getDynamicPriceUnitLabelKey(
  entry: DynamicPriceEntry
): string | null {
  if (entry.unit === 'second') return 's'
  if (entry.unit === 'count') return 'unit'
  if (entry.unit === 'credit') return 'credit'
  // Chat token entries also use unit 'token' but keep the 1M-token label.
  if (entry.unit === 'token' && !entry.variable) return '1M token'
  if (entry.unit === 'request') return 'request'
  return null
}

const PRIMARY_DYNAMIC_FIELDS = new Set([
  'inputPrice',
  'outputPrice',
  'imageOutputPrice',
])

function isTaskPricingTier(tier: DynamicPricingTier): tier is ParsedTaskTier {
  return (
    Object.hasOwn(tier, 'unitPrices') &&
    typeof (tier as ParsedTaskTier).unitPrices === 'object'
  )
}

export function isDynamicPricingModel(model: PricingModel): boolean {
  return model.billing_mode === 'tiered_expr' && Boolean(model.billing_expr)
}

export function hasTaskUsageSchema(model: PricingModel): boolean {
  return Object.keys(model.billing_usage_schema ?? {}).length > 0
}

export function isTaskUsagePricingModel(model: PricingModel): boolean {
  return model.billing_mode === 'tiered_expr' && hasTaskUsageSchema(model)
}

export function isUnconfiguredTaskUsageModel(model: PricingModel): boolean {
  return (
    model.quota_type !== 1 &&
    hasTaskUsageSchema(model) &&
    !isDynamicPricingModel(model)
  )
}

export function getTaskPricingUnit(
  model: PricingModel
): BillingUsageUnit | null {
  const primaryField = getTaskNumberFields(model.billing_usage_schema)[0]
  return primaryField?.[1].unit ?? null
}

export function getDynamicDisplayGroupRatio(
  model: PricingModel,
  selectedGroup?: string
): number {
  return getDisplayGroupRatio(model, selectedGroup)
}

function applyRechargeRate(
  price: number,
  showWithRecharge: boolean,
  priceRate: number,
  usdExchangeRate: number
): number {
  if (!showWithRecharge) return price
  return (price * priceRate) / usdExchangeRate
}

export function formatDynamicUnitPrice(
  valuePerMillionTokens: number,
  options: DynamicPriceOptions
): string {
  const groupRatio = options.groupRatioMultiplier ?? 1
  const priceRate = options.priceRate ?? 1
  const usdExchangeRate = options.usdExchangeRate ?? 1
  const priceUSD =
    (valuePerMillionTokens * groupRatio) /
    TOKEN_UNIT_DIVISORS[options.tokenUnit]
  const displayPrice = applyRechargeRate(
    priceUSD,
    options.showRechargePrice ?? false,
    priceRate,
    usdExchangeRate
  )

  return formatBillingCurrencyFromUSD(displayPrice, {
    showSymbol: options.showCurrencySymbol ?? true,
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}

export function formatTaskUsageUnitPrice(
  valuePerUnit: number,
  options: DynamicPriceOptions
): string {
  const groupRatio = options.groupRatioMultiplier ?? 1
  const priceRate = options.priceRate ?? 1
  const usdExchangeRate = options.usdExchangeRate ?? 1
  const priceUSD = valuePerUnit * groupRatio
  const displayPrice = applyRechargeRate(
    priceUSD,
    options.showRechargePrice ?? false,
    priceRate,
    usdExchangeRate
  )

  return formatBillingCurrencyFromUSD(displayPrice, {
    showSymbol: options.showCurrencySymbol ?? true,
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}

export function getDynamicPricingTiers(
  model: PricingModel
): DynamicPricingTier[] {
  if (!isDynamicPricingModel(model)) return []
  // 金额展示只信任后端投影；任务用量模型读取任务 USD 单位的严格投影。
  if (isTaskUsagePricingModel(model)) {
    return taskTiersFromBillingDisplay(model.billing_display)
  }
  return tiersFromBillingDisplay(model.billing_display)
}

export function hasDynamicRequestRules(model: PricingModel): boolean {
  if (!isDynamicPricingModel(model)) return false
  return ruleGroupsFromBillingDisplay(model.billing_display).length > 0
}

export function getDynamicPriceEntries(
  tier: DynamicPricingTier | null,
  options: DynamicPriceOptions
): DynamicPriceEntry[] {
  if (!tier) return []

  if (isTaskPricingTier(tier) && options.usageSchema) {
    const usageEntries: DynamicPriceEntry[] = getTaskNumberFields(
      options.usageSchema
    ).flatMap(([field, definition]) => {
      const value = Number(tier.unitPrices[field])
      if (!Number.isFinite(value) || value < 0 || !definition.unit) return []
      return [
        {
          key: field,
          field,
          label: field,
          shortLabel: field,
          labelKind: 'schema',
          value,
          formatted: formatTaskUsageUnitPrice(value, options),
          unit: definition.unit,
          description: definition.description,
        } satisfies DynamicPriceEntry,
      ]
    })
    if (tier.hasConstant || tier.constant > 0) {
      usageEntries.push({
        key: 'constant',
        field: 'constant',
        label: 'Additional charge',
        shortLabel: 'Additional charge',
        labelKind: 'i18n',
        value: tier.constant,
        formatted: formatTaskUsageUnitPrice(tier.constant, options),
        unit: 'request',
      })
    }
    return usageEntries
  }

  const entries: DynamicPriceEntry[] = BILLING_PRICING_VARS.flatMap(
    (variable) => {
      if (!variable.field) return []
      const value = Number((tier as ParsedTier)[variable.field])
      if (!Number.isFinite(value) || value < 0) return []

      return [
        {
          key: variable.key,
          field: variable.field,
          label: variable.label,
          shortLabel: variable.shortLabel,
          labelKind: 'i18n' as const,
          value,
          formatted: formatDynamicUnitPrice(value, options),
          unit: 'token' as const,
          variable,
        },
      ]
    }
  ).sort((a, b) => {
    const aPrimary = PRIMARY_DYNAMIC_FIELDS.has(a.field)
    const bPrimary = PRIMARY_DYNAMIC_FIELDS.has(b.field)
    if (aPrimary !== bPrimary) return aPrimary ? -1 : 1
    return 0
  })
  const fixedCharge = Number((tier as ParsedTier).constantCharge ?? 0)
  const hasFixedCharge =
    Number.isFinite(fixedCharge) &&
    (fixedCharge !== 0 || Boolean((tier as ParsedTier).hasConstant))
  if (hasFixedCharge) {
    entries.push({
      key: 'constant',
      field: 'constant',
      label: 'Additional charge',
      shortLabel: 'Additional charge',
      labelKind: 'i18n',
      value: fixedCharge,
      unit: 'request',
      formatted: formatTaskUsageUnitPrice(fixedCharge, options),
    })
  }
  return entries
}

export function getDynamicPricingSummary(
  model: PricingModel,
  options: DynamicPriceOptions
): DynamicPricingSummary | null {
  if (!isDynamicPricingModel(model)) return null

  const tiers = getDynamicPricingTiers(model)
  const isTaskUsage = isTaskUsagePricingModel(model)
  const tier = isTaskUsage ? (tiers.at(-1) ?? null) : (tiers[0] ?? null)
  const entryOptions = { ...options, usageSchema: model.billing_usage_schema }
  // 倍率场景已经覆盖实际分支，不再混入未乘倍率的底价；固定附加费也参与范围。
  const scenarios = isTaskUsage
    ? taskScenarioTiersFromBillingDisplay(model.billing_display)
    : tokenScenarioTiersFromBillingDisplay(model.billing_display)
  const rangeTiers: DynamicPricingTier[] =
    scenarios.length > 0 ? [] : [...tiers]
  for (const scenario of scenarios) {
    rangeTiers.push(...scenario.tiers)
  }
  const entriesByTier = rangeTiers.map(
    (tier) =>
      new Map(
        getDynamicPriceEntries(tier, entryOptions).map((entry) => [
          entry.field,
          entry,
        ])
      )
  )
  const representativeEntries = new Map(
    getDynamicPriceEntries(tier, entryOptions).map((entry) => [
      entry.field,
      entry,
    ])
  )
  const allEntries = new Map(representativeEntries)
  for (const tierEntries of entriesByTier) {
    for (const [field, entry] of tierEntries) {
      if (!allEntries.has(field)) allEntries.set(field, entry)
    }
  }
  const entries = [...allEntries.values()].map((entry) => {
    const format = (value: number) =>
      isTaskUsage || entry.unit === 'request'
        ? formatTaskUsageUnitPrice(value, options)
        : formatDynamicUnitPrice(value, options)
    // 只汇总投影中实际出现的收费项；某档没有该项表示该档不收取此项费用。
    const values = entriesByTier.map(
      (tierEntries) => tierEntries.get(entry.field)?.value ?? 0
    )
    const min = Math.min(...values)
    const max = Math.max(...values)
    const representative = representativeEntries.get(entry.field) ?? {
      ...entry,
      value: 0,
      formatted: format(0),
    }
    let formattedRange: string | undefined
    if (min !== max) {
      formattedRange = `${format(min)} – ${format(max)}`
    } else if (min !== representative.value) {
      // 多个场景同价时仍展示实际金额，不能退回未乘倍率的底价。
      formattedRange = format(min)
    }
    return {
      ...representative,
      formattedRange,
    }
  })
  const rawExpression = model.billing_expr || ''

  const constantEntry = entries.find((entry) => entry.unit === 'request')
  let primaryEntries = isTaskUsage
    ? entries.filter((entry) => entry.unit !== 'request')
    : entries.filter((entry) => PRIMARY_DYNAMIC_FIELDS.has(entry.field))
  const secondaryEntries = isTaskUsage
    ? entries.filter(
        (entry) => entry.unit === 'request' && entry !== constantEntry
      )
    : entries.filter(
        (entry) =>
          !PRIMARY_DYNAMIC_FIELDS.has(entry.field) && entry !== constantEntry
      )
  if (constantEntry) {
    // 固定附加费与用量费同时可见，纯固定费也保留显式零价。
    primaryEntries = [...primaryEntries, constantEntry]
  }

  return {
    tiers,
    tier,
    tierCount: tiers.length,
    hasRequestRules: hasDynamicRequestRules(model),
    isSpecialExpression: rawExpression.trim().length > 0 && tiers.length === 0,
    rawExpression,
    entries,
    primaryEntries,
    secondaryEntries,
    isTaskUsage,
  }
}

export function getCardExamplePrice(
  model: PricingModel,
  options: DynamicPriceOptions
): CardExamplePrice | null {
  if (!isTaskUsagePricingModel(model)) return null
  // 示例金额由后端按冻结表达式求值；本地不再对示例求值。
  const firstExample = model.billing_usage_examples?.[0]
  if (!firstExample || !Number.isFinite(firstExample.total)) return null
  return {
    label: firstExample.label,
    formatted: formatTaskUsageUnitPrice(firstExample.total as number, options),
  }
}
