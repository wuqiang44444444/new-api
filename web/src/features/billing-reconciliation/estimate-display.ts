// Stable reason codes are supplied by the statement projection. Never infer a
// historical discount from a missing amount or today's configuration.
const amountUnavailableReasonLabels: Record<string, string> = {
  original_quota_not_recorded: 'Verified list price is unavailable',
  original_replay_missing_context: 'Historical pricing inputs are incomplete',
  original_replay_missing_usage: 'Historical metered usage is incomplete',
  original_replay_settlement_mismatch: 'Recalculated charge differs from settlement',
  original_replay_unverified: 'Original amount could not be verified for this event',
  missing_contract: 'Historical contract status was not recorded',
  missing_group: 'Historical group ratio was not recorded',
  invalid_facts: 'The billing records are invalid',
  auxiliary_charge: 'Historical surcharge conversion could not be verified',
  amount_out_of_range: 'The amount is out of range',
  combination_limit: 'Combined row details are unavailable',
  published_billing_evidence_unavailable: 'Published billing evidence could not be read',
}

export function amountUnavailableReasonText(
  reasons: string[] | undefined,
  t: (key: string) => string
): string {
  const labels = (reasons ?? [])
    .filter((reason) => Object.hasOwn(amountUnavailableReasonLabels, reason))
    .map((reason) => t(amountUnavailableReasonLabels[reason]))
  return labels.length
    ? [...new Set(labels)].join(' · ')
    : t('Verified list price is unavailable')
}
