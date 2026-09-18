import { formatCustomerStatementQuota } from './lib'
import type { BillingStatementVersionInfo } from './version-api'

// Frozen invoice conversion uses integer arithmetic and rounds only the final
// amount to the same eight decimal places as the server CSV.
function decimalFraction(value: number): [bigint, bigint] {
  const [mantissa, exponent = '0'] = String(value).toLowerCase().split('e')
  const [whole, fraction = ''] = mantissa.split('.')
  const scale = fraction.length - Number(exponent)
  const numerator = BigInt(whole + fraction)
  return scale >= 0
    ? [numerator, 10n ** BigInt(scale)]
    : [numerator * 10n ** BigInt(-scale), 1n]
}

export function formatVersionQuota(
  quota: number | string | null | undefined,
  version?: Pick<
    BillingStatementVersionInfo,
    'quota_per_unit' | 'currency' | 'currency_rate'
  > | null
): string {
  if (!version) {
    return formatCustomerStatementQuota(quota == null ? quota : Number(quota))
  }
  if (quota == null) return '—'
  if (!(version.quota_per_unit > 0) || !(version.currency_rate > 0)) return '—'
  const [unit, unitScale] = decimalFraction(version.quota_per_unit)
  const [rate, rateScale] = decimalFraction(version.currency_rate)
  const numerator = BigInt(quota) * rate * unitScale * 100000000n
  const denominator = unit * rateScale
  const absolute = numerator < 0n ? -numerator : numerator
  const rounded = (absolute * 2n + denominator) / (denominator * 2n)
  const digits = rounded.toString().padStart(9, '0')
  const amount = `${digits.slice(0, -8)}.${digits.slice(-8)}`
  let symbol = `${version.currency} `
  if (version.currency === 'USD') symbol = '$'
  if (version.currency === 'CNY') symbol = '¥'
  return `${numerator < 0n && rounded !== 0n ? '-' : ''}${symbol}${amount}`
}
