import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import type { UsageAnalyticsPeriod, UsagePeriodKind } from './types'
import { formatPeriodRange, shiftDate, shiftMonth } from './utils'

interface UsagePeriodPickerProps {
  period: UsagePeriodKind
  date: string
  resolved: UsageAnalyticsPeriod | undefined
  hideDailyRange?: boolean
  onPeriodChange: (period: 'day' | 'week') => void
  onMonthSelect?: () => void
  onDateChange: (date: string) => void
}

export function UsagePeriodPicker(props: UsagePeriodPickerProps) {
  const { t } = useTranslation()
  const monthly = props.period === 'month'
  const step = props.period === 'week' ? 7 : 1
  const previousDate = monthly
    ? shiftMonth(props.date, -1)
    : shiftDate(props.date, -step)
  const nextDate = monthly
    ? shiftMonth(props.date, 1)
    : shiftDate(props.date, step)
  let range = props.date
  if (props.resolved) {
    range = formatPeriodRange(props.resolved)
  } else if (props.period === 'week') {
    const start = new Date(`${props.date}T00:00:00Z`)
    const monday = shiftDate(props.date, -((start.getUTCDay() + 6) % 7))
    range = `${monday} ~ ${shiftDate(monday, 6)}`
  } else if (monthly) {
    range = `${shiftMonth(props.date, 0)} ~ ${shiftDate(nextDate, -1)}`
  }

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <div className='flex overflow-hidden rounded-md border'>
        <button
          type='button'
          aria-pressed={props.period === 'day'}
          className={`px-3 py-1.5 text-sm ${props.period === 'day' ? 'bg-primary text-primary-foreground' : ''}`}
          onClick={() => props.onPeriodChange('day')}
        >
          {t('Daily')}
        </button>
        <button
          type='button'
          aria-pressed={props.period === 'week'}
          className={`px-3 py-1.5 text-sm ${props.period === 'week' ? 'bg-primary text-primary-foreground' : ''}`}
          onClick={() => props.onPeriodChange('week')}
        >
          {t('Weekly')}
        </button>
        {props.onMonthSelect && (
          <button
            type='button'
            aria-pressed={monthly}
            className={`px-3 py-1.5 text-sm ${monthly ? 'bg-primary text-primary-foreground' : ''}`}
            onClick={props.onMonthSelect}
          >
            {t('Monthly')}
          </button>
        )}
      </div>
      <Button
        variant='outline'
        size='sm'
        onClick={() => props.onDateChange(previousDate)}
      >
        {t('Previous period')}
      </Button>
      <Input
        aria-label={monthly ? t('Monthly') : t('Day')}
        type={monthly ? 'month' : 'date'}
        value={monthly ? props.date.slice(0, 7) : props.date}
        className='w-40'
        onChange={(event) => {
          if (event.target.value) {
            props.onDateChange(
              monthly ? `${event.target.value}-01` : event.target.value
            )
          }
        }}
      />
      <Button
        variant='outline'
        size='sm'
        onClick={() => props.onDateChange(nextDate)}
      >
        {t('Next period')}
      </Button>
      {range && !(props.hideDailyRange && props.period === 'day') && (
        <span className='text-muted-foreground text-sm'>{range}</span>
      )}
    </div>
  )
}
