import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  UsageCallsCell,
  UsageMoneyCell,
  UsageNonTokenCell,
  UsageReferenceAmount,
  UsageTokensCell,
} from './metrics-cells'
import type { UsageAnalyticsMetrics, UsageAnalyticsPeriod } from './types'

export function UsageDailyBreakdown(props: {
  period: UsageAnalyticsPeriod
  days?: UsageAnalyticsMetrics[] | null
  incompleteDays?: string[] | null
  upstream?: boolean
  onDaySelect?: (day: string) => void
}) {
  const { t } = useTranslation()
  const { period, days, incompleteDays, onDaySelect } = props
  if (!days?.length) return null
  return (
    <Collapsible className='space-y-2'>
      <CollapsibleTrigger render={<Button variant='outline' size='sm' />}>
        {t('Daily breakdown')}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <Table className='min-w-[34rem]'>
          <caption className='sr-only'>{t('Daily breakdown')}</caption>
          <TableHeader>
            <TableRow>
              <TableHead scope='col'>{t('Day')}</TableHead>
              <TableHead scope='col'>{t('Calls')}</TableHead>
              <TableHead scope='col'>{t('Total tokens')}</TableHead>
              <TableHead scope='col'>{t('Non-Token usage')}</TableHead>
              <TableHead scope='col'>
                {props.upstream ? t('Reference amount') : t('Net amount')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {period.days.map((day, index) => {
              const metrics = days[index]
              let status: string | undefined
              if (day.future) status = t('Not started')
              else if (day.unpublished) status = t('Not published')
              else if (!metrics) status = t('unrecorded')
              return (
                <TableRow key={day.date}>
                  <TableHead scope='row'>
                    {status || !onDaySelect ? (
                      day.date
                    ) : (
                      <Button
                        variant='link'
                        size='sm'
                        className='px-0'
                        onClick={() => onDaySelect(day.date)}
                      >
                        {day.date}
                      </Button>
                    )}
                    {incompleteDays?.includes(day.date) && (
                      <span
                        data-table-text='secondary'
                        className='text-muted-foreground block'
                      >
                        {t('Partial data')}
                      </span>
                    )}
                  </TableHead>
                  {status ? (
                    <TableCell colSpan={4} className='text-muted-foreground'>
                      {status}
                    </TableCell>
                  ) : (
                    <>
                      <TableCell>
                        <UsageCallsCell metrics={metrics} />
                      </TableCell>
                      <TableCell>
                        <UsageTokensCell metrics={metrics} />
                      </TableCell>
                      <TableCell>
                        <UsageNonTokenCell metrics={metrics} />
                      </TableCell>
                      <TableCell>
                        {props.upstream ? (
                          <UsageReferenceAmount metrics={metrics} />
                        ) : (
                          <UsageMoneyCell metrics={metrics} describeFees />
                        )}
                      </TableCell>
                    </>
                  )}
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </CollapsibleContent>
    </Collapsible>
  )
}
