import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'

export function SourceReviewError({
  error,
  operation = 'check',
}: {
  error: unknown
  operation?: 'check' | 'decision' | 'verification'
}) {
  const { t } = useTranslation()
  const response = isAxiosError<{ reason?: string; request_id?: string }>(error)
    ? error.response
    : undefined
  const messages: Record<string, string> = {
    timeout: t('The check timed out. Try again.'),
    limit_exceeded: t(
      'The records exceed the check limit. Use the request ID to investigate.'
    ),
    maintenance: t(
      'Billing records are under maintenance. Try again when maintenance ends.'
    ),
    unsupported_topology: t(
      'Statement checks are unavailable for this database configuration.'
    ),
    conflict:
      operation !== 'check'
        ? t('The records changed. Run the check again before submitting.')
        : t('Statement records changed during the check. Try again.'),
    decision_rejected: t(
      'This decision could not be recorded. Check the latest records and try again.'
    ),
  }
  const fallback = {
    check: t('Source checks failed. No completeness conclusion is available.'),
    decision: t(
      'Unable to save the decision. Refresh to check for changes before retrying; do not assume it was completed.'
    ),
    verification: t(
      'Verification was rejected. Refresh the checks before retrying.'
    ),
  }[operation]
  const reason = response?.data?.reason
  const requestId =
    response?.data?.request_id ?? response?.headers?.['x-oneapi-request-id']
  return (
    <div role='alert' className='text-destructive text-sm'>
      <p>
        {(reason && Object.hasOwn(messages, reason) && messages[reason]) ||
          fallback}
      </p>
      {typeof requestId === 'string' && requestId && (
        <p>{t('Request ID {{id}}', { id: requestId })}</p>
      )}
    </div>
  )
}
