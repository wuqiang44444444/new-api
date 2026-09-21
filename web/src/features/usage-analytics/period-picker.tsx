import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import type { UsageAnalyticsPeriod } from './types'
import { formatPeriodRange, shiftDate } from './utils'

interface UsagePeriodPickerProps {
  period: 'day' | 'week'
  date: string
  resolved: UsageAnalyticsPeriod | undefined
  onPeriodChange: (period: 'day' | 'week') => void
  onDateChange: (date: string) => void
}

export function UsagePeriodPicker(props: UsagePeriodPickerProps) {
  const { t } = useTranslation()
  const start = new Date(`${props.date}T00:00:00Z`)
  const monday = shiftDate(props.date, -((start.getUTCDay() + 6) % 7))
  let range = props.date
  if (props.resolved) {
    range = formatPeriodRange(props.resolved)
  } else if (props.period === 'week') {
    range = `${monday} ~ ${shiftDate(monday, 6)}`
  }

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <div className='flex overflow-hidden rounded-md border'>
        <button
          type='button'
          className={`px-3 py-1.5 text-sm ${props.period === 'day' ? 'bg-primary text-primary-foreground' : ''}`}
          onClick={() => props.onPeriodChange('day')}
        >
          {t('Daily')}
        </button>
        <button
          type='button'
          className={`px-3 py-1.5 text-sm ${props.period === 'week' ? 'bg-primary text-primary-foreground' : ''}`}
          onClick={() => props.onPeriodChange('week')}
        >
          {t('Weekly')}
        </button>
      </div>
      <Button
        variant='outline'
        size='sm'
        onClick={() =>
          props.onDateChange(
            shiftDate(props.date, props.period === 'day' ? -1 : -7)
          )
        }
      >
        {t('Previous period')}
      </Button>
      <Input
        aria-label={t('Day')}
        type='date'
        value={props.date}
        className='w-40'
        onChange={(event) => {
          if (event.target.value) {
            props.onDateChange(event.target.value)
          }
        }}
      />
      <Button
        variant='outline'
        size='sm'
        onClick={() =>
          props.onDateChange(
            shiftDate(props.date, props.period === 'day' ? 1 : 7)
          )
        }
      >
        {t('Next period')}
      </Button>
      {range && <span className='text-muted-foreground text-sm'>{range}</span>}
    </div>
  )
}
