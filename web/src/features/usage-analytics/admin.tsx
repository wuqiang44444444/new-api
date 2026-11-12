import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { AdminCustomerUsage } from './admin-customer-usage'
import { AdminUpstreamUsage } from './admin-upstream-usage'
import { UsagePeriodPicker } from './period-picker'
import type { UsagePeriodKind } from './types'
import { shanghaiToday, shiftDate } from './utils'

type AdminUsageSection = 'customers' | 'upstream'

export function AdminUsage(props: {
  period: UsagePeriodKind
  date?: string
  section?: AdminUsageSection
  onSearchChange: (search: {
    period: UsagePeriodKind
    date: string
    section?: AdminUsageSection
  }) => void
}) {
  const { t } = useTranslation()
  const scrollContainer = useRef<HTMLDivElement>(null)
  const date = props.date ?? shiftDate(shanghaiToday(), -1)
  const [drill, setDrill] = useState<{
    date: string
    period: UsagePeriodKind
    reportDays?: string
    sourcePeriod?: {
      period: Exclude<UsagePeriodKind, 'day'>
      date: string
      reportDays?: string
    }
  }>()
  const section = props.section ?? 'customers'
  const period = props.period
  const reportDays =
    section === 'upstream' && drill?.date === date && drill.period === period
      ? drill.reportDays
      : undefined
  const sourcePeriod =
    section === 'upstream' && period === 'day' && drill?.date === date
      ? drill.sourcePeriod
      : undefined
  const setSection = (section: string) => {
    setDrill(undefined)
    props.onSearchChange({
      period,
      date,
      section: section as AdminUsageSection,
    })
  }
  return (
    <div
      ref={scrollContainer}
      className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4'
    >
      <h1 className='text-lg font-semibold tracking-tight'>
        {section === 'customers'
          ? t('Admin customer usage')
          : t('Admin upstream usage')}
      </h1>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <UsagePeriodPicker
          period={period}
          date={date}
          resolved={undefined}
          hideDailyRange
          onPeriodChange={(period) => {
            setDrill(undefined)
            props.onSearchChange({ period, date, section: props.section })
          }}
          onMonthSelect={() => {
            setDrill(undefined)
            props.onSearchChange({ period: 'month', date, section })
          }}
          onDateChange={(nextDate) => {
            setDrill(undefined)
            props.onSearchChange({
              period,
              date: nextDate,
              section: props.section,
            })
          }}
        />
      </div>
      <Tabs value={section} onValueChange={setSection}>
        <TabsList>
          <TabsTrigger value='customers'>{t('Customer usage')}</TabsTrigger>
          <TabsTrigger value='upstream'>{t('Upstream usage')}</TabsTrigger>
        </TabsList>
        <TabsContent value='customers'>
          <AdminCustomerUsage
            scrollContainer={scrollContainer}
            period={period}
            date={date}
          />
        </TabsContent>
        <TabsContent value='upstream'>
          <AdminUpstreamUsage
            scrollContainer={scrollContainer}
            period={period}
            date={date}
            reportDays={reportDays}
            onDaySelect={(day, reportDays, periodReportDays) => {
              if (period === 'day') return
              setDrill({
                period: 'day',
                date: day,
                reportDays,
                sourcePeriod: { period, date, reportDays: periodReportDays },
              })
              props.onSearchChange({
                period: 'day',
                date: day,
                section: 'upstream',
              })
            }}
            returnToPeriod={
              sourcePeriod
                ? {
                    period: sourcePeriod.period,
                    onClick: () => {
                      setDrill(sourcePeriod)
                      props.onSearchChange({
                        period: sourcePeriod.period,
                        date: sourcePeriod.date,
                        section: 'upstream',
                      })
                    },
                  }
                : undefined
            }
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}
