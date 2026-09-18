import { useTranslation } from 'react-i18next'

// The server owns readability classification; both evidence entry points use
// the same display copy without exposing raw storage errors.
export function EvidenceBodyStatus(props: { status: string }) {
  const { t } = useTranslation()
  const labels: Record<string, string> = {
    missing: t('Evidence body file is missing'),
    decrypt_failed: t('Evidence body cannot be decrypted or authenticated'),
    integrity_failed: t('Evidence body integrity check failed'),
    storage_unavailable: t('Evidence storage is unavailable'),
    read_failed: t('Evidence body could not be read'),
    binary: t('Binary evidence has no text preview'),
    not_recorded: t('Evidence body unavailable'),
    expired: t('Evidence body expired'),
  }
  if (props.status === 'available') return null
  return (
    <p role='status'>
      {labels[props.status] ?? t('Evidence body unavailable')}
    </p>
  )
}
