import { PersonalUsageOverview } from './personal-usage-overview'
import type { UsageAnalyticsEnvelope, UsageUpstreamView } from './types'
import { UsageUpstreamModelCharts } from './usage-rankings'

export function UpstreamUsageReport(props: {
  report: UsageAnalyticsEnvelope<UsageUpstreamView>
  showModels: boolean
}) {
  const { result } = props.report
  return (
    <>
      {result.url_groups.length > 0 && (
        <PersonalUsageOverview metrics={result.total} upstream />
      )}
      {props.showModels && result.url_groups.length > 0 && (
        <UsageUpstreamModelCharts groups={result.url_groups} />
      )}
    </>
  )
}
