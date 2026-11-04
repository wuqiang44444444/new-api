import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'

import { getUsageSelfSummary } from './api'
import { CustomerUsageTable } from './customer-usage-table'
import { UsageExportButton } from './export-button'
import { UsagePeriodPicker } from './period-picker'
import { shanghaiToday } from './utils'

// Self-service day/week usage page: the caller only ever sees their own API
// Keys and customer models; upstream identity is absent from this payload.
export function MyUsage(props: {
  period: 'day' | 'week'
  date?: string
  onSearchChange: (search: { period: 'day' | 'week'; date: string }) => void
}) {
  const { t } = useTranslation()
  const today = shanghaiToday()
  const date = props.date ?? today
  const query = useQuery({
    queryKey: ['usage-self-summary', props.period, date],
    queryFn: () => getUsageSelfSummary({ period: props.period, date }),
  })

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <UsagePeriodPicker
          period={props.period}
          date={date}
          resolved={query.data?.data.period}
          onPeriodChange={(period) => props.onSearchChange({ period, date })}
          onDateChange={(nextDate) =>
            props.onSearchChange({ period: props.period, date: nextDate })
          }
        />
        <UsageExportButton
          body={{ period: props.period, date, view: 'self' }}
          period={query.data?.data.period}
          customerView={query.data?.data.result}
          ready={query.isSuccess && !query.isFetching}
        />
      </div>
      <p className='text-muted-foreground text-sm'>
        {t('Usage coverage notice')}
      </p>
      {query.isLoading && <LoadingState />}
      {query.isError && <ErrorState />}
      {query.data && (
        <CustomerUsageTable
          view={query.data.data.result}
          period={query.data.data.period}
        />
      )}
    </div>
  )
}
