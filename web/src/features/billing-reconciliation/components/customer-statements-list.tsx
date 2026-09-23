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
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination'
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
import { formatInteger, formatCustomerStatementQuota } from '../lib'
import type {
  AdminBillingSearch,
  CustomerStatementListItem,
  CustomerStatementListQuality,
  CustomerStatementListSortBy,
} from '../types'
import { BillingQualityNotice } from './billing-quality-notice'

type CustomerStatementsListProps = {
  period: { start_timestamp: number; end_timestamp: number }
  search: AdminBillingSearch
  onSearchChange: (
    patch: Partial<AdminBillingSearch>,
    replace?: boolean
  ) => void
  onSelectUser: (userId: number) => void
}

export function CustomerStatementsListView(props: CustomerStatementsListProps) {
  const { t } = useTranslation()
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

  const result = query.data.result
  const items = result.items ?? []
  const totalPages = Math.max(1, Math.ceil(result.total / result.page_size))
  const firstItem =
    result.total === 0 ? 0 : (result.page - 1) * result.page_size + 1
  const lastItem = Math.min(result.page * result.page_size, result.total)
  const visiblePages = customerStatementPageItems(result.page, totalPages)
  const setPage = (nextPage: number) => {
    props.onSearchChange(
      { customerPage: Math.min(Math.max(nextPage, 1), totalPages) },
      true
    )
  }
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
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <Card size='sm' className='shrink-0'>
        <CardContent>
          <FieldGroup className='grid gap-3 md:grid-cols-[minmax(16rem,1fr)_14rem_10rem]'>
            <Field>
              <FieldLabel htmlFor='customer-statement-search'>
                {t('Customer search')}
              </FieldLabel>
              <Input
                id='customer-statement-search'
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
            </Field>
            <Field>
              <FieldLabel>{t('Data quality')}</FieldLabel>
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
                <SelectTrigger className='w-full'>
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
            </Field>
            <Field>
              <FieldLabel>{t('Per page')}</FieldLabel>
              <Select
                items={[10, 20, 50, 100].map((value) => ({
                  value: String(value),
                  label: String(value),
                }))}
                value={String(pageSize)}
                onValueChange={(value) =>
                  value != null &&
                  props.onSearchChange({
                    customerPageSize: Number(value) as 10 | 20 | 50 | 100,
                    customerPage: 1,
                  })
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent align='start'>
                  <SelectGroup>
                    {[10, 20, 50, 100].map((value) => (
                      <SelectItem key={value} value={String(value)}>
                        {value}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          </FieldGroup>
        </CardContent>
      </Card>

      <Card size='sm' className='shrink-0'>
        <CardContent className='grid gap-4 sm:grid-cols-2 xl:grid-cols-5 xl:divide-x xl:[&>div]:px-6 xl:[&>div:first-child]:pl-0 xl:[&>div:last-child]:pr-0'>
          <ListMetric
            label={t('Customers')}
            value={formatInteger(result.summary.customer_count)}
          />
          <ListMetric
            label={t('Requests')}
            value={formatInteger(result.summary.usage.requests)}
          />
          <ListMetric
            label={t('Estimated list price')}
            value={
              result.summary.money_usd
                ? formatStatementUSD(result.summary.money_usd.original)
                : formatCustomerStatementQuota(result.summary.original_quota)
            }
          />
          <ListMetric
            label={t('Estimated savings')}
            value={
              result.summary.money_usd
                ? formatStatementUSD(result.summary.money_usd.discount)
                : formatCustomerStatementQuota(result.summary.discount_quota)
            }
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
        </CardContent>
      </Card>

      <Card className='flex min-h-0 flex-1 flex-col'>
        <CardHeader className='shrink-0'>
          <CardTitle>{t('Customer statements')}</CardTitle>
          <CardDescription>
            {t('Database generated at {{time}} · Asia/Shanghai', {
              time: formatTimestampToDate(query.data.generated_at),
            })}
          </CardDescription>
          <CardDescription>
            {t(
              'Estimated list price minus estimated savings equals net settled amount.'
            )}{' '}
            {t('Total charges − total returns = net settled amount.')}
          </CardDescription>
          <CardDescription>
            {t(
              'Total charges include precharges. Total returns include released precharges and refunds.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex min-h-0 flex-1 flex-col px-0'>
          {items.length === 0 ? (
            <Empty className='min-h-64 border-0'>
              <EmptyHeader>
                <EmptyTitle>{t('No customer statements')}</EmptyTitle>
                <EmptyDescription>
                  {t('No billing facts match the current filters.')}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <div className='min-h-0 flex-1 overflow-auto'>
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
                      label={t('Estimated list price')}
                      sortKey='original_quota'
                      activeSort={sortBy}
                      sortOrder={sortOrder}
                      onSort={setSort}
                      align='right'
                    />
                    <TableHead className='text-right'>
                      {t('Discount')}
                    </TableHead>
                    <SortableHead
                      label={t('Net settled amount')}
                      sortKey='net_quota'
                      activeSort={sortBy}
                      sortOrder={sortOrder}
                      onSort={setSort}
                      align='right'
                    />
                    <TableHead className='text-muted-foreground border-l text-right'>
                      {t('Total charges')}
                    </TableHead>
                    <TableHead className='text-muted-foreground text-right'>
                      {t('Total returns')}
                    </TableHead>
                    <TableHead>{t('Data quality')}</TableHead>
                    <TableHead>{t('Last activity')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((item) => (
                    <CustomerStatementListRow
                      key={item.user_id}
                      item={item}
                      onSelect={() => props.onSelectUser(item.user_id)}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
        {result.total > 0 && (
          <CardFooter className='block shrink-0 bg-transparent'>
            <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
              <div className='text-muted-foreground text-sm'>
                {t('Showing {{start}}–{{end}} of {{total}}', {
                  start: firstItem,
                  end: lastItem,
                  total: result.total,
                })}
              </div>
              <Pagination className='mx-0 w-auto justify-end'>
                <PaginationContent>
                  <PaginationItem>
                    <PaginationPrevious
                      href='#'
                      text={t('Previous')}
                      aria-label={t('Go to previous page')}
                      aria-disabled={result.page <= 1}
                      className={
                        result.page <= 1
                          ? 'pointer-events-none opacity-50'
                          : undefined
                      }
                      onClick={(event) => {
                        event.preventDefault()
                        if (result.page > 1) setPage(result.page - 1)
                      }}
                    />
                  </PaginationItem>
                  {visiblePages.map((visiblePage) =>
                    typeof visiblePage !== 'number' ? (
                      <PaginationItem key={visiblePage}>
                        <PaginationEllipsis />
                      </PaginationItem>
                    ) : (
                      <PaginationItem key={visiblePage}>
                        <PaginationLink
                          href='#'
                          isActive={visiblePage === result.page}
                          aria-label={t('Go to page {{page}}', {
                            page: visiblePage,
                          })}
                          onClick={(event) => {
                            event.preventDefault()
                            setPage(visiblePage)
                          }}
                        >
                          {visiblePage}
                        </PaginationLink>
                      </PaginationItem>
                    )
                  )}
                  <PaginationItem>
                    <PaginationNext
                      href='#'
                      text={t('Next')}
                      aria-label={t('Go to next page')}
                      aria-disabled={result.page >= totalPages}
                      className={
                        result.page >= totalPages
                          ? 'pointer-events-none opacity-50'
                          : undefined
                      }
                      onClick={(event) => {
                        event.preventDefault()
                        if (result.page < totalPages) setPage(result.page + 1)
                      }}
                    />
                  </PaginationItem>
                </PaginationContent>
              </Pagination>
            </div>
          </CardFooter>
        )}
      </Card>
    </div>
  )
}

function CustomerStatementListRow(props: {
  item: CustomerStatementListItem
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
        {item.money_usd
          ? formatStatementUSD(item.money_usd.original)
          : formatCustomerStatementQuota(item.original_quota)}
      </TableCell>
      <TableCell className='text-success text-right'>
        {item.money_usd
          ? formatStatementUSD(item.money_usd.discount)
          : formatCustomerStatementQuota(item.discount_quota)}
      </TableCell>
      <TableCell className='text-right font-medium'>
        {item.money_usd
          ? formatStatementUSD(item.money_usd.net)
          : formatCustomerStatementQuota(item.usage.net_quota)}
      </TableCell>
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
      <TableCell>
        <BillingQualityNotice
          quality={item.data_quality}
          estimateAvailable={
            item.original_quota != null && item.discount_quota != null
          }
          compact
        />
      </TableCell>
      <TableCell>{formatTimestampToDate(item.last_activity_at)}</TableCell>
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
  return (
    <TableHead className={props.align === 'right' ? 'text-right' : undefined}>
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
    <div className='space-y-1 py-1'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div
        className={`text-xl font-medium tabular-nums ${props.valueClassName ?? ''}`}
      >
        {props.value}
      </div>
    </div>
  )
}

function CustomerStatementsListSkeleton() {
  return (
    <div className='space-y-3'>
      <Skeleton className='h-24 rounded-xl' />
      <Skeleton className='h-24 rounded-xl' />
      <Skeleton className='h-96 rounded-xl' />
    </div>
  )
}

function customerListItemLabel(item: CustomerStatementListItem) {
  return item.display_name?.trim() || item.username
}

function customerStatementPageItems(
  page: number,
  totalPages: number
): Array<number | 'ellipsis-leading' | 'ellipsis-trailing'> {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, index) => index + 1)
  }
  if (page <= 4) return [1, 2, 3, 4, 5, 'ellipsis-trailing', totalPages]
  if (page >= totalPages - 3) {
    return [
      1,
      'ellipsis-leading',
      totalPages - 4,
      totalPages - 3,
      totalPages - 2,
      totalPages - 1,
      totalPages,
    ]
  }
  return [
    1,
    'ellipsis-leading',
    page - 1,
    page,
    page + 1,
    'ellipsis-trailing',
    totalPages,
  ]
}

function formatStatementUSD(value: string | null) {
  if (value == null) return '—'
  return value.startsWith('-') ? `-$${value.slice(1)}` : `$${value}`
}
