import { useTranslation } from 'react-i18next'

import { ReportCard } from '@/components/report-card'

import { PersonalUsageOverview } from './personal-usage-overview'
import type { UsageAnalyticsEnvelope, UsageCustomerView } from './types'
import { UsageDailyBreakdown } from './usage-daily-breakdown'
import { AdminCustomerModelRanking, UsageModelRanking } from './usage-rankings'
import { UsageReportEmpty } from './usage-report-empty'
import { UsageReportStatus } from './usage-report-status'

export function UsageCustomerReport({
  report,
  personalView,
  tokenId,
}: {
  report: UsageAnalyticsEnvelope<UsageCustomerView>
  personalView?: UsageCustomerView
  tokenId?: number
}) {
  const { t } = useTranslation()
  const hasPublishedData = (report.published_days?.length ?? 1) > 0
  const view = personalView ?? report.result
  const hasUsage = view.keys.length > 0
  return (
    <>
      {!personalView && (
        <UsageReportStatus
          report={report}
          showReportPeriod
          showCoverage={false}
        />
      )}
      {hasPublishedData && (
        <>
          {hasUsage && <PersonalUsageOverview metrics={view.total} />}
          {hasUsage && !personalView && (
            <AdminCustomerModelRanking rows={view.model_ranks ?? []} />
          )}
          {hasUsage && personalView && report.period.period === 'month' && (
            <UsageDailyBreakdown
              period={report.period}
              days={view.day_totals}
              incompleteDays={report.incomplete_days}
            />
          )}
          {tokenId === undefined && !!report.pending_refunds?.length && (
            <ReportCard>
              <details>
                <summary className='cursor-pointer text-sm font-medium'>
                  {t('Pending refund')} · {report.pending_refunds.length}
                </summary>
                <ul className='mt-2 space-y-1 text-sm'>
                  {report.pending_refunds.map((row) => (
                    <li key={row.task_id}>
                      {t('Task ID')} {row.task_id}
                    </li>
                  ))}
                </ul>
              </details>
            </ReportCard>
          )}
          {hasUsage && personalView && <UsageModelRanking view={view} />}
          {!hasUsage &&
            !report.rebuilding_days?.length &&
            !report.source_unavailable_days?.length && (
              <UsageReportEmpty
                period={report.period}
                personal={!!personalView}
              />
            )}
        </>
      )}
    </>
  )
}
