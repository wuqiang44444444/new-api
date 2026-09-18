// Stable reason codes are supplied by the statement projection. Never infer a
// historical discount from a missing amount or today's configuration.
const estimateReasonLabels: Record<string, string> = {
  missing_contract: 'Cannot estimate: historical contract status not recorded',
  missing_group: 'Cannot estimate: historical group ratio not recorded',
  invalid_facts: 'Estimate failed: invalid billing records',
  auxiliary_charge:
    'Cannot estimate: historical surcharge conversion is unverified',
  amount_out_of_range: 'Estimate failed: amount out of range',
  combination_limit: 'Cannot estimate: combined row details unavailable',
}

export function estimateFailureLabel(
  reasons: string[] | undefined,
  t: (key: string) => string
): string {
  if (
    reasons?.includes('invalid_facts') ||
    reasons?.includes('amount_out_of_range')
  ) {
    return t('Estimate failed')
  }
  return t('Cannot estimate')
}

export function estimateReasonText(
  reasons: string[] | undefined,
  t: (key: string) => string
): string {
  if (!reasons?.length) {
    return t(
      'Cannot estimate: reason not recorded in this historical statement'
    )
  }
  return [...new Set(reasons)]
    .map((reason) =>
      t(
        estimateReasonLabels[reason] ??
          'Cannot estimate: reason not recorded in this historical statement'
      )
    )
    .join('; ')
}
