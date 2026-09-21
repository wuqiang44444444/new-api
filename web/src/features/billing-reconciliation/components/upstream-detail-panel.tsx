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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { getAdminUpstreamDetails } from '../api'
import { createUpstreamExport } from '../export-api'
import { formatCustomerStatementQuota } from '../lib'
import type { UpstreamDetailItem } from '../types'
import {
  formatShanghaiTimestamp,
  upstreamDetailEventLabel,
} from '../upstream-reconciliation-utils'
import {
  cacheMeterUnavailableLabel,
  billingSecondsSourceLabel,
  formatStatementUsage,
} from '../upstream-statement-utils'
import { UpstreamDataStatus } from './upstream-data-status'
import { UsageCell } from './upstream-reconciliation-table'

export type UpstreamDetailSelection = {
  evidenceFilter?: string
  evidenceLabel?: string
  urlKey?: string
  channelId?: number
  channelName?: string
  providerModel?: string
  providerModelFallback?: boolean
  billingMode?: string
}

type UpstreamDetailPanelProps = {
  period: { start_timestamp: number; end_timestamp: number }
  selection: UpstreamDetailSelection
  onClose: () => void
  onShowExports: () => void
}

// 逐笔证据：双方请求 ID 并列展示；任务 ID 单列，缺失显示“未记录”，
// 不按时间或金额猜测匹配；资金调整行有明确事件标记，不冒充新的上游调用。
export function UpstreamDetailPanel(props: UpstreamDetailPanelProps) {
  const { t } = useTranslation()
  const [requestId, setRequestId] = useState('')
  const [upstreamRequestId, setUpstreamRequestId] = useState('')
  const [page, setPage] = useState(1)
  const pageSize = 50
  const queryClient = useQueryClient()
  const [exporting, setExporting] = useState(false)
  const submitExport = async () => {
    setExporting(true)
    try {
      await createUpstreamExport({
        job_type: 'upstream_details',
        start_timestamp: props.period.start_timestamp,
        end_timestamp: props.period.end_timestamp + 1,
        url_key: props.selection.urlKey,
        channel_id: props.selection.channelId,
        model_name: props.selection.providerModel,
        provider_model_fallback: props.selection.providerModelFallback,
        evidence_filter: props.selection.evidenceFilter,
        billing_mode: props.selection.billingMode,
        request_id: requestId,
        upstream_request_id: upstreamRequestId,
      })
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
      props.onShowExports()
      toast.success(t('Export submitted. Track it in export jobs.'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to submit export.')
      )
    } finally {
      setExporting(false)
    }
  }

  const query = useQuery({
    queryKey: [
      'billing-upstream-details',
      props.period.start_timestamp,
      props.period.end_timestamp,
      props.selection.urlKey ?? '',
      props.selection.channelId ?? 0,
      props.selection.providerModel ?? '',
      props.selection.providerModelFallback,
      props.selection.billingMode ?? '',
      props.selection.evidenceFilter ?? '',
      requestId,
      upstreamRequestId,
      page,
    ],
    queryFn: async () => {
      const response = await getAdminUpstreamDetails({
        start_timestamp: props.period.start_timestamp,
        end_timestamp: props.period.end_timestamp,
        ...(props.selection.urlKey ? { url_key: props.selection.urlKey } : {}),
        ...(props.selection.channelId
          ? { channel_id: props.selection.channelId }
          : {}),
        provider_model_fallback: props.selection.providerModelFallback,
        evidence_filter: props.selection.evidenceFilter,
        ...(props.selection.providerModel
          ? { model_name: props.selection.providerModel }
          : {}),
        ...(props.selection.billingMode
          ? { billing_mode: props.selection.billingMode }
          : {}),
        ...(requestId ? { request_id: requestId } : {}),
        ...(upstreamRequestId
          ? { upstream_request_id: upstreamRequestId }
          : {}),
        page,
        page_size: pageSize,
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load upstream details.')
        )
      }
      return response.data
    },
    staleTime: 30_000,
    retry: false,
  })

  const details = query.data?.result
  const totalPages = details
    ? Math.max(1, Math.ceil(details.total / pageSize))
    : 1

  return (
    <div className='space-y-3'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between'>
        <div className='grid flex-1 gap-2 sm:grid-cols-2 xl:max-w-2xl'>
          <Input
            aria-label={t('Our request ID')}
            placeholder={t('Our request ID')}
            value={requestId}
            onChange={(event) => {
              setRequestId(event.target.value)
              setPage(1)
            }}
          />
          <Input
            aria-label={t('Upstream request ID')}
            placeholder={t('Upstream request ID')}
            value={upstreamRequestId}
            onChange={(event) => {
              setUpstreamRequestId(event.target.value)
              setPage(1)
            }}
          />
        </div>
        <Button
          variant='outline'
          size='sm'
          disabled={exporting}
          onClick={() => void submitExport()}
        >
          {t('Export all matching details')}
        </Button>
        <Button variant='outline' size='sm' onClick={props.onShowExports}>
          {t('Export jobs')}
        </Button>
        <Button variant='outline' size='sm' onClick={props.onClose}>
          {t('Close details')}
        </Button>
      </div>

      {query.isPending ? (
        <div role='status'>
          <LoadingState message={t('Loading upstream details...')} />
        </div>
      ) : null}
      {query.isError ? (
        <ErrorState
          title={t('Unable to load upstream details.')}
          description={
            query.error instanceof Error
              ? query.error.message
              : t('Please try again later.')
          }
          onRetry={() => void query.refetch()}
        />
      ) : null}

      {!query.isPending && !query.isError ? (
        <Table className='min-w-300'>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Record ID')}</TableHead>
              <TableHead>{t('Time')}</TableHead>
              <TableHead>{t('Upstream channel')}</TableHead>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Billing mode')}</TableHead>
              <TableHead className='text-right'>{t('Input tokens')}</TableHead>
              <TableHead className='text-right'>{t('Output tokens')}</TableHead>
              <TableHead className='text-right'>
                {t('Cache read tokens')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Cache write tokens')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Billable seconds')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Original amount (local official price)')}
              </TableHead>
              <TableHead>{t('Our request ID')}</TableHead>
              <TableHead>{t('Upstream request ID')}</TableHead>
              <TableHead>{t('Platform task ID')}</TableHead>
              <TableHead>{t('Upstream task ID')}</TableHead>
              <TableHead>{t('Row kind')}</TableHead>
              <TableHead>{t('Data status')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(details?.items ?? []).map((item) => (
              <DetailRow key={item.row_id} item={item} />
            ))}
            {!query.isPending && (details?.items.length ?? 0) === 0 ? (
              <TableRow>
                <TableCell className='text-muted-foreground' colSpan={17}>
                  {t('No evidence rows match the current filters.')}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      ) : null}

      <div className='flex items-center justify-between text-sm'>
        <span className='text-muted-foreground'>
          {details
            ? t('{{total}} rows in the current filters', {
                total: details.total,
              })
            : ''}
        </span>
        <div className='flex items-center gap-2'>
          <Button
            disabled={page <= 1 || query.isFetching}
            size='xs'
            variant='outline'
            onClick={() => setPage((current) => Math.max(1, current - 1))}
          >
            {t('Previous')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {page} / {totalPages}
          </span>
          <Button
            disabled={page >= totalPages || query.isFetching}
            size='xs'
            variant='outline'
            onClick={() => setPage((current) => current + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </div>
    </div>
  )
}

function DetailRow(props: { item: UpstreamDetailItem }) {
  const { t } = useTranslation()
  const item = props.item
  let pricingMode = t('Unknown')
  if (item.test_pricing?.mode === 'ratio') pricingMode = t('Ratio pricing')
  else if (item.test_pricing?.mode === 'fixed_price') {
    pricingMode = t('Fixed-price pricing')
  } else if (item.test_pricing?.mode === 'tiered_expr') {
    pricingMode = t('Expression pricing')
  }
  let pricingStatus = t('Pricing evidence incomplete')
  if (item.test_pricing?.status === 'priced') {
    pricingStatus = t('Priced from recorded evidence')
  } else if (item.test_pricing?.status === 'estimated') {
    pricingStatus = t(
      'Estimated usage or fees; cost pending, excluded from confirmed totals'
    )
  }
  if (item.test_pricing?.basis === 'historical_replay') {
    pricingStatus = t('Recalculated from recorded prices and usage')
  }
  if (item.test_pricing?.basis === 'recorded_expression_result') {
    pricingStatus = t('Recorded expression result; group multiplier 1')
  }
  let seconds = '—'
  if (item.seconds != null) {
    seconds = formatStatementUsage(
      Number(item.seconds),
      document.documentElement.lang || 'en'
    )
  } else if (item.data_quality?.seconds_unavailable_rows) {
    seconds = t('Not recorded')
  }
  return (
    <TableRow>
      <TableCell>{item.row_id}</TableCell>
      <TableCell className='whitespace-nowrap'>
        {formatShanghaiTimestamp(item.time)}
      </TableCell>
      <TableCell>
        {item.channel_name}
        <span className='text-muted-foreground'> #{item.channel_id}</span>
      </TableCell>
      <TableCell>
        <div className='flex flex-col'>
          <span>{item.customer_model}</span>
          <span className='text-muted-foreground text-xs'>
            {item.provider_model_fallback
              ? t('Provider model not recorded')
              : item.provider_model}
          </span>
        </div>
      </TableCell>
      <TableCell className='text-xs'>{item.billing_mode}</TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatStatementUsage(
          item.recorded_input_tokens,
          document.documentElement.lang || 'en'
        )}
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatStatementUsage(
          item.output_tokens,
          document.documentElement.lang || 'en'
        )}
      </TableCell>
      <UsageCell
        value={item.billing_mode === 'token' ? item.cache_read_tokens : null}
        unavailableLabel={cacheMeterUnavailableLabel(
          item.data_quality,
          'read',
          t
        )}
      />
      <UsageCell
        value={item.billing_mode === 'token' ? item.cache_write_tokens : null}
        unavailableLabel={cacheMeterUnavailableLabel(
          item.data_quality,
          'write',
          t
        )}
      />
      <TableCell className='text-right tabular-nums'>
        {seconds}
        {billingSecondsSourceLabel(item.seconds_source, t) ? (
          <span className='text-muted-foreground block text-xs'>
            {billingSecondsSourceLabel(item.seconds_source, t)}
          </span>
        ) : null}
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {item.original_amount == null ? (
          <span className='text-muted-foreground'>
            {item.event === 'channel_test'
              ? t('Test amount pending')
              : t('Unknown')}
          </span>
        ) : (
          formatCustomerStatementQuota(item.original_amount)
        )}
      </TableCell>
      <TableCell className='font-mono text-xs'>
        {item.request_id || t('Not recorded')}
      </TableCell>
      <TableCell className='font-mono text-xs'>
        {item.upstream_request_id || t('Not recorded')}
      </TableCell>
      <TableCell className='font-mono text-xs'>
        {item.platform_task_id || t('Not recorded')}
      </TableCell>
      <TableCell className='font-mono text-xs'>
        {item.upstream_task_id || t('Not recorded')}
      </TableCell>
      <TableCell className='text-xs'>
        {upstreamDetailEventLabel(item.event, t)}
        {item.test_pricing && (
          <span className='text-muted-foreground block'>
            {pricingMode}
            {' · '}
            {pricingStatus}
            {item.test_pricing.recorded_quota != null &&
              item.test_pricing.recomputed_quota != null &&
              item.test_pricing.recorded_quota !==
                item.test_pricing.recomputed_quota && (
                <span className='block'>
                  {t('Recorded test fee: {{amount}}', {
                    amount: formatCustomerStatementQuota(
                      item.test_pricing.recorded_quota
                    ),
                  })}
                </span>
              )}
          </span>
        )}
      </TableCell>
      <TableCell>
        <UpstreamDataStatus
          quality={item.data_quality}
          originalAmount={item.original_amount}
          usageOnly={
            item.event === 'channel_test' && item.original_amount == null
          }
        />
      </TableCell>
    </TableRow>
  )
}
