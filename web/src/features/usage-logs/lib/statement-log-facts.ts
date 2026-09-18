import type { LogOtherData } from '../types'

// Apply the server-resolved historical facts for every usage-log consumer.
export function projectStatementLogFacts(parsed: LogOtherData): LogOtherData {
  const facts = parsed?.billing_facts
  if (!facts) return parsed
  // The statement endpoint has already resolved historical evidence. Keep
  // every existing desktop/mobile/detail consumer on that same projection.
  return {
    ...parsed,
    group_ratio: facts.group_ratio ?? undefined,
    user_group_ratio:
      facts.group_ratio_source === 'user_exclusive'
        ? (facts.group_ratio ?? undefined)
        : undefined,
    contract_applicable:
      facts.contract_applicable === 'unknown'
        ? undefined
        : facts.contract_applicable === 'yes',
    contract_discount:
      facts.contract_applicable === 'yes'
        ? (facts.contract_ratio ?? undefined)
        : undefined,
    contract_name: facts.contract_name || undefined,
    contract_version: facts.contract_version || undefined,
  }
}
