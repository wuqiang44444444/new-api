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
import { ExportJobsDrawer } from '../export-jobs-drawer'
import { formatCustomerStatementQuota } from '../lib'
import type { UpstreamDetailItem } from '../types'
import { upstreamDetailEventLabel } from '../upstream-reconciliation-utils'
import { formatStatementUsage } from '../upstream-statement-utils'

export type UpstreamDetailSelection = {
  urlKey?: string
  channelId?: number
  providerModel?: string
  billingMode?: string
}

type UpstreamDetailPanelProps = {
  period: { start_timestamp: number; end_timestamp: number }
  selection: UpstreamDetailSelection
  onClose: () => void
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
  const [exportOpen, setExportOpen] = useState(false)
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
        billing_mode: props.selection.billingMode,
        request_id: requestId,
        upstream_request_id: upstreamRequestId,
      })
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
      setExportOpen(true)
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
      props.selection.billingMode ?? '',
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
      <ExportJobsDrawer open={exportOpen} onOpenChange={setExportOpen} />
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
        <Button variant='outline' size='sm' onClick={() => setExportOpen(true)}>
          {t('Export jobs')}
        </Button>
        <Button variant='outline' size='sm' onClick={props.onClose}>
          {t('Close details')}
        </Button>
      </div>

      {query.isError ? (
        <p className='text-destructive text-sm'>
          {query.error instanceof Error
            ? query.error.message
            : t('Please try again later.')}
        </p>
      ) : null}

      <Table className='min-w-300'>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Time')}</TableHead>
            <TableHead>{t('Upstream channel')}</TableHead>
            <TableHead>{t('Model')}</TableHead>
            <TableHead>{t('Billing mode')}</TableHead>
            <TableHead className='text-right'>{t('Input tokens')}</TableHead>
            <TableHead className='text-right'>{t('Output tokens')}</TableHead>
            <TableHead className='text-right'>
              {t('Original amount (local official price)')}
            </TableHead>
            <TableHead>{t('Our request ID')}</TableHead>
            <TableHead>{t('Upstream request ID')}</TableHead>
            <TableHead>{t('Platform task ID')}</TableHead>
            <TableHead>{t('Upstream task ID')}</TableHead>
            <TableHead>{t('Row kind')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(details?.items ?? []).map((item) => (
            <DetailRow key={item.row_id} item={item} />
          ))}
          {!query.isPending && (details?.items.length ?? 0) === 0 ? (
            <TableRow>
              <TableCell className='text-muted-foreground' colSpan={12}>
                {t('No evidence rows match the current filters.')}
              </TableCell>
            </TableRow>
          ) : null}
        </TableBody>
      </Table>

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
  return (
    <TableRow>
      <TableCell className='whitespace-nowrap'>
        {new Date(item.time * 1000).toLocaleString()}
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
      <TableCell className='text-right tabular-nums'>
        {item.original_amount == null ? (
          <span className='text-muted-foreground'>{t('Unknown')}</span>
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
      </TableCell>
    </TableRow>
  )
}
