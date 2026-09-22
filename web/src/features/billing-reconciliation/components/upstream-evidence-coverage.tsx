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
  const entries = [
    {
      filter: 'incomplete',
      count: coverage.gap_rows,
      text: t(
        '{{affected}} of {{total}} billing records have incomplete information',
        {
          affected: coverage.gap_rows.toLocaleString(),
          total: coverage.rows.toLocaleString(),
        }
      ),
    },
    {
      filter: 'complete',
      count: coverage.rows - coverage.gap_rows,
      text: t('Amount and usage information complete: {{count}} records', {
        count: (coverage.rows - coverage.gap_rows).toLocaleString(),
      }),
    },
    {
      filter: 'amount_gap',
      count: coverage.amount_gap_rows ?? 0,
      text: t('Amounts that cannot be calculated yet: {{count}} records', {
        count: (coverage.amount_gap_rows ?? 0).toLocaleString(),
      }),
    },
    {
      filter: 'usage_gap',
      count: coverage.usage_gap_rows ?? 0,
      text: t('Amount available, usage details missing: {{count}} records', {
        count: (coverage.usage_gap_rows ?? 0).toLocaleString(),
      }),
    },
    {
      filter: 'other_gap',
      count: coverage.other_gap_rows ?? 0,
      text: t(
        'Amount available, other billing details missing: {{count}} records',
        { count: (coverage.other_gap_rows ?? 0).toLocaleString() }
      ),
    },
  ]
  return (
    <div className='space-y-1 text-xs'>
      {entries
        .filter((entry, index) => index < 2 || entry.count > 0)
        .map((entry, index) => (
          <p
            key={entry.filter}
            className={index === 0 ? 'font-medium' : undefined}
          >
            {entry.text}
            {entry.count > 0 ? (
              <UpstreamEvidenceLink
                entry={entry}
                onViewEvidence={props.onViewEvidence}
              />
            ) : null}
          </p>
        ))}
      <p className='text-muted-foreground'>
        {t(
          'Complete information means it is recorded here; the supplier bill has not been reconciled.'
        )}
      </p>
    </div>
  )
}
