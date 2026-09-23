import type {
  ContractRuleDraft,
  CustomerContractChannelGroupOption,
  CustomerContractChannelOption,
  CustomerContractGroupOption,
  CustomerContractRule,
} from '../types'

export function channelOptionsForRule(
  channelGroups: CustomerContractChannelGroupOption[],
  rule: Pick<CustomerContractRule, 'route_group' | 'model'>
): CustomerContractChannelOption[] {
  return (
    channelGroups
      .find((group) => group.group === rule.route_group)
      ?.models.find((entry) => entry.model === rule.model)?.channels ?? []
  )
}

export function parseContractDiscount(raw: string): number | null {
  const value = raw.trim()
  let divisor = 1
  let maxDecimals = 8
  let numberText = value
  if (value.endsWith('%')) {
    divisor = 100
    maxDecimals = 6
    numberText = value.slice(0, -1).trim()
  } else if (value.endsWith('折')) {
    divisor = 10
    maxDecimals = 7
    numberText = value.slice(0, -1).trim()
  }
  const decimalPattern = new RegExp(`^\\d+(?:\\.\\d{1,${maxDecimals}})?$`)
  if (!decimalPattern.test(numberText)) return null
  const parsed = Number(numberText) / divisor
  return Number.isFinite(parsed) && parsed > 0 && parsed <= 1 ? parsed : null
}

export function normalizeContractDiscount(raw: string): string | null {
  const parsed = parseContractDiscount(raw)
  if (parsed === null) return null
  return parsed.toFixed(8).replace(/\.?0+$/, '')
}

/**
 * Validated result of one batch add. `key`/`params` are i18n inputs for the
 * callers' `t()`; a failed batch never returns partial rules.
 */
export type ContractBatchAddResult =
  | { ok: true; rules: ContractRuleDraft[] }
  | { ok: false; error: { key: string; params?: Record<string, string> } }

export interface ContractBatchAddInput {
  channelGroups: CustomerContractChannelGroupOption[]
  groupOptions: CustomerContractGroupOption[]
  draftRules: ContractRuleDraft[]
  routeGroup: string
  models: string[]
  channelIdsByModel: Record<string, string[]>
  discount: string
}

/**
 * Validates a whole pending batch (models of one add) against itself and the
 * current draft, then expands it into one rule per selected channel. Every
 * rule keeps its own model price and the route group's native ratio facts;
 * any failure rejects the entire batch so callers never append partially.
 */
