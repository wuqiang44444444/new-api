import type { TFunction } from 'i18next'

import type {
  UsageAnalyticsMetrics,
  UsageAnalyticsPeriod,
  UsageCustomerKeyGroup,
  UsageNonTokenMetrics,
} from './types'

export function usageKeyLabel(t: TFunction, key: UsageCustomerKeyGroup) {
  if (key.token_id === 0) return t('No API Key')
  const name = key.token_name || `#${key.token_id}`
  return key.token_deleted ? `${name} · ${t('Deleted key')}` : name
}

// The server attributes all statistics to the Asia/Shanghai calendar, so the
// default "today" is computed in that zone, not the browser zone.
export function shanghaiToday(): string {
  return new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date())
}

export function shiftDate(date: string, days: number): string {
  const next = new Date(`${date}T00:00:00Z`)
  next.setUTCDate(next.getUTCDate() + days)
  return next.toISOString().slice(0, 10)
}

export function shiftMonth(date: string, months: number): string {
  const next = new Date(`${date.slice(0, 7)}-01T00:00:00Z`)
  next.setUTCMonth(next.getUTCMonth() + months)
  return next.toISOString().slice(0, 10)
}

export function formatPeriodRange(period: UsageAnalyticsPeriod): string {
  if (period.period === 'day') {
    return period.date
  }
  const first = period.days[0]?.date ?? ''
  const last = period.days.at(-1)?.date ?? ''
  return `${first} ~ ${last}`
}

export function formatNumber(value: number | undefined | null): string {
  if (value === undefined || value === null) {
    return '—'
  }
  return value.toLocaleString('en-US')
}

export function formatQuotaAmount(value: number | undefined | null): string {
  if (value === undefined || value === null) {
    return '—'
  }
  return value.toLocaleString('en-US')
}

export function hasActivity(metrics: UsageAnalyticsMetrics): boolean {
  return (
    metrics.total_calls > 0 ||
    metrics.net_quota !== 0 ||
    metrics.gross_quota !== 0
  )
}

export function usageNonTokenValues(
  t: TFunction,
  metrics?: UsageNonTokenMetrics,
  upstream = false
) {
  const values = []
  const imageCount = metrics?.image_count ?? 0
  const imageMissing = metrics?.image_count_missing_rows ?? 0
  const imageKnown = metrics?.image_count_known || imageCount > 0
  if (!upstream && (imageKnown || imageMissing > 0)) {
    const value = formatNumber(imageCount)
    let text = t('Image count unrecorded')
    if (imageKnown) {
      text =
        imageCount === 1
          ? t('{{value}} image', { value })
          : t('{{value}} images', { value })
    }
    values.push({
      label: t('Images generated'),
      value: imageKnown ? value : t('unrecorded'),
      text,
      partial: imageKnown && imageMissing > 0,
    })
  }
  for (const [label, count, missing] of [
    [
      t('Billable image count'),
      metrics?.billable_images,
      metrics?.billable_image_missing_rows,
    ],
    [
      t('Channel test billable images'),
      metrics?.test_billable_images,
      metrics?.test_billable_image_missing_rows,
    ],
  ] as const) {
    if (!count && !missing) continue
    const value = count ? formatNumber(count) : t('unrecorded')
    values.push({
      label,
      value,
      text: `${label} · ${value}`,
      partial: !!count && !!missing,
      showSubtotal: !!count && !!missing,
    })
  }
  const seconds = metrics?.seconds
  const secondsMissing = metrics?.seconds_missing_rows ?? 0
  const secondsValueMissing = metrics?.seconds_value_missing_rows ?? 0
  if (seconds != null || secondsMissing > 0) {
    const notes = []
    const missing = secondsMissing - secondsValueMissing
    if (missing > 0) {
      notes.push(t('Seconds unrecorded ({{count}} rows)', { count: missing }))
    }
    if (secondsValueMissing > 0) {
      notes.push(
        t('Second unit known; measured duration unrecorded ({{count}} rows)', {
          count: secondsValueMissing,
        })
      )
    }
    values.push({
      label: t('Billing seconds'),
      value: seconds ?? t('unrecorded'),
      text:
        seconds == null
          ? undefined
          : t('{{value}} billing seconds', { value: seconds }),
      partial: seconds != null && secondsMissing > 0,
      notes,
    })
  }
  if (upstream && metrics?.billing_meter_unknown_rows) {
    const label = t('Billing mode')
    const value = t('{{count}} rows unrecorded', {
      count: metrics.billing_meter_unknown_rows,
    })
    values.push({ label, value, text: `${label} · ${value}`, partial: false })
  }
  return values
}

export function mergeUsageNonToken(
  total: UsageNonTokenMetrics,
  metrics: UsageNonTokenMetrics
): void {
  total.billing_tokens_applicable ||= !!metrics.billing_tokens_applicable
  total.billing_meter_unknown_rows =
    (total.billing_meter_unknown_rows ?? 0) +
    (metrics.billing_meter_unknown_rows ?? 0)
  total.image_count = (total.image_count ?? 0) + (metrics.image_count ?? 0)
  total.image_count_known ||= !!metrics.image_count_known
  total.image_count_missing_rows =
    (total.image_count_missing_rows ?? 0) +
    (metrics.image_count_missing_rows ?? 0)
  total.billable_images =
    (total.billable_images ?? 0) + (metrics.billable_images ?? 0)
  total.billable_image_missing_rows =
    (total.billable_image_missing_rows ?? 0) +
    (metrics.billable_image_missing_rows ?? 0)
  total.test_billable_images =
    (total.test_billable_images ?? 0) + (metrics.test_billable_images ?? 0)
  total.test_billable_image_missing_rows =
    (total.test_billable_image_missing_rows ?? 0) +
    (metrics.test_billable_image_missing_rows ?? 0)
  total.seconds_missing_rows =
    (total.seconds_missing_rows ?? 0) + (metrics.seconds_missing_rows ?? 0)
  total.seconds_value_missing_rows =
    (total.seconds_value_missing_rows ?? 0) +
    (metrics.seconds_value_missing_rows ?? 0)
  if (metrics.seconds == null) return
  if (total.seconds == null) {
    total.seconds = metrics.seconds
    return
  }
  // The report sends exact decimal strings. Keep fractional billing seconds
  // exact when combining model rows across keys or billing modes.
  const [left, leftFraction = ''] = total.seconds.split('.')
  const [right, rightFraction = ''] = metrics.seconds.split('.')
  const scale = Math.max(leftFraction.length, rightFraction.length)
  const sum =
    BigInt(left + leftFraction.padEnd(scale, '0')) +
    BigInt(right + rightFraction.padEnd(scale, '0'))
  const digits = sum.toString().padStart(scale + 1, '0')
  const fraction = scale ? digits.slice(-scale).replace(/0+$/, '') : ''
  const whole = scale ? digits.slice(0, -scale) : digits
  total.seconds = fraction ? `${whole}.${fraction}` : whole
}
