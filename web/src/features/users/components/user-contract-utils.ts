import type { CustomerContractRule } from '../types'

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

export function draftEffectiveMultiplier(rule: CustomerContractRule): string {
  const nativeRatio = Number(rule.native_group_ratio)
  const discount = parseContractDiscount(rule.discount)
  if (!Number.isFinite(nativeRatio) || discount === null) return '—'
  const formatted = (nativeRatio * discount).toFixed(8).replace(/\.?0+$/, '')
  return formatted || '0'
}

export function draftCurrentPrice(rule: CustomerContractRule): string {
  const currentPrice = Number(rule.price.current_discounted_price)
  const savedMultiplier = Number(rule.effective_multiplier)
  const draftMultiplier = Number(draftEffectiveMultiplier(rule))
  if (
    !Number.isFinite(currentPrice) ||
    !Number.isFinite(savedMultiplier) ||
    savedMultiplier <= 0 ||
    !Number.isFinite(draftMultiplier)
  ) {
    return '—'
  }
  return ((currentPrice / savedMultiplier) * draftMultiplier)
    .toFixed(8)
    .replace(/\.?0+$/, '')
}