export function buildContractBatchRules(
  input: ContractBatchAddInput
): ContractBatchAddResult {
  const models = [...new Set(input.models)]
  const normalizedDiscount = normalizeContractDiscount(input.discount)
  if (!input.routeGroup || models.length === 0) {
    return { ok: true, rules: [] }
  }
  if (normalizedDiscount === null) {
    return { ok: false, error: { key: 'Invalid contract discount' } }
  }
  // Two precise names that differ only by letter case can never coexist in
  // one contract, so the pending batch may not contain both spellings.
  const batchCaseKeys = new Map<string, string>()
  for (const model of models) {
    const caseKey = model.toLowerCase()
    const batchTwin = batchCaseKeys.get(caseKey)
    if (batchTwin !== undefined && batchTwin !== model) {
      return {
        ok: false,
        error: {
          key: 'Model names that differ only by letter case cannot coexist',
        },
      }
    }
    batchCaseKeys.set(caseKey, model)
  }
  const channelGroup = input.channelGroups.find(
    (group) => group.group === input.routeGroup
  )
  const groupOption = input.groupOptions.find(
    (option) => option.group === input.routeGroup
  )
  const rules: ContractRuleDraft[] = []
  for (const model of models) {
    const modelEntry = channelGroup?.models.find(
      (entry) => entry.model === model
    )
    const candidates = modelEntry?.channels ?? []
    if (!modelEntry) {
      return {
        ok: false,
        error: {
          key: 'Model {{model}} is no longer a candidate in this group. Reselect models.',
          params: { model },
        },
      }
    }
    const selectedIds = (input.channelIdsByModel[model] ?? []).map(Number)
    if (selectedIds.length === 0) {
      return {
        ok: false,
        error: {
          key: 'Select a channel for model {{model}}',
          params: { model },
        },
      }
    }
    const candidateIds = new Set(candidates.map((channel) => channel.id))
    if (selectedIds.some((id) => !candidateIds.has(id))) {
      return {
        ok: false,
        error: {
          key: 'Selected channels of {{model}} are no longer available. Reselect its channels.',
          params: { model },
        },
      }
    }
    const sameModelRules = input.draftRules.filter(
      (rule) => rule.model.toLowerCase() === model.toLowerCase()
    )
    if (sameModelRules.some((rule) => rule.model !== model)) {
      return {
        ok: false,
        error: {
          key: 'Model names that differ only by letter case cannot coexist',
        },
      }
    }
    // The contract-level unique key is model + channel regardless of route
    // group, so a duplicate in another group is still a conflict.
    const boundChannels = candidates.filter(
      (channel) =>
        selectedIds.includes(channel.id) &&
        sameModelRules.some((rule) => rule.channel_id === channel.id)
    )
    if (boundChannels.length > 0) {
      return {
        ok: false,
        error: {
          key: 'This model already binds channels: {{channels}}',
          params: {
            channels: boundChannels.map((channel) => channel.name).join(', '),
          },
        },
      }
    }
    if (sameModelRules.some((rule) => rule.discount !== normalizedDiscount)) {
      return {
        ok: false,
        error: {
          key: 'All channels of one model must share the same contract discount in this save',
        },
      }
    }
    for (const channelId of selectedIds) {
      rules.push({
        model,
        channel_id: channelId,
        route_group: input.routeGroup,
        discount: normalizedDiscount,
        available: true,
        native_group_ratio: channelGroup?.native_group_ratio || '1',
        effective_multiplier: channelGroup?.native_group_ratio || '1',
        special_group_ratio: channelGroup?.special_group_ratio || false,
        price: groupOption?.prices?.[model] || {
          price_type: 'model_ratio' as const,
        },
      })
    }
  }
  return { ok: true, rules }
}

export function draftEffectiveMultiplier(rule: CustomerContractRule): string {
  const nativeRatio = Number(rule.native_group_ratio)
  const discount = parseContractDiscount(rule.discount)
  if (!Number.isFinite(nativeRatio) || discount === null) return '—'
  const formatted = (nativeRatio * discount).toFixed(8).replace(/\.?0+$/, '')
  return formatted || '0'
}

export function draftPricePreview(
  rule: CustomerContractRule
): CustomerContractRule['price'] {
  const discount = parseContractDiscount(rule.discount)
  const channel = Number(rule.native_group_ratio)
  if (discount === null || !Number.isFinite(channel)) return rule.price
  const effective = channel * discount
  const format = (value: number) => value.toFixed(8).replace(/\.?0+$/, '')
  const price = { ...rule.price }
  if (price.base_model_ratio) {
    price.final_model_ratio = format(Number(price.base_model_ratio) * effective)
    price.current_discounted_price = price.final_model_ratio
  }
  if (price.base_image_ratio) {
    price.final_image_ratio = format(Number(price.base_image_ratio) * effective)
  }
  if (price.base_model_price) {
    price.final_model_price = format(Number(price.base_model_price) * effective)
    price.current_discounted_price = price.final_model_price
  }
  if (
    !price.base_model_ratio &&
    !price.base_model_price &&
    price.current_discounted_price
  ) {
    const current = Number(price.current_discounted_price)
    const saved = Number(rule.effective_multiplier)
    if (Number.isFinite(current) && Number.isFinite(saved) && saved > 0) {
      price.current_discounted_price = format((current / saved) * effective)
    }
  }
  return price
}
