import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'

import {
  createCustomerStatementsExport,
  getCustomerStatementExportDays,
} from '../export-api'
import { formatBillingDate, formatBillingMonth } from '../lib'
import { useBillingExportJob } from '../use-billing-export-job'
import { BillingExportDrawer } from './billing-export-action'
import { BillingExportSection } from './billing-export-section'

export function CustomerStatementsExportAction({
  period,
  monthReportVersion,
}: {
  period: { start_timestamp: number; end_timestamp: number }
  monthReportVersion?: string
}) {
  const { t, i18n } = useTranslation()
  const dateId = useId()
  const statusId = useId()
  const [open, setOpen] = useState(false)
  const [mode, setMode] = useState<'day' | 'month'>('day')
  const [selection, setSelection] = useState<{ month: number; date: string }>()
  const daily = mode === 'day'
  const calendar = useQuery({
    queryKey: ['billing-customer-statement-export-days', period],
    queryFn: ({ signal }) => getCustomerStatementExportDays(period, signal),
    enabled: open && daily,
    retry: false,
  })
  const pickedDate =
    selection?.month === period.start_timestamp ? selection.date : undefined
  const day = calendar.data?.days.find(
    (option) => option.date === (pickedDate ?? calendar.data?.default_date)
  )
  const locale = toIntlLocale(i18n.language)
  const labels = {
    complete: t('Published · Full coverage'),
    incomplete: t('Partial data'),
    empty: t('Published · No billing records'),
    unpublished: t('Not published'),
    in_progress: t('In Progress'),
    rebuilding: t('Rebuilding'),
    source_unavailable: t('Report source unavailable'),
  }
  const ready = daily
    ? !!day?.report_version && !calendar.isFetching && !calendar.isError
    : !!monthReportVersion
  const job = useBillingExportJob(async () => {
    if (daily && ready && day?.report_version) {
      try {
        return await createCustomerStatementsExport({
          billing_date: day.date,
          report_version: day.report_version,
          expected_incomplete: day.status === 'incomplete',
        })
      } catch (error) {
        setSelection({ month: period.start_timestamp, date: day.date })
        void calendar.refetch()
        throw error
      }
    }
    if (!daily && monthReportVersion) {
      return createCustomerStatementsExport({
        start_timestamp: period.start_timestamp,
        end_timestamp: period.end_timestamp + 1,
        report_version: monthReportVersion,
      })
    }
    throw new Error(t('The current statement is unavailable.'))
  })

  let status
  if (calendar.isError) {
    status = (
      <>
        {t('Unable to load billing dates.')}
        <Button
          variant='outline'
          size='xs'
          onClick={() => void calendar.refetch()}
        >
          {t('Retry')}
        </Button>
      </>
    )
  } else if (calendar.isPending) {
    status = t('Loading...')
  } else if (!day) {
    status = t('No complete billing date is available in this period.')
  } else {
    let detail
    if (day.status === 'complete' && day.date === calendar.data?.default_date) {
      detail = t('Latest complete day available to export')
    } else if (day.status === 'incomplete') {
      detail = day.report_version
        ? t('The CSV will indicate incomplete coverage.')
        : t('No records to export.')
    }
    status = (
      <>
        <Badge variant='outline' className='h-auto whitespace-normal'>
          {labels[day.status]}
        </Badge>
        {detail}
      </>
    )
  }

  return (
    <BillingExportDrawer
      open={open}
      focusedJobId={job.focusedJobId}
      onOpenChange={(next) => {
        job.clearFocus()
        if (next) setSelection(undefined)
        setOpen(next)
      }}
    >
      <BillingExportSection
        title={t('Customer statements')}
        actionLabel={job.isPending ? t('Loading...') : t('Export CSV')}
        disabled={!ready || job.isPending}
        notice={
          !daily && !ready
            ? t('The current statement is unavailable.')
            : undefined
        }
        onSubmit={() => job.mutate()}
      >
        <Tabs
          value={mode}
          onValueChange={(value) => {
            if (value === 'day' || value === 'month') setMode(value)
          }}
        >
          <TabsList className='w-full' aria-label={t('Export period')}>
            <TabsTrigger value='month'>{t('By month')}</TabsTrigger>
            <TabsTrigger value='day'>{t('By day')}</TabsTrigger>
          </TabsList>
          <TabsContent value='day' className='space-y-2'>
            <div className='flex flex-wrap items-baseline justify-between gap-x-2 gap-y-1'>
              <Label htmlFor={dateId}>{t('Billing date')}</Label>
              <span className='text-muted-foreground text-xs'>
                {t('Shanghai time')} · UTC+8
              </span>
            </div>
            <NativeSelect
              id={dateId}
              className='w-full'
              value={day?.date ?? ''}
              disabled={!calendar.data?.days.length || calendar.isError}
              aria-describedby={statusId}
              onChange={(event) =>
                setSelection({
                  month: period.start_timestamp,
                  date: event.target.value,
                })
              }
            >
              <NativeSelectOption value='' disabled>
                {t('Select a billing date')}
              </NativeSelectOption>
              {calendar.data?.days.map((option) => (
                <NativeSelectOption
                  key={option.date}
                  value={option.date}
                  disabled={!option.report_version}
                >
                  {formatBillingDate(
                    Date.parse(`${option.date}T00:00:00+08:00`) / 1000,
                    locale
                  )}
                  {option.status !== 'complete' &&
                    ` · ${labels[option.status]}`}
                </NativeSelectOption>
              ))}
            </NativeSelect>
            <div
              id={statusId}
              role='status'
              className='text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-xs'
            >
              {status}
            </div>
          </TabsContent>
          <TabsContent value='month'>
            {formatBillingMonth(period.start_timestamp, locale)}
          </TabsContent>
        </Tabs>
        <Separator />
        <div className='space-y-1 text-sm'>
          <p>{t('All customers')}</p>
          <p className='text-muted-foreground text-xs'>
            {t('One row per customer. List filters do not affect the CSV.')}
          </p>
        </div>
      </BillingExportSection>
    </BillingExportDrawer>
  )
}
