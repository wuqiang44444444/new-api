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
import { Download, FileSearch, ReceiptText } from 'lucide-react'
import { useMemo } from 'react'
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

import { escapeCsvCell, formatBillingDuration } from '../lib'
import type { BillingPeriod, BillingStatementItem, BillingView } from '../types'

type StatementRow = {
  id: string
  tokenId?: number
  tokenName?: string
  modelName?: string
  requests: number
  promptTokens: number
  completionTokens: number
  totalTokens: number
  quota: number
  averageUseTimeSeconds: number
  streamRequests: number
}

type BillingStatementTableProps = {
  items: BillingStatementItem[]
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

function buildStatementRows(
  items: BillingStatementItem[],
  view: BillingView
): StatementRow[] {
  const rows = new Map<string, StatementRow>()
  for (const item of items) {
    let id = `detail:${item.token_id}:${item.model_name}`
    if (view === 'model') {
      id = `model:${item.model_name}`
    } else if (view === 'key') {
      id = `key:${item.token_id}`
    }
    const existing = rows.get(id)
    if (!existing) {
      rows.set(id, {
        id,
        tokenId: view === 'model' ? undefined : item.token_id,
        tokenName: view === 'model' ? undefined : item.token_name,
        modelName: view === 'key' ? undefined : item.model_name,
        requests: item.requests,
        promptTokens: item.prompt_tokens,
        completionTokens: item.completion_tokens,
        totalTokens: item.total_tokens,
        quota: item.net_quota,
        averageUseTimeSeconds: item.average_use_time_seconds,
        streamRequests: item.stream_requests,
      })
      continue
    }
    const totalUseTime =
      existing.averageUseTimeSeconds * existing.requests +
      item.average_use_time_seconds * item.requests
    existing.requests += item.requests
    existing.promptTokens += item.prompt_tokens
    existing.completionTokens += item.completion_tokens
    existing.totalTokens += item.total_tokens
    existing.quota += item.net_quota
    existing.streamRequests += item.stream_requests
    if (existing.requests > 0) {
      existing.averageUseTimeSeconds = totalUseTime / existing.requests
    }
  }
  return [...rows.values()].sort((left, right) => {
    if (left.quota !== right.quota) return right.quota - left.quota
    return right.totalTokens - left.totalTokens
  })
}

export function BillingStatementTable(props: BillingStatementTableProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const locale = toIntlLocale(i18n.resolvedLanguage)
  const rows = useMemo(
    () => buildStatementRows(props.items, props.view),
    [props.items, props.view]
  )
  const totalQuota = rows.reduce((sum, row) => sum + row.quota, 0)

  const keyLabel = (row: StatementRow) => {
    if (row.tokenId == null) return '-'
    if (row.tokenId <= 0) return row.tokenName || `${t('Unknown')} API Key`
    return (
      row.tokenName ||
      t('Deleted API Key ({{id}})', {
        id: row.tokenId,
      })
    )
  }

  const openUsageLogs = (row: StatementRow) => {
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

  const exportCsv = () => {
    const headers = [
      t('API Key'),
      t('Model'),
      t('Requests'),
      t('Input Tokens'),
      t('Output Tokens'),
      t('Total Tokens'),
      t('Average Response Time'),
      t('Settled Cost'),
      t('Cost Share'),
    ]
    const lines = [
      headers.map(escapeCsvCell).join(','),
      ...rows.map((row) =>
        [
          keyLabel(row),
          row.modelName || '-',
          row.requests,
          row.promptTokens,
          row.completionTokens,
          row.totalTokens,
          formatBillingDuration(row.averageUseTimeSeconds, locale),
          formatLogQuota(row.quota),
          new Intl.NumberFormat(locale, {
            style: 'percent',
            maximumFractionDigits: 1,
          }).format(totalQuota > 0 ? row.quota / totalQuota : 0),
        ]
          .map(escapeCsvCell)
          .join(',')
      ),
    ]
    const blob = new Blob([`\uFEFF${lines.join('\n')}`], {
      type: 'text/csv;charset=utf-8',
    })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `usage-statement-${props.period.start_timestamp}-${props.period.end_timestamp}.csv`
    link.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Card>
      <CardHeader className='border-b'>
        <CardTitle>{t('Statement Details')}</CardTitle>
        <CardDescription>
          {t(
            'Costs are the final settled amounts recorded for each model call.'
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
                    <TableHead className='text-right'>
                      {t('Input Tokens')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Output Tokens')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Total Tokens')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Avg. Response')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Settled Cost')}
                    </TableHead>
                    <TableHead className='text-right'>
                      {t('Cost Share')}
                    </TableHead>
                    <TableHead className='text-right'>{t('Action')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((row) => (
                    <TableRow key={row.id}>
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
                        {row.promptTokens.toLocaleString(locale)}
                      </TableCell>
                      <TableCell className='text-right'>
                        {row.completionTokens.toLocaleString(locale)}
                      </TableCell>
                      <TableCell className='text-right font-medium'>
                        {row.totalTokens.toLocaleString(locale)}
                      </TableCell>
                      <TableCell className='text-right'>
                        {formatBillingDuration(
                          row.averageUseTimeSeconds,
                          locale
                        )}
                      </TableCell>
                      <TableCell className='text-right font-semibold'>
                        {formatLogQuota(row.quota)}
                      </TableCell>
                      <TableCell className='text-right'>
                        {new Intl.NumberFormat(locale, {
                          style: 'percent',
                          maximumFractionDigits: 1,
                        }).format(totalQuota > 0 ? row.quota / totalQuota : 0)}
                      </TableCell>
                      <TableCell className='text-right'>
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => openUsageLogs(row)}
                        >
                          <FileSearch />
                          {t('Usage Details')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <div className='divide-y md:hidden'>
              {rows.map((row) => (
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
                        {formatLogQuota(row.quota)}
                      </div>
                      <div className='text-muted-foreground text-xs'>
                        {new Intl.NumberFormat(locale, {
                          style: 'percent',
                          maximumFractionDigits: 1,
                        }).format(totalQuota > 0 ? row.quota / totalQuota : 0)}
                      </div>
                    </div>
                  </div>
                  <div className='grid grid-cols-2 gap-2 text-xs'>
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Requests')}
                      </span>
                      <div className='mt-0.5 font-medium'>
                        {row.requests.toLocaleString(locale)}
                      </div>
                    </div>
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Total Tokens')}
                      </span>
                      <div className='mt-0.5 font-medium'>
                        {row.totalTokens.toLocaleString(locale)}
                      </div>
                    </div>
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Input / Output')}
                      </span>
                      <div className='mt-0.5 font-medium'>
                        {row.promptTokens.toLocaleString(locale)} /{' '}
                        {row.completionTokens.toLocaleString(locale)}
                      </div>
                    </div>
                    <div>
                      <span className='text-muted-foreground'>
                        {t('Avg. Response')}
                      </span>
                      <div className='mt-0.5 font-medium'>
                        {formatBillingDuration(
                          row.averageUseTimeSeconds,
                          locale
                        )}
                      </div>
                    </div>
                  </div>
                  <Button
                    variant='outline'
                    size='sm'
                    className='w-full'
                    onClick={() => openUsageLogs(row)}
                  >
                    <FileSearch />
                    {t('Usage Details')}
                  </Button>
                </div>
              ))}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
