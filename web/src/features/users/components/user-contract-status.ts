import type { User } from '../types'

type Translate = (key: string, options?: Record<string, unknown>) => string

export function getUserContractStatus(user: User, t: Translate) {
  const summary = user.contract_summary
  if (!summary) {
    return {
      label: t('Contract summary unavailable'),
      variant: 'neutral' as const,
    }
  }
  if (summary.total === 0) {
    return { label: t('No contracts'), variant: 'neutral' as const }
  }
  return {
    label: t('Contracts: {{total}} · enabled: {{enabled}}', {
      total: summary.total,
      enabled: summary.enabled,
    }),
    variant: summary.enabled > 0 ? ('info' as const) : ('neutral' as const),
  }
}
