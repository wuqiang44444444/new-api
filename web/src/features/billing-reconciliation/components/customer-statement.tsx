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
  ArrowLeft01Icon,
  ArrowDown01Icon,
  ArrowRight01Icon,
  LinkSquare02Icon,
  Download01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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

import { getAdminCustomerStatement, getSelfCustomerStatement } from '../api'
import { estimateFailureLabel, estimateReasonText } from '../estimate-display'
import {
  createAdminExport,
  createSelfExport,
  requestExportDownload,
  type CustomerExportJobView,
} from '../export-api'
import { ExportJobsDrawer } from '../export-jobs-drawer'
import {
  billingModeLabel,
  formatCustomerStatementQuota as formatCurrentBalance,
  combinedDiscountFactor,
  formatDiscountFactor,
  formatDiscountTier,
  formatInteger,
} from '../lib'
import type {
  BillingMode,
  BillingDimension,
  BillingDiscountCombination,
  CustomerModelSummary,
} from '../types'
import {
  getAdminVersionDownload,
  getSelfVersionDownload,
  type BillingStatementVersionInfo,
} from '../version-api'
import { formatVersionQuota } from '../version-money'
import { BillingQualityNotice } from './billing-quality-notice'
import { StatementVersionPanel } from './statement-version-panel'
import { VersionLinesDrawer } from './version-lines-drawer'

type CustomerStatementProps = {
  isAdmin: boolean
  period: { start_timestamp: number; end_timestamp: number }
  userId?: number
  dimension: BillingDimension
  toolbar?: ReactNode
  onBack?: () => void
  onDimensionChange: (dimension: BillingDimension) => void
}

// 版本绑定的汇总响应（方案 13）：data_source=billing_statement_version 时携带。
type VersionBoundEnvelope = {
  billing_version?: BillingStatementVersionInfo
}

export function CustomerStatementView(props: CustomerStatementProps) {
  return (
    <CustomerStatementBody
      key={`${props.userId}-${props.period.start_timestamp}-${props.period.end_timestamp}`}
      {...props}
    />
  )
}

