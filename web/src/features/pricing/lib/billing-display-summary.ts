import type { PricingModel } from '../types'
import type { DynamicPricingTier, DynamicPriceEntry, DynamicPriceOptions, DynamicPricingSummary } from './dynamic-price'
import { taskScenarioTiersFromBillingDisplay, tokenScenarioTiersFromBillingDisplay } from './billing-display'
export function getProjectedPricingSummary(
  model: PricingModel,
  options: DynamicPriceOptions,
  context: {
    tiers: DynamicPricingTier[]
    isTaskUsage: boolean
    getEntries: (tier: DynamicPricingTier | null, options: DynamicPriceOptions) => DynamicPriceEntry[]
    formatTaskPrice: (value: number, options: DynamicPriceOptions) => string
    formatTokenPrice: (value: number, options: DynamicPriceOptions) => string
    primaryFields: ReadonlySet<string>
    hasRequestRules: boolean
  }
): DynamicPricingSummary | null {
  const { tiers, isTaskUsage } = context
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
        context.getEntries(tier, entryOptions).map((entry) => [
          entry.field,
          entry,
        ])
      )
  )
  const representativeEntries = new Map(
    context.getEntries(tier, entryOptions).map((entry) => [
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
        ? context.formatTaskPrice(value, options)
        : context.formatTokenPrice(value, options)
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
    : entries.filter((entry) => (context.primaryFields.has(entry.field) || entry.field === 'imageOutputPrice'))
  const secondaryEntries = isTaskUsage
    ? entries.filter(
        (entry) => entry.unit === 'request' && entry !== constantEntry
      )
    : entries.filter(
        (entry) =>
          !(context.primaryFields.has(entry.field) || entry.field === 'imageOutputPrice') && entry !== constantEntry
      )
  if (constantEntry) {
    // 固定附加费与用量费同时可见，纯固定费也保留显式零价。
    primaryEntries = [...primaryEntries, constantEntry]
  }

  return {
    tiers,
    tier,
    tierCount: tiers.length,
    hasRequestRules: context.hasRequestRules,
    isSpecialExpression: rawExpression.trim().length > 0 && tiers.length === 0,
    rawExpression,
    entries,
    primaryEntries,
    secondaryEntries,
    isTaskUsage,
  }
}
