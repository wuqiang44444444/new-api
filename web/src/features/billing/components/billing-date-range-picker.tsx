/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { CalendarDays } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import dayjs from '@/lib/dayjs'

type BillingDateRangePickerProps = {
  startTimestamp: number
  endTimestamp: number
  onApply: (startTime: number, endTime: number) => void
}

function toInputValue(timestamp: number) {
  return dayjs(timestamp).format('YYYY-MM-DDTHH:mm')
}

export function BillingDateRangePicker(props: BillingDateRangePickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [draftStart, setDraftStart] = useState(
    toInputValue(props.startTimestamp)
  )
  const [draftEnd, setDraftEnd] = useState(toInputValue(props.endTimestamp))

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setDraftStart(toInputValue(props.startTimestamp))
      setDraftEnd(toInputValue(props.endTimestamp))
    }
    setOpen(nextOpen)
  }

  const startTime = new Date(draftStart).getTime()
  const endTime = new Date(draftEnd).getTime()
  const invalidRange =
    !Number.isFinite(startTime) ||
    !Number.isFinite(endTime) ||
    endTime < startTime ||
    endTime - startTime > 31 * 24 * 60 * 60 * 1000

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='outline'
            className='w-full justify-start font-normal tabular-nums'
          />
        }
      >
        <CalendarDays />
        <span className='truncate'>
          {dayjs(props.startTimestamp).format('YYYY-MM-DD HH:mm')} ~{' '}
          {dayjs(props.endTimestamp).format('YYYY-MM-DD HH:mm')}
        </span>
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='w-[min(520px,calc(100vw-2rem))] p-3'
      >
        <div className='space-y-3'>
          <div className='grid gap-3 sm:grid-cols-2'>
            <label className='space-y-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('Start Time')}
              </span>
              <Input
                type='datetime-local'
                value={draftStart}
                onChange={(event) => setDraftStart(event.target.value)}
              />
            </label>
            <label className='space-y-1.5'>
              <span className='text-muted-foreground text-xs'>
                {t('End Time')}
              </span>
              <Input
                type='datetime-local'
                value={draftEnd}
                onChange={(event) => setDraftEnd(event.target.value)}
              />
            </label>
          </div>
          <div className='flex items-center justify-between gap-3'>
            <p className='text-muted-foreground text-xs'>
              {t('The billing period can be up to 31 days.')}
            </p>
            <Button
              size='sm'
              disabled={invalidRange}
              onClick={() => {
                props.onApply(startTime, endTime)
                setOpen(false)
              }}
            >
              {t('Apply')}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
