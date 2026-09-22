import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { BillingEvidenceEntry } from '../upstream-statement-utils'

export function UpstreamEvidenceLink(props: {
  entry: BillingEvidenceEntry
  onViewEvidence?: (entry: BillingEvidenceEntry) => void
}) {
  const { t } = useTranslation()
  // Entries without a filter (display-only breakdown lines) have no evidence
  // view to open.
  if (!props.onViewEvidence || !props.entry.filter) return null
  return (
    <Button
      variant='link'
      size='xs'
      className='h-auto px-1 py-0 text-xs'
      aria-label={`${t('View records')}: ${props.entry.text}`}
      onClick={() => props.onViewEvidence?.(props.entry)}
    >
      {t('View records')}
    </Button>
  )
}
