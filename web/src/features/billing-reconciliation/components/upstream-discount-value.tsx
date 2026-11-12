/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

import type { ProviderDiscount } from '../types'
import {
  formatShanghaiTimestamp,
  upstreamDiscountLabel,
  upstreamDiscountSourceLabel,
} from '../upstream-reconciliation-utils'

export function UpstreamDiscountRanges(props: {
  start: number
  value: string
  changes: NonNullable<ProviderDiscount['changes']>
}) {
  const month = formatShanghaiTimestamp(props.start).slice(0, 7)
  const [year, monthNumber] = month.split('-').map(Number)
  const end = Date.UTC(year, monthNumber, 1) / 1000 - 8 * 3600
  const points = [
    { effective_from: props.start, value: props.value },
    ...props.changes,
  ]
  return (
    <ul className='space-y-1 text-sm tabular-nums'>
      {points.map((point, index) => (
        <li
          key={point.effective_from}
          className='flex flex-wrap justify-between gap-x-4'
        >
          <span>
            {formatShanghaiTimestamp(point.effective_from).slice(0, 10)} –{' '}
            {formatShanghaiTimestamp(
              (points[index + 1]?.effective_from ?? end) - 1
            ).slice(0, 10)}
          </span>
          <span>×{point.value}</span>
        </li>
      ))}
    </ul>
  )
}

export function UpstreamDiscountValue({
  discount,
}: {
  discount: ProviderDiscount | null
}) {
  const { t } = useTranslation()
  if (discount?.changes?.length && discount.period_start) {
    return (
      <Popover>
        <PopoverTrigger render={<Button variant='outline' size='xs' />}>
          {t('Discounts by date')}
        </PopoverTrigger>
        <PopoverContent className='w-auto max-w-[calc(100vw-2rem)]'>
          <UpstreamDiscountRanges
            start={discount.period_start}
            value={discount.value}
            changes={discount.changes}
          />
        </PopoverContent>
      </Popover>
    )
  }
  return (
    <>
      {upstreamDiscountLabel(discount, t)}
      <span className='text-muted-foreground block' data-table-text='secondary'>
        {upstreamDiscountSourceLabel(discount, t)}
      </span>
    </>
  )
}
