import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from '@/components/ui/collapsible'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { LOG_TYPES } from '@/features/usage-logs/constants'
import { formatTimestampToDate } from '@/lib/format'

import {
  cancelExport,
  isExportJobActive,
  listSelfExports,
  requestExportDownload,
  resubmitExport,
  type CustomerExportJobView,
} from './export-api'
import { billingModeLabel, formatInteger } from './lib'

type ExportJobsDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/**
 * 导出记录抽屉：列出当前登录用户发起的导出任务。只轮询任务小表；
 * 进行中的任务每 5 秒刷新，全部终态后停止轮询。
 */
export function ExportJobsDrawer(props: ExportJobsDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const jobsQuery = useQuery({
    queryKey: ['customer-export-jobs'],
    queryFn: async () => {
      const jobs = await listSelfExports()
      return jobs
    },
    enabled: props.open,
    refetchInterval: (query) => {
      const jobs = query.state.data ?? []
      if (!props.open || !jobs.some(isExportJobActive)) return false
      return 5_000
    },
  })

  const jobs = jobsQuery.data ?? []

  const handleCancel = async (job: CustomerExportJobView) => {
    try {
      await cancelExport(job.job_id)
      toast.success(t('Export cancellation requested.'))
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to cancel export.')
      )
    }
  }

  const handleResubmit = async (job: CustomerExportJobView) => {
    try {
      await resubmitExport(job)
      toast.success(t('Export submitted. Track it in export jobs.'))
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to submit export.')
      )
    }
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-md'
      >
        <SheetHeader>
          <SheetTitle>{t('Export jobs')}</SheetTitle>
          <SheetDescription>
            {t(
              'Jobs you submitted. Files stay downloadable until they expire.'
            )}
          </SheetDescription>
        </SheetHeader>
        <div className='flex-1 space-y-3 overflow-y-auto px-4 pb-6'>
          {jobsQuery.isPending && <LoadingState />}
          {jobsQuery.isError && (
            <ErrorState
              title={t('Unable to load export jobs.')}
              onRetry={() => void jobsQuery.refetch()}
            />
          )}
          {!jobsQuery.isPending && !jobsQuery.isError && jobs.length === 0 && (
            <p className='text-muted-foreground py-10 text-center text-sm'>
              {t('No export jobs yet.')}
            </p>
          )}
          {props.open &&
            jobs.map((job) => (
              <ExportJobCard
                key={job.job_id}
                job={job}
                onCancel={() => handleCancel(job)}
                onResubmit={() => handleResubmit(job)}
              />
            ))}
        </div>
      </SheetContent>
    </Sheet>
  )
}

function exportStatusVariant(status: CustomerExportJobView['status']) {
  if (status === 'succeeded') return 'secondary' as const
  if (status === 'failed') return 'destructive' as const
  return 'outline' as const
}

function exportStatusLabel(
  job: CustomerExportJobView,
  translate: (key: string) => string
) {
  if (isExportJobActive(job) && job.cancel_requested) {
    return translate('Cancelling')
  }
  switch (job.status) {
    case 'queued':
      return translate('Queued')
    case 'running':
      return job.progress.waiting_for_resources
        ? translate('Waiting for resources')
        : translate('Generating')
    case 'succeeded':
      return translate('Ready to download')
    case 'failed':
      return translate('Failed')
    case 'cancelled':
      return translate('Cancelled')
    case 'expired':
      return translate('Expired')
    case 'cancelling':
      return translate('Cancelling')
    default:
      return job.status
  }
}

function exportTypeLabel(
  job: CustomerExportJobView,
  translate: (key: string) => string
) {
  switch (job.job_type) {
    case 'upstream_details':
      return translate('Upstream details')
    case 'statement_summary':
      return translate('Statement summary')
    case 'statement_details':
      return translate('Statement details')
    default:
      return translate('Usage records')
  }
}

