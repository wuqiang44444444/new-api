import { useTranslation } from 'react-i18next'

import type { BillingDataQuality } from '../types'
import type { BillingEvidenceEntry } from '../upstream-statement-utils'
import { UpstreamEvidenceLink } from './upstream-evidence-link'

export function UpstreamEvidenceCoverage(props: {
  quality?: BillingDataQuality
  onViewEvidence?: (entry: BillingEvidenceEntry) => void
}) {
  const { t } = useTranslation()
  const coverage = props.quality?.evidence_coverage
  if (!coverage) return null
  const pending = coverage.amount_gap_rows ?? 0
  const pendingEntry = {
    filter: 'amount_gap',
    text: t('Original amount pending: {{count}} records', {
      count: pending.toLocaleString(),
    }),
  }
  return (
    <div className='space-y-1 text-xs'>
      <p>
        {t('Original amount available: {{count}} records', {
          count: (coverage.rows - pending).toLocaleString(),
        })}
      </p>
      <p>
        {pendingEntry.text}
        {pending > 0 && (
          <UpstreamEvidenceLink
            entry={pendingEntry}
            onViewEvidence={props.onViewEvidence}
          />
        )}
      </p>
    </div>
  )
}
