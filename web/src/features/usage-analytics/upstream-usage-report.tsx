import { useTranslation } from 'react-i18next'

import { DayTotalsTable } from './customer-usage-table'
import { PersonalUsageOverview } from './personal-usage-overview'
import type { UsageAnalyticsEnvelope, UsageUpstreamView } from './types'
import { UsageDailyBreakdown } from './usage-daily-breakdown'
import { UsageUpstreamModelCharts } from './usage-rankings'

export function UpstreamUsageReport(props: {
  report: UsageAnalyticsEnvelope<UsageUpstreamView>
  showModels: boolean
  onDaySelect: (day: string) => void
}) {
  const { t } = useTranslation()
  const { result, period } = props.report
  return (
    <>
      {result.url_groups.length > 0 && (
        <PersonalUsageOverview metrics={result.total} upstream />
      )}
      {period.period === 'week' && (
        <DayTotalsTable
          upstream
          caption={t('Daily breakdown')}
          dates={period.days.map((day) => day.date)}
          futureFlags={period.days.map((day) => !!day.future)}
          unpublishedFlags={period.days.map((day) => !!day.unpublished)}
          incompleteDays={props.report.incomplete_days}
          days={result.day_totals}
          total={result.total}
          onDaySelect={props.onDaySelect}
        />
      )}
      {period.period === 'month' && (
        <UsageDailyBreakdown
          upstream
          period={period}
          days={result.day_totals}
          incompleteDays={props.report.incomplete_days}
          onDaySelect={props.onDaySelect}
        />
      )}
      {props.showModels && result.url_groups.length > 0 && (
        <UsageUpstreamModelCharts groups={result.url_groups} />
      )}
    </>
  )
}