function ExportJobCard(props: {
  job: CustomerExportJobView
  onCancel: () => void
  onResubmit: () => void
}) {
  const { t } = useTranslation()
  const { job } = props
  const dateFormat = new Intl.DateTimeFormat(undefined, {
    timeZone: job.filters.timezone,
    dateStyle: 'short',
    timeStyle: 'short',
  })
  const period = `${dateFormat.format(job.filters.start_timestamp * 1000)} ~ ${dateFormat.format((job.filters.end_timestamp - 1) * 1000)}`
  const cancelling = job.status === 'cancelling' || job.cancel_requested
  const statusVariant = exportStatusVariant(job.status)

  return (
    <article
      className='rounded-lg border p-3'
      aria-label={
        job.job_type === 'upstream_details'
          ? t('Upstream details')
          : t('Customer #{{id}}', { id: job.target_user_id })
      }
    >
      <div className='flex items-center justify-between gap-2'>
        <span className='text-sm font-medium'>{exportTypeLabel(job, t)}</span>
        <Badge variant={statusVariant} className='shrink-0'>
          {exportStatusLabel(job, t)}
        </Badge>
      </div>
      <div className='text-muted-foreground mt-1 space-y-0.5 text-xs'>
        <p className='font-medium'>
          {job.job_type === 'upstream_details'
            ? t('Upstream details')
            : t('Customer #{{id}}', { id: job.target_user_id })}
        </p>
        <p>
          {period} · {job.filters.timezone}
        </p>
        <ExportScope job={job} />
        {job.filters.model_name && (
          <p>
            {t('Model')}: {job.filters.model_name}
          </p>
        )}
        {(job.status === 'running' || job.status === 'cancelling') && (
          <p>
            {t('Scanned {{scanned}} records · {{matched}} matched', {
              scanned: formatInteger(job.progress.scanned),
              matched: formatInteger(job.progress.matched),
            })}
          </p>
        )}
        {job.status === 'succeeded' &&
          (job.artifact && job.artifact.files.length > 0 ? (
            <p>
              {t('Historical export generated {{time}} · {{count}} records', {
                time: formatTimestampToDate(job.artifact.generated_at),
                count: formatInteger(job.artifact.line_count),
              })}
            </p>
          ) : (
            <p>{t('No records matched this export scope.')}</p>
          ))}
        {job.status === 'succeeded' &&
          job.expires_at != null &&
          job.expires_at > 0 && (
            <p>
              {t('Downloadable until {{time}}', {
                time: formatTimestampToDate(job.expires_at),
              })}
            </p>
          )}
        {job.status === 'failed' && job.error_code === 'queue_timeout' && (
          <p className='text-destructive'>{t('Queue wait budget exceeded.')}</p>
        )}
        {job.status === 'failed' && (
          <p className='text-destructive'>
            {t('Export failed. You can submit it again later.')}
          </p>
        )}
      </div>
      <div className='mt-2 flex justify-end gap-2'>
        {(job.status === 'queued' ||
          job.status === 'running' ||
          job.status === 'cancelling') && (
          <Button
            variant='outline'
            size='sm'
            disabled={cancelling}
            onClick={props.onCancel}
          >
            {cancelling ? t('Cancelling') : t('Cancel')}
          </Button>
        )}
        {(job.status === 'failed' || job.status === 'expired') && (
          <Button size='sm' variant='outline' onClick={props.onResubmit}>
            {t('Regenerate')}
          </Button>
        )}
      </div>
      {job.status === 'succeeded' &&
        job.artifact &&
        job.artifact.files.length > 0 && <ExportFiles jobId={job.job_id} />}
    </article>
  )
}

function ExportScope({ job }: { job: CustomerExportJobView }) {
  const { t } = useTranslation()
  const f = job.filters
  const fields = [
    [t('Username'), f.username],
    [t('Channel ID'), f.channel_id],
    [t('API Key'), f.token_name],
    [t('Model'), f.model_name],
    [
      t('Billing Mode'),
      f.billing_mode ? t(billingModeLabel(f.billing_mode)) : undefined,
    ],
    [t('Group'), f.group],
    [t('Request ID'), f.request_id],
    [t('Upstream Request ID'), f.upstream_request_id],
    [
      t('Log Types'),
      f.log_types
        ?.map((type) =>
          t(LOG_TYPES.find((item) => item.value === type)?.label ?? 'Unknown')
        )
        .join(', '),
    ],
    [t('Language'), f.language],
    [t('Currency'), f.currency],
  ].filter(([, value]) => value != null && value !== '')
  return (
    <Collapsible>
      <CollapsibleTrigger render={<Button variant='ghost' size='sm' />}>
        {t('Export scope')}
      </CollapsibleTrigger>
      <CollapsibleContent>
        {f.token_id != null && (
          <p>{t('API Key ID: {{id}}', { id: f.token_id })}</p>
        )}
        <dl className='space-y-1 break-all'>
          {fields.map(([label, value]) => (
            <div key={label}>
              <dt className='inline'>{label}: </dt>
              <dd className='inline'>{value}</dd>
            </div>
          ))}
        </dl>
      </CollapsibleContent>
    </Collapsible>
  )
}

function ExportFiles({ jobId }: { jobId: string }) {
  const { t } = useTranslation()
  const [expired, setExpired] = useState(false)
  const download = useMutation({
    mutationFn: () => requestExportDownload(jobId),
  })
  const files = download.data?.files
  useEffect(() => {
    if (!files?.length) return
    const remaining =
      Math.min(...files.map((file) => file.expires_at * 1000)) - Date.now()
    setExpired(remaining <= 0)
    if (remaining <= 0) return
    const timer = window.setTimeout(() => setExpired(true), remaining)
    return () => window.clearTimeout(timer)
  }, [files])

  let downloadLabel = download.data
    ? t('Refresh download links')
    : t('Download')
  if (download.isPending) downloadLabel = t('Loading...')

  return (
    <div className='mt-2 space-y-2' aria-live='polite'>
      <Button
        size='sm'
        disabled={download.isPending}
        onClick={() => download.mutate()}
      >
        {downloadLabel}
      </Button>
      {download.isError && (
        <p className='text-destructive text-sm'>
          {t('Unable to download export.')}
        </p>
      )}
      {expired && (
        <p className='text-muted-foreground text-sm'>
          {t('Download links expired. Refresh them to continue.')}
        </p>
      )}
      {download.data && !files?.length && (
        <p className='text-muted-foreground text-sm'>
          {t('No records matched this export scope.')}
        </p>
      )}
      {!expired && files && (
        <ul className='space-y-2'>
          {files.map((file) => (
            <li key={file.file_name} className='space-y-1'>
              <Button
                role='link'
                variant='outline'
                size='sm'
                className='h-auto max-w-full break-all whitespace-normal'
                render={
                  <a
                    href={file.url}
                    target='_blank'
                    rel='noopener noreferrer'
                  />
                }
                onClick={(event) => {
                  if (file.expires_at * 1000 <= Date.now()) {
                    event.preventDefault()
                    setExpired(true)
                  }
                }}
              >
                {t('Download {{file}}', { file: file.file_name })}
              </Button>
              <p className='text-muted-foreground text-xs'>
                {t('{{count}} records · {{bytes}} bytes', {
                  count: formatInteger(file.line_count),
                  bytes: formatInteger(file.size_bytes),
                })}
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
