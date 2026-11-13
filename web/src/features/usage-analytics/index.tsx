import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { buttonVariants } from '@/components/ui/button'

import { getUsageSelfSummary, usageReportRebuildInterval } from './api'
import { UsageExportButton } from './export-button'
import { UsagePeriodPicker } from './period-picker'
import { PersonalUsageKeyFilter } from './personal-usage-overview'
import type { UsagePeriodKind } from './types'
import { UsageCustomerReport } from './usage-customer-report'
import { UsageReportStatus } from './usage-report-status'
import { shanghaiToday, shiftDate } from './utils'

// The caller only ever sees their own API
// Keys and customer models; upstream identity is absent from this payload.
export function MyUsage(props: {
  period: UsagePeriodKind
  date?: string
  onSearchChange: (search: { period: UsagePeriodKind; date: string }) => void
}) {
  const { t } = useTranslation()
  const today = shanghaiToday()
  const date = props.date ?? shiftDate(today, -1)
  const [tokenId, setTokenId] = useState<number>()
  const query = useQuery({
    queryKey: ['usage-self-summary', props.period, date],
    queryFn: ({ signal }) =>
      getUsageSelfSummary({ period: props.period, date }, signal),
    refetchInterval: usageReportRebuildInterval,
  })
  const report = query.data?.data
  const selectedKey = report?.result.keys.find(
    (key) => key.token_id === tokenId
  )
  const view = selectedKey
    ? {
        keys: [selectedKey],
        total: selectedKey.total,
        day_totals: selectedKey.days,
      }
    : report?.result

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4'>
      <h1 className='text-lg font-semibold tracking-tight'>{t('My usage')}</h1>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex min-w-0 flex-wrap items-center gap-2'>
          <UsagePeriodPicker
            period={props.period}
            date={date}
            resolved={query.data?.data.period}
            hideDailyRange
            onPeriodChange={(period) => props.onSearchChange({ period, date })}
            onMonthSelect={() =>
              props.onSearchChange({ period: 'month', date })
            }
            onDateChange={(nextDate) =>
              props.onSearchChange({ period: props.period, date: nextDate })
            }
          />
          {!!report?.result.keys.length && (
            <PersonalUsageKeyFilter
              keys={report.result.keys}
              selectedKey={selectedKey}
              onChange={setTokenId}
            />
          )}
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          {!!report?.result.keys.length && (
            <Link
              to='/billing'
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
              search={{
                month: report.period.date.slice(0, 7),
                modelKeyId: selectedKey?.token_id,
              }}
            >
              {t('My billing')}
            </Link>
          )}
          <UsageExportButton
            body={{
              period: props.period,
              date,
              view: 'self',
              token_id: selectedKey?.token_id,
              report_days: query.data?.data.report_days,
            }}
            period={query.data?.data.period}
            customerView={view}
            apiKey={selectedKey}
            ready={
              query.isSuccess &&
              !query.isFetching &&
              (query.data.data.published_days?.length ?? 1) > 0
            }
          />
        </div>
      </div>
      <UsageReportStatus report={query.data?.data} showReportPeriod />
      {query.isLoading && <LoadingState />}
      {query.isError && <ErrorState />}
      {report && view && (
        <UsageCustomerReport
          key={date}
          report={report}
          personalView={view}
          tokenId={selectedKey?.token_id}
        />
      )}
    </div>
  )
}
