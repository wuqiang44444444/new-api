import type { UsageAnalyticsMetrics, UsageAnalyticsPeriod } from './types'

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
