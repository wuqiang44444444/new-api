import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type { BillingReportManifest } from '../types'
import { formatShanghaiTimestamp } from '../upstream-reconciliation-utils'

export function BillingReportStatus({
  report,
}: {
  report?: BillingReportManifest
}) {
  const { t } = useTranslation()
  if (!report) return null
  let status = t('Published')
  if (report.published_days === 0) {
    status = t('Not published')
  } else if (
    report.published_days < report.expected_days ||
    report.incomplete_days
  ) {
    status = t('Partial data')
  }

  return (
    <div className='flex min-w-0 flex-wrap items-center gap-2'>
      <Badge variant='outline'>
        {status} ·{' '}
        {t('{{published}} of {{expected}} days published', {
          published: report.published_days,
          expected: report.expected_days,
        })}
        {!!report.incomplete_days && (
          <>
            {' · '}
            {t('Days with incomplete data · {{count}}', {
              count: report.incomplete_days,
            })}
          </>
        )}
      </Badge>
      {report.estimated && (
        <Badge variant='secondary'>
          {t('Data as of {{time}}', {
            time: formatShanghaiTimestamp(report.estimated_at ?? report.as_of),
          })}
        </Badge>
      )}
    </div>
  )
}
