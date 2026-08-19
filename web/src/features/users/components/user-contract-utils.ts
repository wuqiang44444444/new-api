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

export function draftPricePreview(rule: CustomerContractRule): CustomerContractRule['price'] {
  const discount = parseContractDiscount(rule.discount)
  const channel = Number(rule.native_group_ratio)
  if (discount === null || !Number.isFinite(channel)) return rule.price
  const effective = channel * discount
  const format = (value: number) =>
    value.toFixed(8).replace(/\.?0+$/, '')
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
