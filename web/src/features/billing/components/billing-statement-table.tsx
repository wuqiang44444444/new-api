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
import { useNavigate } from '@tanstack/react-router'
import { ChevronDown, Download, FileSearch, ReceiptText } from 'lucide-react'
import { Fragment, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import { downloadBillingStatementCsv } from '../lib/statement-csv'
import {
  buildBillingStatementRows,
  type BillingStatementRow,
} from '../lib/statement-rows'
import type {
  BillingBreakdownItem,
  BillingPeriod,
  BillingStatementItem,
  BillingView,
} from '../types'
import { BillingStatementRowDetails } from './billing-statement-row-details'

type BillingStatementTableProps = {
  items: BillingStatementItem[]
  breakdownItems: BillingBreakdownItem[]
  breakdownLoading: boolean
  breakdownUnavailable: boolean
  period: BillingPeriod
  view: BillingView
  loading: boolean
  onViewChange: (view: BillingView) => void
}

const BILLING_TABLE_SKELETON_KEYS = [
  'billing-row-1',
  'billing-row-2',
  'billing-row-3',
  'billing-row-4',
  'billing-row-5',
]

export function BillingStatementTable(props: BillingStatementTableProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const locale = toIntlLocale(i18n.resolvedLanguage)
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set())
  const rows = useMemo(
    () =>
      buildBillingStatementRows(props.items, props.breakdownItems, props.view),
    [props.breakdownItems, props.items, props.view]
  )
  const totalQuota = rows.reduce((sum, row) => sum + row.netQuota, 0)
  const desktopColumnCount = props.view === 'detail' ? 9 : 8
  const percentFormatter = new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumFractionDigits: 1,
  })

  const keyLabel = (row: BillingStatementRow) => {
    if (row.tokenId == null) return '-'
    if (row.tokenId <= 0) return row.tokenName || `${t('Unknown')} API Key`
    return (
      row.tokenName ||
      t('Deleted API Key ({{id}})', {
        id: row.tokenId,
      })
    )
  }

  const openUsageLogs = (row: BillingStatementRow) => {
    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        page: 1,
        type: ['2', '6'],
        token: row.tokenName || undefined,
        model: row.modelName || undefined,
        startTime: props.period.start_timestamp * 1000,
        endTime: props.period.end_timestamp * 1000,
      },
    })
  }

  const toggleRow = (rowId: string) => {
    setExpandedRows((current) => {
      const next = new Set(current)
      if (next.has(rowId)) next.delete(rowId)
      else next.add(rowId)
      return next
    })
  }

  const exportCsv = () => {
    downloadBillingStatementCsv({
      rows,
      totalQuota,
      locale,
      period: props.period,
      t,
      keyLabel,
    })
  }

  const cacheSummary = (row: BillingStatementRow) => {
    if (props.breakdownLoading) return <Skeleton className='ml-auto h-7 w-20' />
    if (!row.breakdownComplete) {
      return <span className='text-destructive'>{t('Unavailable')}</span>
    }
    if (!row.cache) return <span className='text-muted-foreground'>-</span>
    return (
      <div>
        <div className='font-medium'>
          {percentFormatter.format(row.cache.hit_request_ratio)}
        </div>
        <div className='text-muted-foreground text-[11px]'>
          {row.cache.read_tokens.toLocaleString(locale)} /{' '}
          {row.cache.write_tokens.toLocaleString(locale)}
        </div>
      </div>
    )
  }

  const contextSummary = (row: BillingStatementRow) => {
    if (props.breakdownLoading) return <Skeleton className='ml-auto h-7 w-20' />
    if (!row.breakdownComplete) {
      return <span className='text-destructive'>{t('Unavailable')}</span>
    }
    if (!row.context) return <span className='text-muted-foreground'>-</span>
    return (
      <div>
        <div className='font-medium'>
          {row.context.long_requests.toLocaleString(locale)} /{' '}
          {row.context.short_requests.toLocaleString(locale)}
        </div>
        <div className='text-muted-foreground text-[11px]'>
          {t('Long / Short')}
        </div>
      </div>
    )
  }

  return (
    <Card>
      <CardHeader className='border-b'>
        <CardTitle>{t('Statement Details')}</CardTitle>
        <CardDescription>
          {t(
            'Amounts are taken from settlement logs and may be updated by later refunds or adjustments.'
          )}
        </CardDescription>
        <CardAction className='flex items-center gap-2'>
          <Tabs
            value={props.view}
            onValueChange={(value) => props.onViewChange(value as BillingView)}
          >
            <TabsList>
              <TabsTrigger value='detail'>{t('Details')}</TabsTrigger>
              <TabsTrigger value='model'>{t('By Model')}</TabsTrigger>
              <TabsTrigger value='key'>{t('By API Key')}</TabsTrigger>
            </TabsList>
          </Tabs>
          <Button
            variant='outline'
            size='sm'
            disabled={props.loading || rows.length === 0}
            onClick={exportCsv}
          >
            <Download />
            <span className='hidden sm:inline'>{t('Export CSV')}</span>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='p-0'>
        {props.loading && (
          <div className='space-y-2 p-4'>
            {BILLING_TABLE_SKELETON_KEYS.map((key) => (
              <Skeleton key={key} className='h-11 w-full' />
            ))}
          </div>
        )}
        {!props.loading && rows.length === 0 && (
          <EmptyState
            icon={ReceiptText}
            title={t('No settled usage in this period')}
            description={t('Try another billing period, API key, or model.')}
          />
        )}
        {!props.loading && rows.length > 0 && (
          <>
            <div className='hidden md:block'>
              <Table>
                <TableHeader>
                  <TableRow>
                    {props.view !== 'model' && (
                      <TableHead>{t('API Key')}</TableHead>
                    )}
                    {props.view !== 'key' && (
                      <TableHead>{t('Model')}</TableHead>
                    )}
                    <TableHead className='text-right'>
                      {t('Requests')}
                    </TableHead>
                    <TableHead className='text-right'>{t('Tokens')}</TableHead>
                    <TableHead className='text-right'>{t('Cache')}</TableHead>
                    <TableHead className='text-right'>{t('Context')}</TableHead>
                    <TableHead className='text-right'>
                      {t('Billing and settlement')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Cost Share')}
                    </TableHead>
                    <TableHead className='text-right'>{t('Action')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((row) => {
                    const expanded = expandedRows.has(row.id)
                    return (
                      <Fragment key={row.id}>
                        <TableRow>
                          {props.view !== 'model' && (
                            <TableCell className='font-medium'>
                              {keyLabel(row)}
                            </TableCell>
                          )}
                          {props.view !== 'key' && (
                            <TableCell className='font-mono'>
                              {row.modelName || '-'}
                            </TableCell>
                          )}
                          <TableCell className='text-right'>
                            {row.requests.toLocaleString(locale)}
                          </TableCell>
                          <TableCell className='text-right'>
                            <div className='font-medium'>
                              {row.totalTokens.toLocaleString(locale)}
                            </div>
                            <div className='text-muted-foreground text-[11px]'>
                              {row.promptTokens.toLocaleString(locale)} /{' '}
                              {row.completionTokens.toLocaleString(locale)}
                            </div>
                          </TableCell>
                          <TableCell className='text-right'>
                            {cacheSummary(row)}
                          </TableCell>
                          <TableCell className='text-right'>
                            {contextSummary(row)}
                          </TableCell>
                          <TableCell className='text-right'>
                            <div className='text-muted-foreground text-[11px]'>
                              {t('Gross')}: {formatLogQuota(row.grossQuota)}
                            </div>
                            <div className='text-muted-foreground text-[11px]'>
                              {t('Refund')}: {formatLogQuota(row.refundQuota)}
                            </div>
                            <div className='font-semibold'>
                              {t('Net')}: {formatLogQuota(row.netQuota)}
                            </div>
                          </TableCell>
                          <TableCell className='text-right'>
                            {percentFormatter.format(
                              totalQuota > 0 ? row.netQuota / totalQuota : 0
                            )}
                          </TableCell>
                          <TableCell className='text-right'>
                            <div className='flex justify-end gap-1'>
                              <Button
                                variant='ghost'
                                size='sm'
                                aria-expanded={expanded}
                                aria-controls={`billing-row-details-${row.id}`}
                                onClick={() => toggleRow(row.id)}
                              >
                                {t('Details')}
                                <ChevronDown
                                  className={cn(
                                    'transition-transform',
                                    expanded && 'rotate-180'
                                  )}
                                />
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon-sm'
                                aria-label={t('Usage Details')}
                                onClick={() => openUsageLogs(row)}
                              >
                                <FileSearch />
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                        {expanded && (
                          <TableRow
                            id={`billing-row-details-${row.id}`}
                            className='bg-muted/20 hover:bg-muted/20'
                          >
                            <TableCell
                              colSpan={desktopColumnCount}
                              className='p-0'
                            >
                              <BillingStatementRowDetails
                                row={row}
                                breakdownLoading={props.breakdownLoading}
                                breakdownUnavailable={
                                  props.breakdownUnavailable
                                }
                              />
                            </TableCell>
                          </TableRow>
                        )}
                      </Fragment>
                    )
                  })}
                </TableBody>
              </Table>
            </div>

            <div className='divide-y md:hidden'>
              {rows.map((row) => {
                const expanded = expandedRows.has(row.id)
                let mobileCacheValue = '-'
                let mobileContextValue = '-'
                if (!row.breakdownComplete) {
                  mobileCacheValue = t('Unavailable')
                  mobileContextValue = t('Unavailable')
                } else {
                  if (row.cache) {
                    mobileCacheValue = percentFormatter.format(
                      row.cache.hit_request_ratio
                    )
                  }
                  if (row.context) {
                    mobileContextValue = `${row.context.long_requests.toLocaleString(
                      locale
                    )} / ${row.context.short_requests.toLocaleString(locale)}`
                  }
                }
                return (
                  <div key={row.id} className='space-y-3 p-4'>
                    <div className='flex items-start justify-between gap-3'>
                      <div className='min-w-0'>
                        {props.view !== 'model' && (
                          <div className='truncate font-medium'>
                            {keyLabel(row)}
                          </div>
                        )}
                        {props.view !== 'key' && (
                          <div className='text-muted-foreground truncate font-mono text-xs'>
                            {row.modelName || '-'}
                          </div>
                        )}
                      </div>
                      <div className='shrink-0 text-right'>
                        <div className='font-semibold'>
                          {formatLogQuota(row.netQuota)}
                        </div>
                        <div className='text-muted-foreground text-xs'>
                          {percentFormatter.format(
                            totalQuota > 0 ? row.netQuota / totalQuota : 0
                          )}
                        </div>
                      </div>
                    </div>
                    <div className='grid grid-cols-2 gap-2 text-xs'>
                      <DetailSummary
                        label={t('Requests')}
                        value={row.requests.toLocaleString(locale)}
                      />
                      <DetailSummary
                        label={t('Total Tokens')}
                        value={row.totalTokens.toLocaleString(locale)}
                      />
                      <DetailSummary
                        label={t('Cache')}
                        value={mobileCacheValue}
                      />
                      <DetailSummary
                        label={t('Long / Short')}
                        value={mobileContextValue}
                      />
                      <DetailSummary
                        label={t('Gross')}
                        value={formatLogQuota(row.grossQuota)}
                      />
                      <DetailSummary
                        label={t('Refund')}
                        value={formatLogQuota(row.refundQuota)}
                      />
                      <DetailSummary
                        label={t('Net')}
                        value={formatLogQuota(row.netQuota)}
                      />
                    </div>
                    <div className='grid grid-cols-2 gap-2'>
                      <Button
                        variant='outline'
                        size='sm'
                        aria-expanded={expanded}
                        aria-controls={`billing-mobile-details-${row.id}`}
                        onClick={() => toggleRow(row.id)}
                      >
                        {t('Details')}
                        <ChevronDown
                          className={cn(
                            'transition-transform',
                            expanded && 'rotate-180'
                          )}
                        />
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => openUsageLogs(row)}
                      >
                        <FileSearch />
                        {t('Usage Details')}
                      </Button>
                    </div>
                    {expanded && (
                      <div id={`billing-mobile-details-${row.id}`}>
                        <BillingStatementRowDetails
                          row={row}
                          breakdownLoading={props.breakdownLoading}
                          breakdownUnavailable={props.breakdownUnavailable}
                        />
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}

function DetailSummary(props: { label: string; value: string }) {
  return (
    <div>
      <span className='text-muted-foreground'>{props.label}</span>
      <div className='mt-0.5 font-medium'>{props.value}</div>
    </div>
  )
}
