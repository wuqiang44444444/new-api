import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'

import { createUsageSelfExport, getUsageSelfSummary } from './api'
import { CustomerUsageTable } from './customer-usage-table'
import { UsagePeriodPicker } from './period-picker'
import { shanghaiToday } from './utils'

// Self-service day/week usage page: the caller only ever sees their own API
// Keys and customer models; upstream identity is absent from this payload.
export function MyUsage(props: {
  period: 'day' | 'week'
  date?: string
  onSearchChange: (search: { period: 'day' | 'week'; date: string }) => void
}) {
  const { t, i18n } = useTranslation()
  const today = shanghaiToday()
  const date = props.date ?? today
  const query = useQuery({
    queryKey: ['usage-self-summary', props.period, date],
    queryFn: () => getUsageSelfSummary({ period: props.period, date }),
  })
  const exportMutation = useSelfExportMutation()

  return (
    <div className='flex flex-col gap-4 p-4'>
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
        <Button
          size='sm'
          disabled={exportMutation.isPending || !query.data}
          onClick={() =>
            exportMutation.mutate({
              period: props.period,
              date,
              language: i18n.language.startsWith('zh') ? 'zh' : 'en',
            })
          }
        >
          {t('Export CSV')}
        </Button>
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

function useSelfExportMutation() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (body: {
      period: 'day' | 'week'
      date: string
      language: string
    }) => {
      const response = await createUsageSelfExport(body)
      if (!response.success) {
        throw new Error(response.message)
      }
      return response.data
    },
    onSuccess: () => {
      toast.success(
        t('Export job queued; check billing export tasks for the file')
      )
      queryClient.invalidateQueries({ queryKey: ['self-export-jobs'] })
    },
    onError: (error: Error) => {
      toast.error(error.message)
    },
  })
}
