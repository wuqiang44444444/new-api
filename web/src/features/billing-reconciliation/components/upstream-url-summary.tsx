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

import { ReportCard } from '@/components/report-card'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'

import { formatCustomerStatementQuota } from '../lib'
import type { ProviderUrlSummary, UpstreamUrlChannelRow } from '../types'
import { UpstreamDiscountValue } from './upstream-discount-value'
import { AmountCell, UsageCell } from './upstream-reconciliation-table'
import {
  UpstreamTotalMetric,
  UpstreamUsageTotalMetrics,
  UpstreamUsageValues,
} from './upstream-usage-values'

export type UpstreamUrlSort = 'reference_amount' | 'requests'

export function UpstreamUrlTotals(props: {
  totals: NonNullable<ProviderUrlSummary['totals']>
  quality: ProviderUrlSummary['data_quality']
  filtered: boolean
}) {
  const { t, i18n } = useTranslation()
  const totals = props.totals
  const unverified = props.quality?.has_estimated_original
  const original = totals.original_amount ?? totals.known_original_amount
  const reference = totals.reference_amount ?? totals.known_reference_amount
  const hasPartialAmount =
    !unverified &&
    ((totals.original_amount == null && original != null) ||
      (totals.reference_amount == null && reference != null))
  const formatAmount = (amount?: number) =>
    unverified || amount == null
      ? t('Cannot calculate')
      : formatCustomerStatementQuota(amount)
  const metrics = [
    {
      label: t('Requests'),
      value: totals.usage.requests.toLocaleString(
        toIntlLocale(i18n?.language || 'en')
      ),
    },
    {
      label: t('List price'),
      value: formatAmount(original),
    },
    {
      label: t('Discounted reference amount'),
      value: formatAmount(reference),
    },
  ]
  return (
    <ReportCard className='py-1.5'>
      <section className='flex flex-wrap items-baseline gap-x-3 gap-y-1'>
        <h2 className='text-sm font-medium'>
          {props.filtered ? t('Filtered total') : t('Total')}
        </h2>
        <dl className='flex min-w-[min(100%,16rem)] flex-1 flex-wrap items-baseline gap-x-4 gap-y-1 border-s ps-3'>
          {metrics.map((metric) => (
            <UpstreamTotalMetric key={metric.label} {...metric} />
          ))}
          <UpstreamUsageTotalMetrics
            usage={totals.usage}
            quality={props.quality}
          />
        </dl>
        {hasPartialAmount && (
          <p className='text-muted-foreground basis-full text-xs'>
            {t('Amounts reflect records priced to date.')}
          </p>
        )}
        {Boolean(totals.discount_pending_channels) && (
          <p className='text-muted-foreground basis-full text-xs'>
            {t(
              'Reference amount pending discount confirmation for {{count}} channels',
              {
                count: totals.discount_pending_channels,
              }
            )}
          </p>
        )}
      </section>
    </ReportCard>
  )
}

export function UpstreamUrlSummaryTable(props: {
  rows: UpstreamUrlChannelRow[]
  sort: UpstreamUrlSort
  onSort: (sort: UpstreamUrlSort) => void
  onSelect: (row: UpstreamUrlChannelRow) => void
  onEditDiscount: (channelId: number) => void
}) {
  const { t } = useTranslation()
  return (
    <Table withContainer={false}>
      <TableHeader className='sticky top-0 z-10 bg-(--table-header)'>
        <TableRow>
          <TableHead scope='col'>{t('Upstream URL')}</TableHead>
          <TableHead scope='col'>{t('Channel')}</TableHead>
          <TableHead
            scope='col'
            aria-sort={props.sort === 'requests' ? 'descending' : undefined}
            className='text-right'
          >
            <Button
              variant='ghost'
              size='xs'
              onClick={() => props.onSort('requests')}
            >
              {t('Requests')}
              {props.sort === 'requests' && <span aria-hidden>↓</span>}
            </Button>
          </TableHead>
          <TableHead scope='col'>{t('Usage')}</TableHead>
          <TableHead
            scope='col'
            className='text-right'
            title={t('Original amount (local official price)')}
          >
            {t('List price')}
          </TableHead>
          <TableHead scope='col'>{t('Channel discount')}</TableHead>
          <TableHead
            scope='col'
            aria-sort={
              props.sort === 'reference_amount' ? 'descending' : undefined
            }
            className='text-right'
          >
            <Button
              variant='ghost'
              size='xs'
              onClick={() => props.onSort('reference_amount')}
            >
              {t('Discounted reference amount')}
              {props.sort === 'reference_amount' && <span aria-hidden>↓</span>}
            </Button>
          </TableHead>
          <TableHead scope='col' className='text-right'>
            {t('Actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.rows.map((row) => (
          <TableRow key={JSON.stringify([row.url_key, row.channel_id])}>
            <TableCell className='min-w-36 wrap-anywhere whitespace-normal'>
              {row.base_url || row.url_name}
            </TableCell>
            <TableCell className='min-w-32 wrap-anywhere whitespace-normal'>
              {row.channel_id > 0
                ? row.channel_name || t('Channel')
                : t('Not recorded')}
              {row.channel_id > 0 && (
                <span
                  className='text-muted-foreground block'
                  data-table-text='secondary'
                >
                  #{row.channel_id}
                  {row.upstream_name &&
                    row.upstream_name !== row.channel_name &&
                    ` · ${row.upstream_name}`}
                  {row.deleted && ` · ${t('Deleted channel')}`}
                </span>
              )}
            </TableCell>
            <UsageCell value={row.usage.requests} />
            <TableCell>
              <UpstreamUsageValues
                usage={row.usage}
                quality={row.data_quality}
              />
            </TableCell>
            <AmountCell
              value={row.original_amount}
              knownSubtotal={row.known_original_amount}
              unverified={row.data_quality?.has_estimated_original}
            />
            <TableCell>
              <UpstreamDiscountValue discount={row.discount} />
            </TableCell>
            <AmountCell
              unverified={row.data_quality?.has_estimated_original}
              value={row.reference_amount}
              knownSubtotal={row.known_reference_amount}
            />
            <TableCell className='text-right'>
              {row.channel_id > 0 && (
                <>
                  <Button
                    variant='link'
                    size='xs'
                    onClick={() => props.onEditDiscount(row.channel_id)}
                  >
                    {t('Set discount')}
                  </Button>
                  <Button
                    variant='link'
                    size='xs'
                    onClick={() => props.onSelect(row)}
                  >
                    {t('View details')}
                  </Button>
                </>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
