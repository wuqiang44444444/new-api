import { useTranslation } from 'react-i18next'

// Shared business language for the form and its immutable history.
export function useSourceDecisionLabels() {
  const { t } = useTranslation()
  return {
    decisions: {
      keep_record: t('Keep the record; no additional refund'),
      investigate: t('Amount questioned; needs review'),
      request_waiver: t('Request a fee waiver'),
      request_duplicate_exclusion: t('Request removal of a duplicate charge'),
      request_period_correction: t('Request a billing period correction'),
      withdraw_request: t('Withdraw request; keep existing accounting'),
      reject_request: t('Reject request; keep existing accounting'),
    },
    reasons: {
      accept_missing_details: t('Accept missing historical billing details'),
      amount_questioned: t('The customer questions the amount'),
      missing_evidence: t('More billing evidence is needed'),
      customer_agreement: t('Proposed customer agreement'),
      service_issue: t('Service issue'),
      suspected_duplicate: t('Possible duplicate billing record'),
      wrong_period: t('Possibly assigned to the wrong billing period'),
      request_withdrawn: t('The request is withdrawn'),
      request_rejected: t('The request is rejected after review'),
    },
  }
}
