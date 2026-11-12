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
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  ArrowUp01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { ReportCard } from '@/components/report-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardContent, CardFooter } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useDebounce } from '@/hooks/use-debounce'
import { formatTimestampToDate } from '@/lib/format'

import { getAdminCustomerStatements } from '../api'
import { createCustomerStatementsExport } from '../export-api'
import { formatInteger, formatCustomerStatementQuota } from '../lib'
import type {
  AdminBillingSearch,
  CustomerStatementListItem,
  CustomerStatementListQuality,
  CustomerStatementListSortBy,
} from '../types'
import { BillingExportAction } from './billing-export-action'
import { BillingPagination } from './billing-pagination'
import { BillingQualityNotice } from './billing-quality-notice'
import { BillingReportStatus } from './billing-report-status'

type CustomerStatementsListProps = {
  period: { start_timestamp: number; end_timestamp: number }
  search: AdminBillingSearch
  onSearchChange: (
    patch: Partial<AdminBillingSearch>,
    replace?: boolean
  ) => void
  onSelectUser: (userId: number, version?: string) => void
}

export function CustomerStatementsListView(props: CustomerStatementsListProps) {
  const { t } = useTranslation()
  const [showFunds, setShowFunds] = useState(false)
  const search = props.search.customerSearch ?? ''
  const quality = props.search.customerQuality ?? 'all'
  const sortBy = props.search.customerSortBy ?? 'net_quota'
  const sortOrder = props.search.customerSortOrder ?? 'desc'
  const page = props.search.customerPage ?? 1
  const pageSize = props.search.customerPageSize ?? 20
  const debouncedSearch = useDebounce(search, 300)
  const query = useQuery({
    queryKey: [
      'billing-customer-statements',
      props.period.start_timestamp,
      props.period.end_timestamp,
      debouncedSearch,
      quality,
      sortBy,
      sortOrder,
      page,
      pageSize,
    ],
    queryFn: async () => {
      const response = await getAdminCustomerStatements({
        ...props.period,
        search: debouncedSearch || undefined,
        quality_status: quality === 'all' ? undefined : quality,
        sort_by: sortBy,
        sort_order: sortOrder,
        page,
        page_size: pageSize,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load customer statements.')
        )
      }
      return response.data
    },
    placeholderData: (previousData) => previousData,
    staleTime: 30_000,
    retry: false,
  })

  if (query.isError) {
    return (
      <div className='min-h-0 flex-1 overflow-y-auto'>
        <ErrorState
          title={t('Unable to load customer statements')}
          description={
            query.error instanceof Error
              ? query.error.message
              : t('Please try again later.')
          }
          onRetry={() => query.refetch()}
        />
      </div>
    )
  }

  if (query.isPending || !query.data) {
    return (
      <div className='min-h-0 flex-1 overflow-y-auto'>
        <CustomerStatementsListSkeleton />
      </div>
    )
  }

  const reportVersion = query.data.billing_report?.version
  const result = query.data.result
  const items = result.items ?? []
  const firstItem =
    result.total === 0 ? 0 : (result.page - 1) * result.page_size + 1
  const lastItem = Math.min(result.page * result.page_size, result.total)
  const setSort = (nextSortBy: CustomerStatementListSortBy) => {
    const nextOrder =
      sortBy === nextSortBy && sortOrder === 'desc' ? 'asc' : 'desc'
    props.onSearchChange({
      customerSortBy: nextSortBy,
      customerSortOrder: nextOrder,
      customerPage: 1,
    })
  }

  return (
    <div className='grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)] grid-rows-[minmax(0,auto)_minmax(min(8rem,45%),1fr)_auto] gap-3'>
      <div className='flex min-h-0 flex-col gap-3 overflow-y-auto overscroll-contain'>
        <div className='flex shrink-0 flex-wrap items-center justify-between gap-2'>
          <div className='flex min-w-0 flex-wrap items-center gap-2 max-sm:w-full'>
            <Input
              className='w-full sm:w-64 lg:w-80'
              aria-label={t('Customer search')}
              value={search}
              placeholder={t('Search by customer name, username, or ID')}
              onChange={(event) =>
                props.onSearchChange(
                  {
                    customerSearch: event.target.value || undefined,
                    customerPage: 1,
                  },
                  true
                )
              }
            />
            <Select
              items={[
                { value: 'all', label: t('All data quality') },
                { value: 'complete', label: t('Complete') },
                { value: 'partial', label: t('Partial data') },
              ]}
              value={quality}
              onValueChange={(value) =>
                value != null &&
                props.onSearchChange({
                  customerQuality: value as CustomerStatementListQuality,
                  customerPage: 1,
                })
              }
            >
              <SelectTrigger aria-label={t('Data quality')}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent align='start'>
                <SelectGroup>
                  <SelectItem value='all'>{t('All data quality')}</SelectItem>
                  <SelectItem value='complete'>{t('Complete')}</SelectItem>
                  <SelectItem value='partial'>{t('Partial data')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <Button
              size='sm'
              variant={showFunds ? 'secondary' : 'outline'}
              aria-pressed={showFunds}
              onClick={() => setShowFunds(!showFunds)}
            >
              {t('Fund movements')}
            </Button>
            <BillingExportAction
              startTimestamp={props.period.start_timestamp}
              statement={t('Customer statements')}
              scope={[t('All customers')]}
              description={t(
                'The CSV includes one statement summary per customer for the selected month, regardless of list filters.'
              )}
              available={!!reportVersion && !query.isPlaceholderData}
              submit={() =>
                createCustomerStatementsExport({
                  start_timestamp: props.period.start_timestamp,
                  end_timestamp: props.period.end_timestamp + 1,
                  report_version: reportVersion ?? '',
                })
              }
            />
          </div>
          <div className='flex min-w-0 flex-wrap items-center gap-2'>
            <BillingReportStatus report={query.data.billing_report} />
            <Popover>
              <PopoverTrigger render={<Button variant='ghost' size='sm' />}>
                {t('Billing notes')}
              </PopoverTrigger>
              <PopoverContent align='end' className='text-xs'>
                <PopoverTitle>{t('Billing notes')}</PopoverTitle>
                <p>
                  {t('Database generated at {{time}} · Asia/Shanghai', {
                    time: formatTimestampToDate(query.data.generated_at),
                  })}
                </p>
                <p>
                  {t('List price minus savings equals net settled amount.')}
                </p>
                <p>
                  {t('Total charges − total returns = net settled amount.')}
                </p>
                <p>
                  {t(
                    'Total charges include precharges. Total returns include released precharges and refunds.'
                  )}
                </p>
              </PopoverContent>
            </Popover>
          </div>
        </div>
        <ReportCard className='shrink-0 flex-row flex-wrap items-baseline gap-x-4 gap-y-1 py-1.5'>
          <ListMetric
            label={t('Customers')}
            value={formatInteger(result.summary.customer_count)}
          />
          <ListMetric
            label={t('Requests')}
            value={formatInteger(result.summary.usage.requests)}
          />
          <ListMetric
            label={t('List price')}
            value={formatRecordedStatementMoney(
              result.recorded_original_amounts,
              result.summary.original_quota,
              result.summary.money_usd?.original,
              t('Not recorded')
            )}
          />
          <ListMetric
            label={t('Savings')}
            value={formatRecordedStatementMoney(
              result.recorded_original_amounts,
              result.summary.discount_quota,
              result.summary.money_usd?.discount,
              t('Not recorded')
            )}
            valueClassName='text-success'
          />
          <ListMetric
            label={t('Net settled amount')}
            value={
              result.summary.money_usd
                ? formatStatementUSD(result.summary.money_usd.net)
                : formatCustomerStatementQuota(result.summary.usage.net_quota)
            }
          />
        </ReportCard>
      </div>
      <ReportCard
        aria-label={t('Customer statements')}
        className='row-span-2 max-h-full min-h-0 gap-0 self-start p-0'
      >
        <CardContent className='min-h-0 overflow-auto overscroll-contain px-0'>
          {items.length === 0 ? (
            <Empty className='border-0 px-3 py-4'>
              <EmptyHeader>
                <EmptyTitle>{t('No customer statements')}</EmptyTitle>
                <EmptyDescription>
                  {t('No billing facts match the current filters.')}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Table withContainer={false}>
              <TableHeader className='sticky top-0 z-10 bg-(--table-header)'>
                <TableRow>
                  <SortableHead
                    label={t('Customer')}
                    sortKey='username'
                    activeSort={sortBy}
                    sortOrder={sortOrder}
                    onSort={setSort}
                  />
                  <SortableHead
                    label={t('Requests')}
                    sortKey='requests'
                    activeSort={sortBy}
                    sortOrder={sortOrder}
                    onSort={setSort}
                    align='right'
                  />
                  <SortableHead
                    label={t('List price')}
                    sortKey='original_quota'
                    activeSort={sortBy}
                    sortOrder={sortOrder}
                    onSort={setSort}
                    align='right'
                  />
                  <TableHead className='text-right'>{t('Discount')}</TableHead>
                  <SortableHead
                    label={t('Net settled amount')}
                    sortKey='net_quota'
                    activeSort={sortBy}
                    sortOrder={sortOrder}
                    onSort={setSort}
                    align='right'
                  />
                  {showFunds && (
                    <>
                      <TableHead className='text-muted-foreground border-l text-right'>
                        {t('Total charges')}
                      </TableHead>
                      <TableHead className='text-muted-foreground text-right'>
                        {t('Total returns')}
                      </TableHead>
                    </>
                  )}
                  <TableHead>{t('Data quality')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <CustomerStatementListRow
                    key={item.user_id}
                    item={item}
                    showFunds={showFunds}
                    recordedOriginalAmounts={result.recorded_original_amounts}
                    onSelect={() => {
                      const version = query.data?.billing_report?.version
                      if (version) props.onSelectUser(item.user_id, version)
                      else props.onSelectUser(item.user_id)
                    }}
                  />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
        {result.total > 0 && (
          <CardFooter className='block shrink-0 bg-transparent px-3 py-2'>
            <div className='grid min-w-0 grid-cols-[minmax(0,1fr)] items-center gap-2 sm:grid-cols-[auto_minmax(0,1fr)] sm:gap-3'>
              <div className='text-muted-foreground text-sm'>
                {t('Showing {{start}}–{{end}} of {{total}}', {
                  start: firstItem,
                  end: lastItem,
                  total: result.total,
                })}
              </div>
              <BillingPagination
                page={result.page}
                pageSize={result.page_size}
                pageSizes={[10, 20, 50, 100]}
                total={result.total}
                disabled={query.isFetching}
                label={t('Customer billing')}
                onChange={(nextPage, nextSize) => {
                  if (nextSize === result.page_size) {
                    props.onSearchChange({ customerPage: nextPage }, true)
                  } else if (
                    nextSize === 10 ||
                    nextSize === 20 ||
                    nextSize === 50 ||
                    nextSize === 100
                  ) {
                    props.onSearchChange({
                      customerPageSize: nextSize,
                      customerPage: 1,
                    })
                  }
                }}
              />
            </div>
          </CardFooter>
        )}
      </ReportCard>
    </div>
  )
}

function CustomerStatementListRow(props: {
  item: CustomerStatementListItem
  recordedOriginalAmounts?: boolean
  showFunds: boolean
  onSelect: () => void
}) {
  const { t } = useTranslation()
  const item = props.item
  return (
    <TableRow>
      <TableCell>
        <div className='font-medium'>
          {customerListItemLabel(item)}{' '}
          {item.billing_version && (
            <Badge variant='secondary'>
              {t('Confirmed')} v{item.billing_version.version_number}
            </Badge>
          )}
        </div>
        <div className='text-muted-foreground text-xs'>
          {item.username} · #{item.user_id}
          {item.deleted ? ` · ${t('Deleted')}` : ''}
        </div>
      </TableCell>
      <TableCell className='text-right'>
        {formatInteger(item.usage.requests)}
      </TableCell>
      <TableCell className='text-right'>
        {formatRecordedStatementMoney(
          props.recordedOriginalAmounts,
          item.original_quota,
          item.money_usd?.original,
          t('Not recorded')
        )}
      </TableCell>
      <TableCell className='text-success text-right'>
        {formatRecordedStatementMoney(
          props.recordedOriginalAmounts,
          item.discount_quota,
          item.money_usd?.discount,
          t('Not recorded')
        )}
      </TableCell>
      <TableCell className='text-right font-semibold tabular-nums'>
        {item.money_usd
          ? formatStatementUSD(item.money_usd.net)
          : formatCustomerStatementQuota(item.usage.net_quota)}
      </TableCell>
      {props.showFunds && (
        <>
          <TableCell className='text-muted-foreground border-l text-right'>
            {item.money_usd
              ? formatStatementUSD(item.money_usd.gross)
              : formatCustomerStatementQuota(item.usage.gross_quota)}
          </TableCell>
          <TableCell className='text-muted-foreground text-right'>
            {item.money_usd
              ? formatStatementUSD(item.money_usd.refund)
              : formatCustomerStatementQuota(item.usage.refund_quota)}
          </TableCell>
        </>
      )}
      <TableCell>
        <BillingQualityNotice
          quality={item.data_quality}
          discloseReasons
          rowDetails={
            item.last_activity_at > 0 ? (
              <p>
                {t('Last activity')} ·{' '}
                {formatTimestampToDate(item.last_activity_at)}
              </p>
            ) : null
          }
          compact
        />
      </TableCell>
      <TableCell className='text-right'>
        <Button variant='link' size='xs' onClick={props.onSelect}>
          {t('View statement')}
          <HugeiconsIcon
            icon={ArrowRight01Icon}
            strokeWidth={2}
            data-icon='inline-end'
          />
        </Button>
      </TableCell>
    </TableRow>
  )
}

function SortableHead(props: {
  label: string
  sortKey: CustomerStatementListSortBy
  activeSort: CustomerStatementListSortBy
  sortOrder: 'asc' | 'desc'
  onSort: (sortKey: CustomerStatementListSortBy) => void
  align?: 'left' | 'right'
}) {
  const active = props.activeSort === props.sortKey
  const ariaSort = props.sortOrder === 'asc' ? 'ascending' : 'descending'
  return (
    <TableHead
      aria-sort={active ? ariaSort : undefined}
      className={props.align === 'right' ? 'text-right' : undefined}
    >
      <Button
        variant='ghost'
        size='xs'
        className={props.align === 'right' ? '-mr-2' : '-ml-2'}
        onClick={() => props.onSort(props.sortKey)}
      >
        {props.label}
        {active && (
          <HugeiconsIcon
            icon={props.sortOrder === 'asc' ? ArrowUp01Icon : ArrowDown01Icon}
            strokeWidth={2}
            data-icon='inline-end'
          />
        )}
      </Button>
    </TableHead>
  )
}

function ListMetric(props: {
  label: string
  value: string
  valueClassName?: string
}) {
  return (
    <div className='flex items-baseline gap-1.5'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div
        className={`text-sm font-semibold tabular-nums ${props.valueClassName ?? ''}`}
      >
        {props.value}
      </div>
    </div>
  )
}

function CustomerStatementsListSkeleton() {
  return (
    <div className='space-y-3'>
      <Skeleton className='h-9 rounded-lg' />
      <Skeleton className='h-9 rounded-lg' />
      <Skeleton className='h-40 rounded-lg' />
    </div>
  )
}

function customerListItemLabel(item: CustomerStatementListItem) {
  return item.display_name?.trim() || item.username
}

function formatRecordedStatementMoney(
  recorded: boolean | undefined,
  quota: number | undefined,
  usd: string | null | undefined,
  missing: string
) {
  if (!recorded) return missing
  return usd === undefined
    ? formatCustomerStatementQuota(quota)
    : formatStatementUSD(usd)
}

function formatStatementUSD(value: string | null) {
  if (value == null) return '—'
  return value.startsWith('-') ? `-$${value.slice(1)}` : `$${value}`
}
