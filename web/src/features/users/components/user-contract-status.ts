import type { User } from '../types'

type Translate = (key: string, options?: Record<string, unknown>) => string

interface ContractStatusSource {
  contractMode: boolean
  contractVersion: number
  ruleCount: number
}

export function getContractStatusPresentation(
  source: ContractStatusSource,
  t: Translate
) {
  if (source.contractMode && source.ruleCount === 0) {
    return {
      label: t('Contract active · no model access'),
      variant: 'danger' as const,
    }
  }
  if (source.contractMode) {
    return {
      label: t('Contract active · {{count}} rules', {
        count: source.ruleCount,
      }),
      variant: 'warning' as const,
    }
  }
  if (source.contractVersion > 0) {
    return {
      label: t('Contract inactive · {{count}} retained rules', {
        count: source.ruleCount,
      }),
      variant: 'neutral' as const,
    }
  }
  return { label: t('Native mode'), variant: 'success' as const }
}

export function getUserContractStatus(user: User, t: Translate) {
  return getContractStatusPresentation(
    {
      contractMode: user.contract_mode,
      contractVersion: user.contract_version,
      ruleCount: user.contract_rule_count,
    },
    t
  )
}
