import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import { CustomerUsageTable } from './customer-usage-table'
import { PersonalUsageOverview } from './personal-usage-overview'
import type { UsageAnalyticsEnvelope, UsageCustomerView } from './types'
import { UsageDailyBreakdown } from './usage-daily-breakdown'
import { UsageModelRanking, UsageOverview } from './usage-rankings'
import { UsageReportEmpty } from './usage-report-empty'
import { UsageReportStatus } from './usage-report-status'

export function UsageCustomerReport({
  report,
  onDaySelect,
  personalView,
  tokenId,
  modelName,
  onModelChange,
  adminCharts,
}: {
  report: UsageAnalyticsEnvelope<UsageCustomerView>
  onDaySelect?: (day: string) => void
  personalView?: UsageCustomerView
  tokenId?: number
  modelName?: string
  onModelChange?: (modelName: string | undefined) => void
  adminCharts?: boolean
}) {
  const { t } = useTranslation()
  const hasPublishedData = (report.published_days?.length ?? 1) > 0
  const view = personalView ?? report.result
  const hasUsage = view.keys.length > 0
  const details = hasUsage && !personalView && (
    <>
      <UsageModelRanking
        view={view}
        selected={modelName}
        onSelect={onModelChange}
      />
      <CustomerUsageTable
        view={view}
        period={report.period}
        modelName={modelName}
        onDaySelect={onDaySelect}
      />
    </>
  )
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
          {hasUsage &&
            (personalView || adminCharts ? (
              <PersonalUsageOverview metrics={view.total} />
            ) : (
              <UsageOverview metrics={view.total} />
            ))}
          {hasUsage && adminCharts && <UsageModelRanking view={view} />}
          {hasUsage &&
            (report.period.period === 'month' ||
              (personalView && report.period.period === 'week')) && (
              <UsageDailyBreakdown
                period={report.period}
                days={view.day_totals}
                incompleteDays={report.incomplete_days}
                onDaySelect={onDaySelect}
              />
            )}
          {tokenId === undefined && !!report.pending_refunds?.length && (
            <details className='rounded-md border p-3'>
              <summary className='cursor-pointer font-medium'>
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
          )}
          {hasUsage && personalView && (
            <UsageModelRanking view={view} showDetails />
          )}
          {adminCharts && details && (
            <Collapsible>
              <CollapsibleTrigger
                render={<Button size='sm' variant='outline' />}
              >
                {t('View details')}
              </CollapsibleTrigger>
              <CollapsibleContent className='mt-3 space-y-3'>
                {details}
              </CollapsibleContent>
            </Collapsible>
          )}
          {!adminCharts && details}
          {!hasUsage && (
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
