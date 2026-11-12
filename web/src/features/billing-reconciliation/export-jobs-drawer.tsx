import { ArrowDown01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
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
import { useEffect, useState, type ReactNode } from 'react'
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
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampToDate } from '@/lib/format'

import { BillingPagination } from './components/billing-pagination'
import { ExportArtifactFiles } from './components/export-artifact-files'
import {
  cancelExport,
  getExport,
  isExportJobActive,
  listSelfExports,
  requestExportDownload,
  resubmitExport,
  type CustomerExportJobView,
} from './export-api'
import { billingModeLabel, formatBillingMonth, formatInteger } from './lib'

type ExportJobsDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  children?: ReactNode
  focusedJobId?: string
  billingStatement?: boolean
  upstreamBilling?: boolean
  showCustomer?: boolean
  personalUsage?: boolean
  adminCustomerUsage?: boolean
}

/**
 * 导出记录抽屉：列出当前登录用户发起的导出任务。只轮询任务小表；
 * 进行中的任务每 5 秒刷新，全部终态后停止轮询。
 */
export function ExportJobsDrawer(props: ExportJobsDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const compactExports =
    props.billingStatement ||
    props.upstreamBilling ||
    props.personalUsage ||
    props.adminCustomerUsage

  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [activeJobId, setActiveJobId] = useState<string>()
  const [busyJobId, setBusyJobId] = useState<string>()
  useEffect(() => {
    if (!props.open || !props.focusedJobId) return
    setActiveJobId(props.focusedJobId)
    setPage(1)
  }, [props.focusedJobId, props.open])
  const focusedQuery = useQuery({
    queryKey: ['customer-export-job', activeJobId],
    queryFn: activeJobId ? () => getExport(activeJobId) : skipToken,
    enabled: props.open && !!activeJobId,
    refetchInterval: (query) =>
      props.open && query.state.data && isExportJobActive(query.state.data)
        ? 5_000
        : false,
  })
  const jobsQuery = useQuery({
    queryKey: ['customer-export-jobs', page, pageSize],
    queryFn: async () => {
      const jobs = await listSelfExports(page, pageSize)
      return jobs
    },
    enabled: props.open,
    refetchInterval: (query) => {
      const jobs = query.state.data?.items ?? []
      if (!props.open || !jobs.some(isExportJobActive)) return false
      return 5_000
    },
  })

  const jobs = jobsQuery.data?.items ?? []

  const handleCancel = async (job: CustomerExportJobView) => {
    setBusyJobId(job.job_id)
    try {
      await cancelExport(job.job_id)
      toast.success(t('Export cancellation requested.'))
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-job', job.job_id],
      })
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to cancel export.')
      )
    } finally {
      setBusyJobId(undefined)
    }
  }

  const handleResubmit = async (job: CustomerExportJobView) => {
    setBusyJobId(job.job_id)
    try {
      const submitted = await resubmitExport(job)
      queryClient.setQueryData(
        ['customer-export-job', submitted.job_id],
        submitted
      )
      setActiveJobId(submitted.job_id)
      setPage(1)
      toast.success(t('Export submitted. Track it in export jobs.'))
      await queryClient.invalidateQueries({
        queryKey: ['customer-export-jobs'],
      })
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Unable to submit export.')
      )
    } finally {
      setBusyJobId(undefined)
    }
  }

  return (
    <Sheet
      open={props.open}
      onOpenChange={(open) => {
        if (!open && compactExports) {
          setActiveJobId(undefined)
          setPage(1)
        }
        props.onOpenChange(open)
      }}
    >
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-md'
      >
        <SheetHeader>
          <SheetTitle>
            {props.billingStatement || props.upstreamBilling
              ? t('Export and download')
              : t('Export jobs')}
          </SheetTitle>
          <SheetDescription>
            {compactExports
              ? t('Track exports and download files when they are ready.')
              : t(
                  'Jobs you submitted. Files stay downloadable until they expire.'
                )}
          </SheetDescription>
        </SheetHeader>
        <div className='flex-1 space-y-3 overflow-y-auto px-4 pb-6'>
          {props.children}
          {activeJobId && focusedQuery.isError && (
            <ErrorState
              title={t('Unable to load export jobs.')}
              onRetry={() => void focusedQuery.refetch()}
            />
          )}
          {activeJobId && focusedQuery.data && (
            <section className='space-y-2' aria-live='polite'>
              <h2 className='text-sm font-medium'>
                {compactExports ? t('This export') : t('Current export')}
              </h2>
              <ExportJobCard
                job={focusedQuery.data}
                billingStatement={props.billingStatement}
                upstreamBilling={props.upstreamBilling}
                showCustomer={props.showCustomer}
                personalUsage={props.personalUsage}
                adminCustomerUsage={props.adminCustomerUsage}
                busy={!!busyJobId}
                onCancel={() => handleCancel(focusedQuery.data)}
                onResubmit={() => handleResubmit(focusedQuery.data)}
              />
            </section>
          )}
          {(props.billingStatement || props.upstreamBilling) && (
            <h2 className='text-sm font-medium'>{t('Export jobs')}</h2>
          )}
          {jobsQuery.isPending && <LoadingState />}
          {jobsQuery.isError && (
            <ErrorState
              title={t('Unable to load export jobs.')}
              onRetry={() => void jobsQuery.refetch()}
            />
          )}
          {!jobsQuery.isPending &&
            !jobsQuery.isError &&
            jobs.length === 0 &&
            !focusedQuery.data && (
              <p className='text-muted-foreground py-10 text-center text-sm'>
                {t('No export jobs yet.')}
              </p>
            )}
          {props.open &&
            jobs
              .filter((job) => job.job_id !== focusedQuery.data?.job_id)
              .map((job) => (
                <ExportJobCard
                  key={job.job_id}
                  job={job}
                  billingStatement={props.billingStatement}
                  upstreamBilling={props.upstreamBilling}
                  showCustomer={props.showCustomer}
                  personalUsage={props.personalUsage}
                  adminCustomerUsage={props.adminCustomerUsage}
                  busy={!!busyJobId}
                  onCancel={() => handleCancel(job)}
                  onResubmit={() => handleResubmit(job)}
                />
              ))}
        </div>
        <div className='p-4'>
          <BillingPagination
            label={t('Export job pages')}
            page={page}
            pageSize={pageSize}
            total={jobsQuery.data?.total ?? 0}
            disabled={jobsQuery.isFetching}
            onChange={(p, size) => {
              setPage(p)
              setPageSize(size)
            }}
          />
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
    case 'usage_summary':
      return translate('Usage statistics')
    case 'upstream_summary':
      return translate('Upstream cost statement')
    case 'upstream_details':
      return translate('Upstream details')
    case 'customer_statements':
      return translate('Customer statements')
    case 'statement_summary':
      return translate('Statement summary')
    case 'statement_details':
      return translate('Statement details')
    default:
      return translate('Usage records')
  }
}