function CustomerStatementBody(props: CustomerStatementProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set())
  const [exportDrawerOpen, setExportDrawerOpen] = useState(false)
  const [confirmExportOpen, setConfirmExportOpen] = useState(false)
  const [exportType, setExportType] = useState<
    'statement_summary' | 'statement_details' | 'usage_logs'
  >('statement_summary')
  const [previewVersionId, setPreviewVersionId] = useState<string | null>(null)
  const [linesDrawerOpen, setLinesDrawerOpen] = useState(false)
  const [linesFilter, setLinesFilter] = useState<
    | {
        channel_id?: number
        token_id?: number
        model_name?: string
        billing_mode?: string
      }
    | undefined
  >(undefined)
  const selectedUserId = props.userId

  // 合并入口先确认；服务端复用相同数据的已有账单，管理员始终绑定所选客户。
  const exportPayload = (
    jobType: 'statement_details' | 'statement_summary' | 'usage_logs'
  ) => ({
    job_type: jobType,
    start_timestamp: props.period.start_timestamp,
    end_timestamp: props.period.end_timestamp + 1,
  })
  const submitExport = (
    jobType: 'statement_details' | 'statement_summary' | 'usage_logs'
  ) => {
    return props.isAdmin && selectedUserId != null
      ? createAdminExport(selectedUserId, exportPayload(jobType))
      : createSelfExport(exportPayload(jobType))
  }
  const handleExportSubmitted = async (job: CustomerExportJobView) => {
    setConfirmExportOpen(false)
    if (job.status === 'succeeded') {
      const download = await requestExportDownload(job.job_id)
      if (download.emptyResult) {
        toast.info(t('No records to export.'))
      } else if (download.files.length === 1) {
        window.open(download.files[0].url, '_blank', 'noopener')
      } else {
        setExportDrawerOpen(true)
      }
      return
    }
    toast.success(t('Export is being prepared. Track it in export jobs.'))
    setExportDrawerOpen(true)
  }
  const handleExportFailed = (error: unknown) => {
    toast.error(
      error instanceof Error ? error.message : t('Unable to submit export.')
    )
  }
  const statementExport = useMutation({
    mutationFn: () => submitExport(exportType),
    onSuccess: handleExportSubmitted,
    onError: handleExportFailed,
  })
  const statementQuery = useQuery({
    queryKey: [
      'billing-customer-reconciliation',
      props.isAdmin,
      selectedUserId,
      props.dimension,
      props.period.start_timestamp,
      props.period.end_timestamp,
      previewVersionId,
    ],
    queryFn: async () => {
      if (props.isAdmin && selectedUserId == null) {
        throw new Error(t('Select customer'))
      }
      const versionParams = previewVersionId
        ? { version: previewVersionId }
        : {}
      const response = props.isAdmin
        ? await getAdminCustomerStatement({
            ...props.period,
            ...versionParams,
            user_id: selectedUserId as number,
            dimension: props.dimension,
          })
        : await getSelfCustomerStatement({ ...props.period, ...versionParams })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load customer billing.')
        )
      }
      return response.data as typeof response.data & VersionBoundEnvelope
    },
    enabled: !props.isAdmin || selectedUserId != null,
    staleTime: 30_000,
    retry: false,
  })

  const statement = statementQuery.data?.result
  const groups = statement?.groups ?? []
  const boundVersion = statementQuery.data?.billing_version ?? null
  const formatCustomerStatementQuota = (value: number | null | undefined) =>
    formatVersionQuota(value, boundVersion)
  const versionBound = boundVersion != null
  const versionDownload = useMutation({
    mutationFn: async (input: { draftPublicId: string; role?: string }) => {
      const response = props.isAdmin
        ? await getAdminVersionDownload(input.draftPublicId, input.role)
        : await getSelfVersionDownload(input.draftPublicId, input.role)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Download is not ready.'))
      }
      return response.data
    },
    onSuccess: (data) => {
      setConfirmExportOpen(false)
      window.open(data.url, '_blank', 'noopener')
    },
    onError: (error: Error) => toast.error(error.message),
  })

  let exportDescription = t(
    'An existing export will be reused when the data has not changed. Otherwise, a new export will be prepared. Continue?'
  )
  if (exportType === 'usage_logs') {
    exportDescription = t(
      'Export usage records for the selected billing month?'
    )
  } else if (versionBound) {
    exportDescription = t('Download the statement version currently displayed?')
  }

  return (
    <div className='space-y-3'>
      {props.isAdmin && props.onBack && (
        <Button variant='ghost' size='sm' onClick={props.onBack}>
          <HugeiconsIcon
            icon={ArrowLeft01Icon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Back to customer statements')}
        </Button>
      )}
      <div className='flex flex-wrap items-end justify-end gap-2'>
        {props.toolbar}
        {props.dimension === 'api_key' && (
          <Button
            variant='outline'
            disabled={
              statementQuery.isFetching ||
              statementQuery.isError ||
              !statement ||
              statementExport.isPending ||
              versionDownload.isPending
            }
            onClick={() => {
              setExportType('statement_summary')
              setConfirmExportOpen(true)
            }}
          >
            <HugeiconsIcon
              icon={Download01Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Export and download')}
          </Button>
        )}
        <Button variant='ghost' onClick={() => setExportDrawerOpen(true)}>
          {t('Export jobs')}
        </Button>
        {props.dimension === 'api_key' &&
          props.isAdmin &&
          selectedUserId != null && (
            <Button
              variant='ghost'
              disabled={
                statementQuery.isFetching ||
                statementQuery.isError ||
                !statement ||
                statementExport.isPending
              }
              onClick={() => {
                setExportType('usage_logs')
                setConfirmExportOpen(true)
              }}
            >
              {t('Export usage records')}
            </Button>
          )}
      </div>
      <ConfirmDialog
        open={confirmExportOpen}
        onOpenChange={setConfirmExportOpen}
        title={t('Confirm export and download')}
        desc={exportDescription}
        confirmText={t('Confirm export and download')}
        isLoading={statementExport.isPending || versionDownload.isPending}
        handleConfirm={() => {
          if (statementExport.isPending || versionDownload.isPending) return
          if (boundVersion && exportType !== 'usage_logs') {
            versionDownload.mutate({
              draftPublicId: boundVersion.draft_public_id,
              role:
                exportType === 'statement_details' ? 'detail_csv' : undefined,
            })
          } else {
            statementExport.mutate()
          }
        }}
      >
        {exportType !== 'usage_logs' && (
          <label className='space-y-1.5'>
            <span className='text-sm font-medium'>{t('Export content')}</span>
            <Select
              items={[
                { value: 'statement_summary', label: t('Statement summary') },
                { value: 'statement_details', label: t('Statement details') },
              ]}
              value={exportType}
              disabled={statementExport.isPending || versionDownload.isPending}
              onValueChange={(value) => {
                if (
                  value === 'statement_summary' ||
                  value === 'statement_details'
                ) {
                  setExportType(value)
                }
              }}
            >
              <SelectTrigger
                className='w-full'
                aria-label={t('Export content')}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value='statement_summary'>
                    {t('Statement summary')}
                  </SelectItem>
                  <SelectItem value='statement_details'>
                    {t('Statement details')}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-sm' aria-live='polite'>
              {exportType === 'statement_summary'
                ? t(
                    'One row per API Key and model: requests, net amount, estimated list price and savings.'
                  )
                : t(
                    'Per-call billing records: time, request ID, usage, amounts and discount factors. Charges and refunds are separate rows; refunds have negative net amounts.'
                  )}
            </p>
          </label>
        )}
      </ConfirmDialog>
      <Card size='sm'>
        <CardContent className='grid gap-3 md:grid-cols-3'>
          {props.isAdmin && statement && (
            <div className='space-y-1.5'>
              <div className='text-muted-foreground text-xs font-medium'>
                {t('Customer')}
              </div>
              <div className='font-medium'>
                {customerStatementLabel(statement)}
              </div>
              <div className='text-muted-foreground text-xs'>
                {t('Customer ID')}: {statement.user_id}
              </div>
            </div>
          )}
          {props.isAdmin && (
            <label className='space-y-1.5'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Aggregation dimension')}
              </span>
              <Select
                items={[
                  { value: 'api_key', label: t('API Key') },
                  { value: 'channel', label: t('Channel') },
                ]}
                value={props.dimension}
                disabled={statementQuery.isPending}
                onValueChange={(value) =>
                  value != null &&
                  props.onDimensionChange(value as BillingDimension)
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue>
                    {props.dimension === 'api_key'
                      ? t('API Key')
                      : t('Channel')}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent align='start'>
                  <SelectGroup>
                    <SelectItem value='api_key'>{t('API Key')}</SelectItem>
                    <SelectItem value='channel'>{t('Channel')}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </label>
          )}
          <div className='space-y-1.5 md:self-end'>
            <div className='text-muted-foreground text-xs font-medium'>
              {t('Authoritative source')}
            </div>
            <div className='flex h-8 items-center gap-2'>
              <Badge variant='outline'>{t('Database')}</Badge>
              <span className='text-muted-foreground text-xs'>
                {t('Asia/Shanghai settlement period')}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      <StatementVersionPanel
        isAdmin={props.isAdmin}
        userId={selectedUserId}
        period={props.period}
        previewVersionId={previewVersionId}
        onPreviewChange={setPreviewVersionId}
        onChanged={() => void statementQuery.refetch()}
      />

      {statementQuery.isError && (
        <ErrorState
          title={t('Unable to load customer billing')}
          description={
            statementQuery.error instanceof Error
              ? statementQuery.error.message
              : t('Please try again later.')
          }
          onRetry={() => statementQuery.refetch()}
        />
      )}
      {!statementQuery.isError && (statementQuery.isPending || !statement) && (
        <StatementSkeleton />
      )}
      {!statementQuery.isError && !statementQuery.isPending && statement && (
        <>
          {versionBound && boundVersion && (
            <div
              role='note'
              className='border-primary/40 bg-primary/5 rounded-lg border px-3 py-2 text-sm'
            >
              {boundVersion.status === 'confirmed'
                ? t(
                    'You are viewing confirmed version v{{version}}. Amounts are frozen at confirmation time.',
                    { version: boundVersion.version_number ?? '?' }
                  )
                : t(
                    'You are previewing a pending statement version. It is not published to the customer yet.'
                  )}
            </div>
          )}
          <BillingQualityNotice
            quality={statement.data_quality}
            estimateAvailable={
              statement.original_quota != null &&
              statement.discount_quota != null
            }
          />
          {props.dimension === 'api_key' &&
            statement.discount_combinations &&
            statement.discount_combinations.length > 0 && (
              <DiscountCombinationSection
                version={boundVersion}
                combinations={statement.discount_combinations}
                originalQuota={statement.original_quota}
                discountQuota={statement.discount_quota}
                estimateReasons={statement.estimate_reasons}
                netQuota={statement.summary.net_quota}
                groupNames={
                  new Map(
                    groups.map((group) => [
                      group.id,
                      { name: group.name, deleted: group.deleted },
                    ])
                  )
                }
              />
            )}
          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
            <SummaryCard
              title={t('Current balance')}
              value={formatCurrentBalance(statement.current_balance)}
              description={
                statement.current_balance === null
                  ? t(
                      'Customer record unavailable; current balance cannot be read.'
                    )
                  : t('Read directly from the main database')
              }
            />
            <SummaryCard
              title={t('Estimated list price')}
              value={
                statement.original_quota != null
                  ? formatCustomerStatementQuota(statement.original_quota)
                  : estimateFailureLabel(statement.estimate_reasons, t)
              }
              description={
                statement.original_quota != null
                  ? t(
                      'Estimated from recorded charges and historical discounts'
                    )
                  : estimateReasonText(statement.estimate_reasons, t)
              }
            />
            <SummaryCard
              title={t('Estimated savings')}
              value={
                statement.discount_quota != null
                  ? formatCustomerStatementQuota(statement.discount_quota)
                  : estimateFailureLabel(statement.estimate_reasons, t)
              }
              description={
                statement.discount_quota != null
                  ? t('Estimated list price minus net settled amount')
                  : estimateReasonText(statement.estimate_reasons, t)
              }
            />
            <SummaryCard
              title={t('Net settled amount')}
              value={formatCustomerStatementQuota(statement.summary.net_quota)}
              description={t('{{count}} aggregated requests', {
                count: statement.summary.requests,
              })}
            />
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'List price and savings are estimates reconstructed from rounded charges and historical discounts. Net amounts include task holds and adjustments.'
            )}
          </p>

          <Card>
            <CardHeader>
              <CardTitle>{t('Customer billing summary')}</CardTitle>
              <CardDescription>
                {t(
                  'Expand a group to inspect model totals. Request rows remain in usage logs.'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='px-0'>
              {groups.length === 0 ? (
                <div className='text-muted-foreground px-4 py-12 text-center'>
                  {t('No settled usage in this billing period.')}
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-10' />
                      <TableHead>
                        {props.dimension === 'api_key'
                          ? t('API Key')
                          : t('Channel')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Requests')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Estimated list price')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Estimated savings')}
                      </TableHead>
                      <TableHead className='text-right'>
                        {t('Net amount')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groups.map((group) => {
                      const isExpanded = expanded.has(group.id)
                      const fallbackName =
                        props.dimension === 'channel'
                          ? t('Channel #{{id}}', { id: group.id })
                          : t('API Key #{{id}}', { id: group.id })
                      return [
                        <TableRow key={`group-${group.id}`}>
                          <TableCell>
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              aria-label={
                                isExpanded
                                  ? t('Collapse models')
                                  : t('Expand models')
                              }
                              aria-expanded={isExpanded}
                              onClick={() =>
                                setExpanded((current) =>
                                  toggleSet(current, group.id)
                                )
                              }
                            >
                              <HugeiconsIcon
                                icon={
                                  isExpanded
                                    ? ArrowDown01Icon
                                    : ArrowRight01Icon
                                }
                                strokeWidth={2}
                              />
                            </Button>
                          </TableCell>
                          <TableCell>
                            <div className='font-medium'>
                              {group.deleted ? fallbackName : group.name}
                            </div>
                            <div
                              data-table-text='secondary'
                              className='text-muted-foreground max-w-sm text-xs whitespace-normal'
                            >
                              {group.deleted
                                ? t(
                                    'Record unavailable; historical charges are retained.'
                                  )
                                : `#${group.id}`}
                            </div>
                          </TableCell>
                          <TableCell className='text-right'>
                            {formatInteger(group.usage.requests)}
                          </TableCell>
                          <TableCell className='text-right'>
                            {combinationAmountCell(
                              group.original_quota,
                              t,
                              boundVersion,
                              group.estimate_reasons
                            )}
                          </TableCell>
                          <TableCell className='text-right'>
                            {combinationAmountCell(
                              group.discount_quota,
                              t,
                              boundVersion,
                              group.estimate_reasons
                            )}
                          </TableCell>
                          <TableCell className='text-right font-medium'>
                            {formatCustomerStatementQuota(
                              group.usage.net_quota
                            )}
                          </TableCell>
                        </TableRow>,
                        ...(isExpanded
                          ? group.models.map((model) => (
                              <CustomerModelRow
                                version={boundVersion}
                                key={`model-${group.id}-${model.model_name}-${model.billing_mode}`}
                                model={model}
                                dimension={props.dimension}
                                userId={statement.user_id}
                                versionBound={versionBound}
                                onViewDetails={
                                  versionBound
                                    ? (filter) => {
                                        setLinesFilter(filter)
                                        setLinesDrawerOpen(true)
                                      }
                                    : undefined
                                }
                              />
                            ))
                          : []),
                      ]
                    })}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </>
      )}
      <ExportJobsDrawer
        open={exportDrawerOpen}
        onOpenChange={setExportDrawerOpen}
      />
      <VersionLinesDrawer
        key={JSON.stringify([boundVersion?.draft_public_id, linesFilter])}
        open={linesDrawerOpen}
        onOpenChange={(open) => {
          if (!open) {
            setLinesFilter(undefined)
          }
          setLinesDrawerOpen(open)
        }}
        version={boundVersion}
        isAdmin={props.isAdmin}
        initialFilter={linesFilter}
      />
    </div>
  )
}

function CustomerModelRow(props: {
  version?: BillingStatementVersionInfo | null
  model: CustomerModelSummary
  dimension: BillingDimension
  userId: number
  versionBound?: boolean
  onViewDetails?: (filter: {
    channel_id?: number
    token_id?: number
    model_name?: string
    billing_mode?: string
  }) => void
}) {
  const formatCustomerStatementQuota = (value: number | null | undefined) =>
    formatVersionQuota(value, props.version)
  const { t } = useTranslation()
  const filter = props.model.detail_filter
  const params = new URLSearchParams({
    billing: 'true',
    billingUserId: String(props.userId),
    billingMode: props.model.billing_mode,
    model: props.model.model_name,
    startTime: String(filter.start_timestamp * 1000),
    endTime: String(filter.end_timestamp * 1000),
  })
  if (props.dimension === 'channel') {
    params.set('channel', String(filter.channel_id ?? ''))
  }
  if (props.dimension === 'api_key' && filter.token_id != null) {
    params.set('tokenId', String(filter.token_id))
  }
  const detailHref = `/usage-logs/common?${params.toString()}`
  const usage = props.model.usage
  const versionDetailFilter = {
    ...(props.dimension === 'channel' ? { channel_id: filter.channel_id } : {}),
    ...(props.dimension === 'api_key' && filter.token_id != null
      ? { token_id: filter.token_id }
      : {}),
    model_name: props.model.model_name,
    billing_mode: props.model.billing_mode,
  }
  return (
    <TableRow className='bg-muted/20'>
      <TableCell />
      <TableCell>
        <div className='pl-4'>
          <div className='flex items-center gap-2'>
            <span>{props.model.model_name || t('Unknown model')}</span>
            <Badge variant='secondary'>
              {t(billingModeLabel(props.model.billing_mode))}
            </Badge>
          </div>
          {(props.model.billing_mode === 'token' ||
            props.model.billing_mode === 'per_call') && (
            <div className='text-muted-foreground mt-1 text-xs'>
              {props.model.billing_mode === 'token'
                ? t(
                    'Input {{input}} · Cache read {{cacheRead}} · Cache write {{cacheWrite}} · Output {{output}}',
                    {
                      input: props.model.data_quality
                        ?.input_tokens_unavailable_requests
                        ? t('Unknown')
                        : formatInteger(usage.input_tokens),
                      cacheRead: formatInteger(usage.cache_read_tokens),
                      cacheWrite: formatInteger(usage.cache_write_tokens),
                      output: formatInteger(usage.output_tokens),
                    }
                  )
                : t('Billable {{billable}} · Refunded {{refunded}}', {
                    billable: formatInteger(usage.billable_calls),
                    refunded: formatInteger(usage.refunded_calls),
                  })}
            </div>
          )}
        </div>
      </TableCell>
      <TableCell className='text-right'>
        {formatInteger(usage.requests)}
      </TableCell>
      <TableCell className='text-right'>
        {combinationAmountCell(
          props.model.original_quota,
          t,
          props.version,
          props.model.estimate_reasons
        )}
      </TableCell>
      <TableCell className='text-right'>
        {combinationAmountCell(
          props.model.discount_quota,
          t,
          props.version,
          props.model.estimate_reasons
        )}
      </TableCell>
      <TableCell className='text-right'>
        <div className='font-medium'>
          {formatCustomerStatementQuota(usage.net_quota)}
        </div>
        {props.versionBound && props.onViewDetails ? (
          <Button
            variant='link'
            size='xs'
            onClick={() => props.onViewDetails?.(versionDetailFilter)}
          >
            {t('View details')}
            <HugeiconsIcon
              icon={LinkSquare02Icon}
              strokeWidth={2}
              data-icon='inline-end'
            />
          </Button>
        ) : (
          <Button
            variant='link'
            size='xs'
            nativeButton={false}
            render={<a href={detailHref} />}
          >
            {t('View details')}
            <HugeiconsIcon
              icon={LinkSquare02Icon}
              strokeWidth={2}
              data-icon='inline-end'
            />
          </Button>
        )}
      </TableCell>
    </TableRow>
  )
}

function SummaryCard(props: {
  title: string
  value: string
  description: string
}) {
  return (
    <Card size='sm'>
      <CardHeader>
        <CardDescription>{props.title}</CardDescription>
        <CardTitle className='text-xl tabular-nums'>{props.value}</CardTitle>
      </CardHeader>
      <CardContent className='text-muted-foreground text-xs'>
        {props.description}
      </CardContent>
    </Card>
  )
}

function StatementSkeleton() {
  return (
    <div className='space-y-3'>
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {[0, 1, 2, 3].map((item) => (
          <Skeleton key={item} className='h-28 rounded-xl' />
        ))}
      </div>
      <Skeleton className='h-72 rounded-xl' />
    </div>
  )
}

function DiscountCombinationSection(props: {
  version?: BillingStatementVersionInfo | null
  combinations: BillingDiscountCombination[]
  originalQuota?: number
  discountQuota?: number
  estimateReasons?: string[]
  netQuota: number
  groupNames: Map<number, { name: string; deleted?: boolean }>
}) {
  const formatCustomerStatementQuota = (value: number | null | undefined) =>
    formatVersionQuota(value, props.version)
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const combinations = props.combinations
  const visible = expanded ? combinations : combinations.slice(0, 5)
  const collapsed = combinations.length - visible.length
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Discount breakdown')}</CardTitle>
        <CardDescription>
          {t(
            'One row per discount combination actually applied this period; totals reconcile with the summary cards.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='px-0'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Combination scope')}</TableHead>
              <TableHead>{t('Group discount/ratio')}</TableHead>
              <TableHead>{t('Contract discount')}</TableHead>
              <TableHead>{t('Final discount')}</TableHead>
              <TableHead className='text-right'>
                {t('Estimated list price')}
              </TableHead>
              <TableHead className='text-right'>
                {t('Estimated savings')}
              </TableHead>
              <TableHead className='text-right'>{t('Net amount')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.map((combo) => (
              <TableRow
                key={JSON.stringify([
                  combo.other,
                  combo.group_id,
                  combo.model_name,
                  combo.billing_mode,
                  combo.group_ratio,
                  combo.group_name,
                  combo.group_ratio_source,
                  combo.contract_applicable,
                  combo.contract_ratio,
                  combo.contract_id_known,
                  combo.contract_id,
                  combo.contract_version,
                ])}
              >
                <TableCell>
                  {combinationScopeCell(combo, props.groupNames, t)}
                </TableCell>
                <TableCell>{combinationGroupFactorCell(combo, t)}</TableCell>
                <TableCell>{combinationContractCell(combo, t)}</TableCell>
                <TableCell>
                  <span className='inline-block max-w-56 whitespace-normal'>
                    {combinationFinalFactorCell(combo, t)}
                  </span>
                </TableCell>
                <TableCell className='text-right'>
                  {combinationAmountCell(
                    combo.original_quota,
                    t,
                    props.version,
                    combo.estimate_reasons
                  )}
                </TableCell>
                <TableCell className='text-right'>
                  {combinationAmountCell(
                    combo.discount_quota,
                    t,
                    props.version,
                    combo.estimate_reasons
                  )}
                </TableCell>
                <TableCell className='text-right font-medium'>
                  {formatCustomerStatementQuota(combo.usage.net_quota)}
                </TableCell>
              </TableRow>
            ))}
            <TableRow className='bg-muted/30'>
              <TableCell colSpan={4} className='font-medium'>
                {t('Total')}
              </TableCell>
              <TableCell className='text-right font-medium'>
                {combinationAmountCell(
                  props.originalQuota,
                  t,
                  props.version,
                  props.estimateReasons
                )}
              </TableCell>
              <TableCell className='text-right font-medium'>
                {combinationAmountCell(
                  props.discountQuota,
                  t,
                  props.version,
                  props.estimateReasons
                )}
              </TableCell>
              <TableCell className='text-right font-medium'>
                {formatCustomerStatementQuota(props.netQuota)}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        {collapsed > 0 && (
          <div className='px-4 pt-2'>
            <Button variant='ghost' size='sm' onClick={() => setExpanded(true)}>
              {t('Show {{count}} more combinations', { count: collapsed })}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function combinationScopeCell(
  combo: BillingDiscountCombination,
  groupNames: Map<number, { name: string; deleted?: boolean }>,
  translate: (key: string, opts?: Record<string, unknown>) => string
) {
  if (combo.other) {
    return translate('Other combinations')
  }
  const group = groupNames.get(combo.group_id)
  const groupLabel = group ? group.name : `#${combo.group_id}`
  return (
    <div className='flex max-w-xs flex-col'>
      <span className='truncate'>
        {combo.model_name || translate('Unknown model')}
      </span>
      <span className='text-muted-foreground text-xs'>
        {groupLabel} ·{' '}
        {translate(billingModeLabel(combo.billing_mode as BillingMode))}
      </span>
    </div>
  )
}

function combinationGroupFactorCell(
  combo: BillingDiscountCombination,
  translate: (key: string, opts?: Record<string, unknown>) => string
) {
  if (combo.other) return '—'
  let sourceLabel = translate('Not recorded')
  if (combo.group_ratio_source === 'user_exclusive') {
    sourceLabel = translate('User Exclusive Ratio')
  } else if (combo.group_ratio_source === 'group') {
    sourceLabel = translate('Group Ratio')
  }
  return (
    <div className='flex flex-col'>
      <span>
        {combo.group_name || translate('Historical identity not recorded')}
      </span>
      <span className='text-muted-foreground text-xs'>{sourceLabel}</span>
      <span className='tabular-nums'>
        {combo.group_ratio == null ? (
          translate('Not recorded')
        ) : (
          <>
            {combo.group_ratio < 1 && (
              <span>{formatDiscountTier(combo.group_ratio, translate)} </span>
            )}
            <span className='text-muted-foreground'>
              {formatDiscountFactor(combo.group_ratio)}
            </span>
          </>
        )}
      </span>
    </div>
  )
}

function combinationContractCell(
  combo: BillingDiscountCombination,
  translate: (key: string, opts?: Record<string, unknown>) => string
) {
  if (combo.other) {
    return '—'
  }
  if (combo.contract_applicable === 'no') {
    return translate('No contract discount applied')
  }
  if (
    combo.contract_applicable === 'unknown' ||
    combo.contract_applicable === 'unrecorded'
  ) {
    return translate('Not recorded')
  }
  let label: string
  if (combo.contract_name) {
    label = combo.contract_name
  } else if (combo.contract_id_known) {
    label = translate('Contract #{{id}}', { id: combo.contract_id })
  } else {
    label = translate('Historical identity not recorded')
  }
  return (
    <div className='flex flex-col'>
      <span className='truncate'>{label}</span>
      {combo.contract_ratio != null && (
        <span className='text-muted-foreground text-xs tabular-nums'>
          {combo.contract_ratio < 1 && (
            <>{formatDiscountTier(combo.contract_ratio, translate)} </>
          )}
          {formatDiscountFactor(combo.contract_ratio)}
        </span>
      )}
    </div>
  )
}

function combinationFinalFactorCell(
  combo: BillingDiscountCombination,
  translate: (key: string, opts?: Record<string, unknown>) => string
) {
  if (combo.other) return translate('Not applicable to a combined row')
  if (combo.estimate_reasons?.includes('invalid_facts')) {
    return translate('Unknown: invalid billing records')
  }
  if (combo.group_ratio == null) {
    return translate('Unknown: historical group ratio not recorded')
  }
  if (combo.contract_applicable === 'unknown') {
    return translate('Unknown: historical contract status not recorded')
  }
  const contractFactor =
    combo.contract_applicable === 'yes' ? (combo.contract_ratio ?? null) : 1
  const final = combinedDiscountFactor(combo.group_ratio, contractFactor)
  if (final == null || !Number.isFinite(final)) {
    return translate('Unknown: invalid billing records')
  }
  if (final === 1) return translate('No discount (×1)')
  return (
    <span className='tabular-nums'>
      {final < 1 && <>{formatDiscountTier(final, translate)} </>}
      <span className='text-muted-foreground'>
        {formatDiscountFactor(final)}
      </span>
    </span>
  )
}

function combinationAmountCell(
  value: number | undefined,
  translate: (key: string) => string,
  version?: BillingStatementVersionInfo | null,
  reasons?: string[]
) {
  if (value != null) return formatVersionQuota(value, version)
  return (
    <span className='inline-block max-w-64 whitespace-normal'>
      {estimateReasonText(reasons, translate)}
    </span>
  )
}

function customerStatementLabel(statement: {
  username: string
  display_name: string
}) {
  const displayName = statement.display_name?.trim()
  return displayName
    ? `${displayName} (${statement.username})`
    : statement.username
}

function toggleSet(current: Set<number>, value: number) {
  const next = new Set(current)
  if (next.has(value)) next.delete(value)
  else next.add(value)
  return next
}
