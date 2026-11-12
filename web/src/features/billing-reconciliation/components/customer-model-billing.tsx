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
import type { ReactNode, Ref } from 'react'
import { useTranslation } from 'react-i18next'

import { ReportCard } from '@/components/report-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardContent, CardDescription, CardHeader } from '@/components/ui/card'
import {
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { CustomerBillingTable } from './customer-billing-table'

export function CustomerModelBilling(props: {
  id: string
  headingRef: Ref<HTMLHeadingElement>
  filterLabel?: string
  onClearFilter: () => void
  showPriceColumns: boolean
  original: string
  savings: string
  net: string
  requests: string
  pagination: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  const headingId = `${props.id}-heading`

  return (
    <section id={props.id} aria-labelledby={headingId}>
      <ReportCard className='gap-0 p-0'>
        <CardHeader className='gap-2 px-3 py-2'>
          <div className='flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1'>
            <h2
              id={headingId}
              ref={props.headingRef}
              tabIndex={-1}
              className='scroll-mt-4 rounded-sm text-sm font-medium'
            >
              {t('Model billing')}
            </h2>
            {props.filterLabel && (
              <>
                <Badge
                  variant='secondary'
                  className='h-auto max-w-full wrap-anywhere whitespace-normal'
                >
                  {t('API Key')}: {props.filterLabel}
                </Badge>
                <Button size='sm' variant='ghost' onClick={props.onClearFilter}>
                  {t('Clear filters')}
                </Button>
              </>
            )}
          </div>
          {!props.showPriceColumns && (
            <CardDescription>
              {t('List price and savings were not recorded for these models.')}
            </CardDescription>
          )}
        </CardHeader>
        <CardContent className='flex min-h-0 flex-col px-0'>
          <div className='max-h-[45vh] min-h-0 overflow-auto'>
            <CustomerBillingTable
              withContainer={false}
              aria-labelledby={headingId}
            >
              <TableHeader className='sticky top-0 z-10 bg-(--table-header)'>
                <TableRow>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead>{t('API Key')}</TableHead>
                  <TableHead className='text-right'>{t('Requests')}</TableHead>
                  <TableHead className='text-right'>
                    {t('Contract discount')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('Group discount/ratio')}
                  </TableHead>
                  {props.showPriceColumns && (
                    <>
                      <TableHead className='text-right'>
                        {t('List price')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Savings')}
                      </TableHead>
                    </>
                  )}
                  <TableHead className='text-right'>
                    {t('Net settled amount')}
                  </TableHead>
                  <TableHead data-column='actions' className='w-px text-right'>
                    {t('Actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>{props.children}</TableBody>
              <TableFooter>
                <TableRow>
                  <TableHead colSpan={2} scope='row'>
                    {t('Total')}
                  </TableHead>
                  <TableCell className='text-right'>{props.requests}</TableCell>
                  <TableCell colSpan={2} />
                  {props.showPriceColumns && (
                    <>
                      <TableCell className='text-right'>
                        {props.original}
                      </TableCell>
                      <TableCell className='text-right'>
                        {props.savings}
                      </TableCell>
                    </>
                  )}
                  <TableCell className='text-right font-semibold!'>
                    {props.net}
                  </TableCell>
                  <TableCell />
                </TableRow>
              </TableFooter>
            </CustomerBillingTable>
          </div>
          <div className='px-3 py-2'>{props.pagination}</div>
        </CardContent>
      </ReportCard>
    </section>
  )
}