function exportAudienceLabel(
  job: CustomerExportJobView,
  translate: (key: string, params?: { id: number }) => string
): string {
  if (job.job_type === 'customer_statements') return translate('All customers')
  if (job.job_type === 'usage_summary' && job.target_user_id === 0) {
    return translate(
      job.filters.usage_view === 'customers'
        ? 'Customer usage'
        : 'Upstream usage'
    )
  }
  if (
    job.job_type === 'upstream_summary' ||
    job.job_type === 'upstream_details'
  ) {
    return translate('Upstream usage')
  }
  return translate('Customer #{{id}}', { id: job.target_user_id })
}

function ExportJobCard(props: {
  job: CustomerExportJobView
  billingStatement?: boolean
  upstreamBilling?: boolean
  showCustomer?: boolean
  personalUsage?: boolean
  adminCustomerUsage?: boolean
  onCancel: () => void
  onResubmit: () => void
  busy: boolean
}) {
  const { t, i18n } = useTranslation()
  const { job } = props
  const usageRecords = job.job_type === 'usage_logs'
  const allStatements = job.job_type === 'customer_statements'
  const summary = job.job_type === 'statement_summary'
  const upstreamExport =
    job.job_type === 'upstream_details' || job.job_type === 'upstream_summary'
  const personalUsage =
    props.personalUsage &&
    job.job_type === 'usage_summary' &&
    job.user_id === job.target_user_id &&
    job.filters.usage_view === 'self'
  const adminCustomerUsage =
    props.adminCustomerUsage &&
    job.job_type === 'usage_summary' &&
    (job.filters.usage_view === 'customers' ||
      job.filters.usage_view === 'customer')
  const compactUsage = personalUsage || adminCustomerUsage
  const compact =
    props.upstreamBilling ||
    compactUsage ||
    (props.billingStatement && allStatements) ||
    (props.billingStatement &&
      (props.showCustomer || job.user_id === job.target_user_id) &&
      (summary ||
        job.job_type === 'statement_details' ||
        (props.showCustomer && usageRecords)))
  const locale = compact ? toIntlLocale(i18n.language) : undefined
  const dateFormat = new Intl.DateTimeFormat(locale, {
    timeZone: job.filters.timezone,
    dateStyle: 'short',
    timeStyle: 'short',
  })
  const type = exportTypeLabel(job, t)
  let title = type
  if (compactUsage) title = t('Usage statistics')
  if (adminCustomerUsage) {
    title = exportAudienceLabel(job, t)
    if (job.filters.usage_view === 'customers') {
      title = job.filters.usage_search
        ? t('Customer search results')
        : t('All customers')
    }
  }
  if (compact && !compactUsage) {
    title = t('{{month}} {{statement}}', {
      month: formatBillingMonth(job.filters.start_timestamp, locale),
      statement: type,
    })
  }
  const { start_timestamp, end_timestamp, token_id, model_name } = job.filters
  const usageRange = compactUsage
    ? new Intl.DateTimeFormat(locale, {
        timeZone: job.filters.timezone,
        dateStyle: 'medium',
      }).formatRange(start_timestamp * 1000, (end_timestamp - 1) * 1000)
    : undefined
  const usageKey =
    token_id === 0 ? t('No API Key') : t('API Key ID: {{id}}', { id: token_id })
  const files = job.artifact?.files
  const rowCount = { rows: formatInteger(job.artifact?.line_count) }
  let csvRows = t('CSV · {{rows}} rows', rowCount)
  if (summary || allStatements || job.job_type === 'upstream_summary') {
    csvRows = t('CSV · {{rows}} summary rows', rowCount)
  } else if (job.job_type === 'statement_details') {
    csvRows = t('CSV · {{rows}} billing entries', rowCount)
  }
  const expiresAt = job.expires_at ?? job.artifact?.expires_at
  const [expiredAt, setExpiredAt] = useState<number>()
  useEffect(() => {
    if (
      !compact ||
      job.status !== 'succeeded' ||
      !expiresAt ||
      !files?.length
    ) {
      return
    }
    const remaining = expiresAt * 1000 - Date.now()
    if (remaining <= 0) {
      setExpiredAt(expiresAt)
      return
    }
    const timer = window.setTimeout(() => setExpiredAt(expiresAt), remaining)
    return () => window.clearTimeout(timer)
  }, [compact, expiresAt, files?.length, job.status])

  const status =
    expiredAt != null && expiredAt === expiresAt && job.status === 'succeeded'
      ? 'expired'
      : job.status
  let statusLabel = exportStatusLabel(job, t)
  if (status === 'expired') statusLabel = t('Expired')
  if (compact && status === 'succeeded' && !files?.length) {
    statusLabel = t('No data')
  }
  const showAudience =
    !compact ||
    (!compactUsage &&
      (props.showCustomer || (props.upstreamBilling && !upstreamExport)))
  const upstreamName =
    job.filters.upstream?.group_name || job.filters.upstream?.url_key
  const cancelling = job.status === 'cancelling' || job.cancel_requested

  return (
    <article
      className='rounded-lg border p-3'
      aria-label={compact ? title : exportAudienceLabel(job, t)}
    >
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>{title}</h3>
        <Badge variant={exportStatusVariant(status)} className='shrink-0'>
          {statusLabel}
        </Badge>
      </div>
      <div className='text-muted-foreground mt-1 space-y-0.5 text-xs'>
        {showAudience && (
          <p className='font-medium'>{exportAudienceLabel(job, t)}</p>
        )}
        {!compact && (
          <>
            <p>
              {dateFormat.format(job.filters.start_timestamp * 1000)} ~{' '}
              {dateFormat.format((job.filters.end_timestamp - 1) * 1000)} ·{' '}
              {job.filters.timezone}
            </p>
            <p>
              {t('Created {{time}}', {
                time: formatTimestampToDate(job.created_at),
              })}
            </p>
          </>
        )}
        {upstreamName && <p className='break-all'>{upstreamName}</p>}
        {compactUsage ? (
          <p className='wrap-anywhere'>
            {usageRange}
            {job.filters.usage_view !== 'customers' &&
              ` · ${model_name || t('All models')}`}
            {job.filters.usage_search && ` · ${job.filters.usage_search}`}
            {token_id != null && ` · ${usageKey}`}
          </p>
        ) : (
          <ExportScope job={job} compact={compact} />
        )}
        {!compact &&
          (job.filters.model_name || job.job_type === 'usage_summary') && (
            <p>
              {t('Model')}: {job.filters.model_name || t('All models')}
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
        {compact &&
          status === 'succeeded' &&
          job.artifact &&
          !!files?.length && (
            <>
              <p>{csvRows}</p>
              <p>
                {t('Generated {{time}}', {
                  time: dateFormat.format(job.artifact.generated_at * 1000),
                })}
                {expiresAt
                  ? ` · ${t('Downloadable until {{time}}', { time: dateFormat.format(expiresAt * 1000) })}`
                  : ''}
              </p>
            </>
          )}
        {!compact &&
          status === 'succeeded' &&
          job.artifact &&
          !!files?.length && (
            <p>
              {t('Historical export generated {{time}} · {{count}} records', {
                time: formatTimestampToDate(job.artifact.generated_at),
                count: formatInteger(job.artifact.line_count),
              })}
            </p>
          )}
        {status === 'succeeded' && !files?.length && (
          <p>{t('No records matched this export scope.')}</p>
        )}
        {!compact &&
          status === 'succeeded' &&
          job.expires_at != null &&
          job.expires_at > 0 && (
            <p>
              {t('Downloadable until {{time}}', {
                time: formatTimestampToDate(job.expires_at),
              })}
            </p>
          )}
        {job.status === 'failed' && (
          <p className='text-destructive'>
            {t(exportFailureMessage(job.error_code))}
          </p>
        )}
        {compact && !compactUsage && status === 'expired' && (
          <p>{t('A new export may include updated billing data.')}</p>
        )}
      </div>
      <div className='mt-2 flex justify-end gap-2 empty:hidden'>
        {(job.status === 'queued' ||
          job.status === 'running' ||
          job.status === 'cancelling') && (
          <Button
            variant='outline'
            size='sm'
            disabled={cancelling || props.busy}
            onClick={props.onCancel}
          >
            {cancelling ? t('Cancelling') : t('Cancel')}
          </Button>
        )}
        {(status === 'failed' || status === 'expired') && (
          <Button
            size='sm'
            variant='outline'
            disabled={props.busy}
            onClick={props.onResubmit}
          >
            {compact
              ? t('Export again with the same filters')
              : t('Regenerate with original filters')}
          </Button>
        )}
      </div>
      {status === 'succeeded' &&
        !!files?.length &&
        (compact ? (
          <ExportArtifactFiles
            key={job.job_id}
            jobId={job.job_id}
            files={files}
          />
        ) : (
          <ExportFiles jobId={job.job_id} />
        ))}
    </article>
  )
}

function exportFailureMessage(code?: string): string {
  switch (code) {
    case 'format_changed':
      return 'Export format changed. Generate a new file using the current format.'
    case 'budget_exceeded':
      return 'Export exceeded the processing limit. Try a smaller scope.'
    case 'queue_timeout':
      return 'Queue wait budget exceeded.'
    case 'storage_unavailable':
      return 'Export storage is unavailable. Contact an administrator before retrying.'
    case 'authorization_or_lease':
      return 'Export access or execution expired. Check your permissions before retrying.'
    default:
      return 'Export failed. You can submit it again later.'
  }
}

function ExportScope({
  job,
  compact,
}: {
  job: CustomerExportJobView
  compact?: boolean
}) {
  const { t } = useTranslation()
  const f = job.filters
  const fields = [
    [t('Username'), f.username],
    [
      t('Channel ID'),
      f.channel_id ?? (f.upstream?.channel_ids ?? []).join(', '),
    ],
    [t('Channel'), f.upstream?.all_channels ? t('All channels') : undefined],
    [t('Upstream URL'), f.usage_url_key],
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
    [t('Language'), compact ? undefined : f.language],
    [t('Currency'), compact ? undefined : f.currency],
  ].filter(([, value]) => value != null && value !== '')
  if (compact && f.token_id == null && fields.length === 0) return null
  return (
    <Collapsible>
      <CollapsibleTrigger
        render={<Button variant='ghost' size='sm' className='group' />}
      >
        {compact ? t('Filters') : t('Export scope')}
        {compact && (
          <HugeiconsIcon
            icon={ArrowDown01Icon}
            aria-hidden='true'
            className='transition-transform group-aria-expanded:rotate-180'
          />
        )}
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
