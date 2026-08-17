import type { User } from '../types'

type Translate = (key: string, options?: Record<string, unknown>) => string

export function getUserContractStatus(user: User, t: Translate) {
  const ruleCount = user.contract_rule_count || 0
  if (user.contract_mode && ruleCount === 0) {
    return {
      label: t('Contract active · no model access'),
      variant: 'danger' as const,
    }
  }
  if (user.contract_mode) {
    return {
      label: t('Contract active · {{count}} rules', { count: ruleCount }),
      variant: 'warning' as const,
    }
  }
  if ((user.contract_version || 0) > 0) {
    return {
      label: t('Contract inactive · {{count}} retained rules', {
        count: ruleCount,
      }),
      variant: 'neutral' as const,
    }
  }
  return { label: t('Native mode'), variant: 'success' as const }
}
