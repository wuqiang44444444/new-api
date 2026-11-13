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
  Activity01Icon,
  ArrowLeft01Icon,
  ArrowDown01Icon,
  ArrowRight01Icon,
  Discount01Icon,
  Invoice01Icon,
  LinkSquare02Icon,
  Download01Icon,
  ReceiptDollarIcon,
  Wallet01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { ReportCard, ReportMetricCard } from '@/components/report-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
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
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useResultPage } from '@/hooks/use-result-page'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import { getAdminCustomerStatement, getSelfCustomerStatement } from '../api'
import {
  createAdminExport,
  createSelfExport,
  downloadExportFile,
} from '../export-api'
import { ExportJobsDrawer } from '../export-jobs-drawer'
import {
  billingModeLabel,
  formatBillingMonth,
  formatCustomerStatementQuota as formatCurrentBalance,
  formatInteger,
} from '../lib'
import { statementWithRecordedAmounts } from '../recorded-amounts'
import type { BillingDimension, CustomerModelSummary } from '../types'
import { useBillingExportJob } from '../use-billing-export-job'
import {
  getAdminVersionDownload,
  getSelfVersionDownload,
  type BillingStatementVersionInfo,
} from '../version-api'
import { formatVersionQuota } from '../version-money'
import { BillingExportSection } from './billing-export-section'
import { BillingPagination } from './billing-pagination'
import { BillingQualityNotice } from './billing-quality-notice'
import { BillingReportStatus } from './billing-report-status'
import { CustomerBillingTable } from './customer-billing-table'
import { CustomerKeyBillingTotal } from './customer-key-billing-total'
import { CustomerModelBilling } from './customer-model-billing'
import { CustomerModelDiscounts } from './customer-model-discounts'
import { MyBillingHeader } from './my-billing-header'
import { StatementVersionPanel } from './statement-version-panel'
import { VersionLinesDrawer } from './version-lines-drawer'

type CustomerStatementProps = {
  reportVersion?: string
  isAdmin: boolean
  period: { start_timestamp: number; end_timestamp: number }
  userId?: number
  dimension: BillingDimension
  toolbar?: ReactNode
  heading?: string
  modelKeyId?: number
  onModelKeyChange?: (id?: number) => void
  onBack?: () => void
  onDimensionChange: (dimension: BillingDimension) => void
}

// 版本绑定的汇总响应（方案 13）：data_source=billing_statement_version 时携带。
type VersionBoundEnvelope = {
  billing_version?: BillingStatementVersionInfo
  pending_refund_count?: number
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
  const { t, i18n } = useTranslation()
  const byApiKey = props.dimension === 'api_key'
  const [adminModelKeyId, setAdminModelKeyId] = useState<number>()
  const exportContentId = useId()
  const modelBillId = useId()
  const keySummaryHeadingId = useId()
  const modelBillHeadingRef = useRef<HTMLHeadingElement>(null)
  const showModelBilling = (keyId?: number) => {
    if (props.isAdmin) setAdminModelKeyId(keyId)
    else props.onModelKeyChange?.(keyId)
    modelBillHeadingRef.current?.focus({ preventScroll: true })
    modelBillHeadingRef.current?.scrollIntoView?.({ block: 'start' })
  }
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set())
  const [expandScrollTarget, setExpandScrollTarget] = useState<number | null>(
    null
  )
  const firstModelRowRef = useRef<HTMLTableRowElement | null>(null)
  // 分组展开后模型行可能落在封顶滚动容器的折叠区内，把首行滚入视野。
  useEffect(() => {
    if (expandScrollTarget == null) return
    firstModelRowRef.current?.scrollIntoView({ block: 'nearest' })
    setExpandScrollTarget(null)
  }, [expandScrollTarget])
  const [exportDrawerOpen, setExportDrawerOpen] = useState(false)
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

  // 服务端复用相同数据的已有账单，管理员始终绑定所选客户。
  const exportPayload = (
    jobType: 'statement_details' | 'statement_summary' | 'usage_logs'
  ) => ({
    job_type: jobType,
    start_timestamp: props.period.start_timestamp,
    end_timestamp: props.period.end_timestamp + 1,
    ...(jobType !== 'usage_logs' && report
      ? { report_version: report.version }
      : {}),
  })
  const submitExport = () => {
    const payload = exportPayload(exportType)
    if (!props.isAdmin) return createSelfExport(payload)
    if (selectedUserId == null) throw new Error(t('Select customer'))
    return createAdminExport(selectedUserId, payload)
  }
  const statementExport = useBillingExportJob(submitExport)
  const statementQuery = useQuery({
    queryKey: [
      'billing-customer-reconciliation',
      props.isAdmin,
      selectedUserId,
      props.dimension,
      props.period.start_timestamp,
      props.period.end_timestamp,
      previewVersionId,
      ...(props.reportVersion ? [props.reportVersion] : []),
    ],
    queryFn: async () => {
      if (props.isAdmin && selectedUserId == null) {
        throw new Error(t('Select customer'))
      }
      const statementParams = {
        ...props.period,
        version: previewVersionId ?? undefined,
        include_pending_refunds: true,
        ...(props.reportVersion ? { report_version: props.reportVersion } : {}),
      }
      const response = props.isAdmin
        ? await getAdminCustomerStatement({
            ...statementParams,
            user_id: selectedUserId as number,
            dimension: props.dimension,
          })
        : await getSelfCustomerStatement(statementParams)
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

  const report = statementQuery.data?.billing_report
  const statement = useMemo(
    () => statementWithRecordedAmounts(statementQuery.data?.result),
    [statementQuery.data?.result]
  )
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
    onSuccess: (data) => downloadExportFile(data.url),
    onError: (error: Error) => toast.error(error.message),
  })

  const downloadBoundVersion = versionBound && exportType !== 'usage_logs'
  const exportBusy = statementExport.isPending || versionDownload.isPending
  const statementUnavailable = statementQuery.isError || !statement
  const exportDisabled =
    statementQuery.isFetching || statementUnavailable || exportBusy
  const handleStatementExport = () => {
    if (exportDisabled) return
    if (boundVersion && exportType !== 'usage_logs') {
      versionDownload.mutate({
        draftPublicId: boundVersion.draft_public_id,
        role: exportType === 'statement_details' ? 'detail_csv' : undefined,
      })
    } else {
      statementExport.mutate()
    }
  }
  const exportOptions = [
    { value: 'statement_summary', label: t('Statement summary') },
    { value: 'statement_details', label: t('Statement details') },
    ...(props.isAdmin
      ? [{ value: 'usage_logs', label: t('Usage records') }]
      : []),
  ] as const
  const exportHelp = {
    statement_summary: t('Totals grouped by API Key, model, and billing mode.'),
    statement_details: t(
      'Per-call billing records: time, request ID, usage, amounts and discount factors. Charges and refunds are separate rows; refunds have negative net amounts.'
    ),
    usage_logs: t('Export usage records for the selected billing month.'),
  }
  const exportContent = (
    <Field>
      <FieldLabel htmlFor={exportContentId}>{t('Export content')}</FieldLabel>
      <Select
        items={exportOptions}
        value={exportType}
        disabled={exportDisabled}
        onValueChange={(value) => {
          if (value) setExportType(value)
        }}
      >
        <SelectTrigger
          id={exportContentId}
          className='w-full'
          aria-label={t('Export content')}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {exportOptions.map(({ value, label }) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      <p className='text-muted-foreground text-sm' aria-live='polite'>
        {exportHelp[exportType]}
      </p>
    </Field>
  )
  let exportLabel = downloadBoundVersion ? t('Download CSV') : t('Export CSV')
  if (exportBusy) exportLabel = t('Loading...')
  let exportNotice: string | undefined
  if (statementQuery.isFetching) {
    exportNotice = t('Loading statement...')
  } else if (statementUnavailable) {
    exportNotice = t('The current statement is unavailable.')
  } else if (!downloadBoundVersion && exportType !== 'usage_logs') {
    exportNotice = t(
      'A matching export is reused when its file is still available.'
    )
  }

  let heading = props.heading ?? t('My billing')
  if (props.isAdmin) {
    heading = statement
      ? customerStatementLabel(statement)
      : t('Customer billing')
  }
  const actions = (
    <>
      {props.toolbar}
      <Button
        variant={props.isAdmin ? 'outline' : 'default'}
        size={props.isAdmin ? 'sm' : 'default'}
        aria-haspopup='dialog'
        onClick={() => {
          setExportType('statement_summary')
          statementExport.clearFocus()
          setExportDrawerOpen(true)
        }}
      >
        <HugeiconsIcon
          icon={Download01Icon}
          strokeWidth={2}
          data-icon='inline-start'
        />
        {t('Export and download')}
      </Button>
    </>
  )
  const reportStatus = <BillingReportStatus report={report} />
  const pendingRefund = statementQuery.data?.pending_refund_count ? (
    <p className='text-sm font-medium'>
      {t('Pending refund')} · {statementQuery.data.pending_refund_count}
    </p>
  ) : null
  const versionNotice =
    versionBound && boundVersion ? (
      <div role='note' className='text-sm font-medium'>
        {boundVersion.status === 'confirmed'
          ? t(
              'You are viewing confirmed version v{{version}}. Amounts are frozen at confirmation time.',
              { version: boundVersion.version_number ?? '?' }
            )
          : t(
              'You are previewing a pending statement version. It is not published to the customer yet.'
            )}
      </div>
    ) : null
  const qualityNotice =
    !statementQuery.isError && statement ? (
      <BillingQualityNotice inline quality={statement.data_quality} />
    ) : null

  const modelKeyId = props.isAdmin ? adminModelKeyId : props.modelKeyId
  const selectedModelKey = groups.find((group) => group.id === modelKeyId)
  const modelGroups = selectedModelKey ? [selectedModelKey] : groups
  const modelRows = byApiKey
    ? modelGroups.flatMap((group) =>
        group.models.map((model) => ({ group, model }))
      )
    : []
  const groupPaging = useResultPage(groups.length, props.dimension)
  const modelPaging = useResultPage(modelRows.length, selectedModelKey?.id)
  const visibleGroups = groups.slice(
    groupPaging.offset,
    groupPaging.offset + groupPaging.pageSize
  )
  const visibleModelRows = modelRows.slice(
    modelPaging.offset,
    modelPaging.offset + modelPaging.pageSize
  )
  const hasModelRows = Boolean(
    byApiKey && groups.some((group) => group.models.length > 0)
  )
  const showModelPrices = modelGroups.some((group) =>
    group.models.some(
      (model) => model.original_quota != null || model.discount_quota != null
    )
  )
  const modelTotals = selectedModelKey ?? statement
  const modelTotalUsage = selectedModelKey?.usage ?? statement?.summary
  const openVersionLines = (filter: NonNullable<typeof linesFilter>) => {
    setLinesFilter(filter)
    setLinesDrawerOpen(true)
  }

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='@container min-h-0 flex-1 space-y-3 overflow-y-auto px-2 pb-4 sm:px-3'>
        <StatementVersionPanel
          isAdmin={props.isAdmin}
          userId={selectedUserId}
          period={props.period}
          statementDataCutoff={
            report?.published_days
              ? (report.estimated_at ?? report.as_of)
              : undefined
          }
          previewVersionId={previewVersionId}
          onPreviewChange={setPreviewVersionId}
          onChanged={() => void statementQuery.refetch()}
          renderHeader={(versionStatus, versionActions) => (
            <MyBillingHeader
              heading={heading}
              backAction={
                props.onBack && (
                  <Button variant='ghost' size='sm' onClick={props.onBack}>
                    <HugeiconsIcon
                      icon={ArrowLeft01Icon}
                      strokeWidth={2}
                      data-icon='inline-start'
                    />
                    {t('Back to customer statements')}
                  </Button>
                )
              }
              customerDetails={
                props.isAdmin &&
                selectedUserId != null && (
                  <span className='text-muted-foreground text-xs'>
                    #{selectedUserId}
                    {statement?.deleted && ` · ${t('Deleted')}`}
                  </span>
                )
              }
              actions={actions}
              versionActions={versionActions}
              reportStatus={reportStatus}
              versionStatus={versionStatus}
              pendingRefund={pendingRefund}
              versionNotice={versionNotice}
              qualityNotice={qualityNotice}
            />
          )}
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
        {!statementQuery.isError &&
          (statementQuery.isPending || !statement) && <StatementSkeleton />}
        {!statementQuery.isError && !statementQuery.isPending && statement && (
          <>
            <div className={summaryGridClassName}>
              <SummaryCard
                icon={Wallet01Icon}
                title={t('Current balance')}
                value={formatCurrentBalance(statement.current_balance)}
                description={
                  statement.current_balance === null
                    ? t(
                        'Customer record unavailable; current balance cannot be read.'
                      )
                    : undefined
                }
              />
              <SummaryCard
                icon={Activity01Icon}
                title={t('Requests')}
                value={formatInteger(statement.summary.requests)}
              />
              <SummaryCard
                icon={Invoice01Icon}
                title={t('List price')}
                value={
                  statement.original_quota != null
                    ? formatCustomerStatementQuota(statement.original_quota)
                    : t('Not recorded')
                }
              />
              <SummaryCard
                icon={Discount01Icon}
                title={t('Savings')}
                value={
                  statement.discount_quota != null
                    ? formatCustomerStatementQuota(statement.discount_quota)
                    : t('Not recorded')
                }
              />
              <SummaryCard
                emphasized
                icon={ReceiptDollarIcon}
                title={t('Net settled amount')}
                value={formatCustomerStatementQuota(
                  statement.summary.net_quota
                )}
              />
            </div>
            <p className='text-muted-foreground text-xs'>
              {t('Net amounts include task holds and adjustments.')}
            </p>

            <ReportCard className='gap-0 p-0'>
              <CardHeader className='flex flex-wrap items-center justify-between gap-2 px-3 py-2'>
                <CardTitle id={keySummaryHeadingId} className='text-sm'>
                  {byApiKey ? t('Billing by API Key') : t('Billing by channel')}
                </CardTitle>
                {props.isAdmin && (
                  <Select
                    items={[
                      { value: 'api_key', label: t('API Key') },
                      { value: 'channel', label: t('Channel') },
                    ]}
                    value={props.dimension}
                    onValueChange={(value) => {
                      if (value == null) return
                      setAdminModelKeyId(undefined)
                      setExpanded(new Set())
                      props.onDimensionChange(value as BillingDimension)
                    }}
                  >
                    <SelectTrigger aria-label={t('Aggregation dimension')}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent align='end'>
                      <SelectGroup>
                        <SelectItem value='api_key'>{t('API Key')}</SelectItem>
                        <SelectItem value='channel'>{t('Channel')}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                )}
              </CardHeader>
              <CardContent className='flex min-h-0 flex-col px-0'>
                {groups.length === 0 ? (
                  <div className='text-muted-foreground px-4 py-12 text-center'>
                    {t('No settled usage in this billing period.')}
                  </div>
                ) : (
                  <div
                    className={cn(
                      'min-h-0 overflow-auto',
                      props.isAdmin && byApiKey
                        ? 'max-h-[30vh]'
                        : 'max-h-[45vh]'
                    )}
                  >
                    <CustomerBillingTable
                      withContainer={false}
                      aria-labelledby={keySummaryHeadingId}
                    >
                      <TableHeader className='sticky top-0 z-10 bg-(--table-header)'>
                        <TableRow>
                          {!byApiKey && <TableHead className='w-10' />}
                          <TableHead>
                            {props.dimension === 'api_key'
                              ? t('API Key')
                              : t('Channel')}
                          </TableHead>
                          <TableHead className='text-right'>
                            {t('Requests')}
                          </TableHead>
                          <TableHead className='text-right'>
                            {t('List price')}
                          </TableHead>
                          <TableHead className='text-right'>
                            {t('Savings')}
                          </TableHead>
                          <TableHead className='text-right'>
                            {t('Net settled amount')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {visibleGroups.map((group) => {
                          const isExpanded = expanded.has(group.id)
                          const canFilter =
                            hasModelRows && group.models.length > 0
                          const isSelected =
                            canFilter && selectedModelKey?.id === group.id
                          const fallbackName =
                            props.dimension === 'channel'
                              ? t('Channel #{{id}}', { id: group.id })
                              : t('API Key #{{id}}', { id: group.id })
                          const groupName = group.deleted
                            ? fallbackName
                            : group.name
                          const modelRows =
                            !byApiKey && isExpanded
                              ? [
                                  <PagedChannelModels
                                    key={`models-${group.id}`}
                                    label={groupName}
                                    models={group.models}
                                    renderRow={(model, modelIndex) => (
                                      <CustomerModelRow
                                        version={boundVersion}
                                        reportVersion={report?.version}
                                        key={`model-${group.id}-${model.model_name}-${model.billing_mode}`}
                                        model={model}
                                        firstModelRef={
                                          modelIndex === 0 &&
                                          expandScrollTarget === group.id
                                            ? firstModelRowRef
                                            : undefined
                                        }
                                        dimension={props.dimension}
                                        userId={statement.user_id}
                                        versionBound={versionBound}
                                        onViewDetails={openVersionLines}
                                      />
                                    )}
                                  />,
                                ]
                              : []
                          return [
                            <TableRow
                              key={`group-${group.id}`}
                              className={cn(
                                canFilter && 'cursor-pointer',
                                isSelected && 'bg-muted!'
                              )}
                              onClick={
                                canFilter
                                  ? () => showModelBilling(group.id)
                                  : undefined
                              }
                            >
                              {!byApiKey && (
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
                                    onClick={() => {
                                      if (!isExpanded) {
                                        setExpandScrollTarget(group.id)
                                      }
                                      setExpanded((current) =>
                                        toggleSet(current, group.id)
                                      )
                                    }}
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
                              )}
                              <TableCell>
                                {canFilter ? (
                                  <Button
                                    variant='link'
                                    className='h-auto max-w-sm justify-start px-0 text-left wrap-anywhere whitespace-normal'
                                    aria-label={
                                      group.deleted
                                        ? groupName
                                        : `${groupName} #${group.id}`
                                    }
                                    aria-controls={modelBillId}
                                    aria-pressed={isSelected}
                                  >
                                    {groupName}
                                  </Button>
                                ) : (
                                  <div className='font-medium'>{groupName}</div>
                                )}
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
                                {statementAmountCell(
                                  group.original_quota,
                                  t,
                                  boundVersion
                                )}
                              </TableCell>
                              <TableCell className='text-right'>
                                {statementAmountCell(
                                  group.discount_quota,
                                  t,
                                  boundVersion
                                )}
                              </TableCell>
                              <TableCell className='bg-muted/20 text-right font-semibold!'>
                                {formatCustomerStatementQuota(
                                  group.usage.net_quota
                                )}
                              </TableCell>
                            </TableRow>,
                            ...modelRows,
                          ]
                        })}
                      </TableBody>
                      {byApiKey && (
                        <CustomerKeyBillingTotal
                          requests={formatInteger(statement.summary.requests)}
                          original={statementAmountCell(
                            statement.original_quota,
                            t,
                            boundVersion
                          )}
                          savings={statementAmountCell(
                            statement.discount_quota,
                            t,
                            boundVersion
                          )}
                          net={formatCustomerStatementQuota(
                            statement.summary.net_quota
                          )}
                        />
                      )}
                    </CustomerBillingTable>
                  </div>
                )}
                {groups.length > 0 && (
                  <div className='px-3 py-2'>
                    <BillingPagination
                      {...groupPaging}
                      label={
                        byApiKey
                          ? t('Billing by API Key')
                          : t('Billing by channel')
                      }
                    />
                  </div>
                )}
              </CardContent>
            </ReportCard>
            {hasModelRows && modelTotalUsage && (
              <CustomerModelBilling
                id={modelBillId}
                headingRef={modelBillHeadingRef}
                filterLabel={
                  selectedModelKey &&
                  `${selectedModelKey.name} #${selectedModelKey.id}`
                }
                onClearFilter={() => showModelBilling()}
                showPriceColumns={showModelPrices}
                original={statementAmountCell(
                  modelTotals?.original_quota,
                  t,
                  boundVersion
                )}
                savings={statementAmountCell(
                  modelTotals?.discount_quota,
                  t,
                  boundVersion
                )}
                net={formatCustomerStatementQuota(modelTotalUsage.net_quota)}
                requests={formatInteger(modelTotalUsage.requests)}
                pagination={
                  <BillingPagination
                    {...modelPaging}
                    label={t('Model billing')}
                  />
                }
              >
                {visibleModelRows.map(({ group, model }) => (
                  <CustomerModelRow
                    key={JSON.stringify([
                      group.id,
                      model.model_name,
                      model.billing_mode,
                    ])}
                    version={boundVersion}
                    reportVersion={report?.version}
                    model={model}
                    apiKey={{ id: group.id, name: group.name }}
                    billingModelKeyId={selectedModelKey?.id}
                    showPriceColumns={showModelPrices}
                    isAdmin={props.isAdmin}
                    dimension='api_key'
                    userId={statement.user_id}
                    versionBound={versionBound}
                    onViewDetails={openVersionLines}
                  />
                ))}
              </CustomerModelBilling>
            )}
          </>
        )}
      </div>
      <ExportJobsDrawer
        open={exportDrawerOpen}
        onOpenChange={(open) => {
          setExportDrawerOpen(open)
          if (!open) statementExport.clearFocus()
        }}
        billingStatement
        showCustomer={props.isAdmin}
        focusedJobId={statementExport.focusedJobId}
      >
        <BillingExportSection
          title={
            <>
              {t('{{month}} statement', {
                month: formatBillingMonth(
                  props.period.start_timestamp,
                  toIntlLocale(i18n.language)
                ),
              })}
              {exportType !== 'usage_logs' &&
                boundVersion?.version_number != null &&
                ` · v${boundVersion.version_number}`}
            </>
          }
          notice={exportNotice}
          actionLabel={exportLabel}
          disabled={exportDisabled}
          onSubmit={handleStatementExport}
        >
          {props.isAdmin && selectedUserId != null && (
            <p className='text-muted-foreground text-sm'>
              {statement
                ? `${heading} · #${selectedUserId}`
                : t('Customer #{{id}}', { id: selectedUserId })}
            </p>
          )}
          {exportContent}
        </BillingExportSection>
      </ExportJobsDrawer>
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

function PagedChannelModels(props: {
  label: string
  models: CustomerModelSummary[]
  renderRow: (model: CustomerModelSummary, index: number) => ReactNode
}) {
  const { t } = useTranslation()
  const paging = useResultPage(props.models.length)
  return (
    <>
      {props.models
        .slice(paging.offset, paging.offset + paging.pageSize)
        .map(props.renderRow)}
      {paging.total > 0 && (
        <TableRow>
          <TableCell colSpan={6}>
            <BillingPagination
              {...paging}
              label={`${t('Model billing')} · ${props.label}`}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  )
}

function CustomerModelRow(props: {
  version?: BillingStatementVersionInfo | null
  reportVersion?: string
  model: CustomerModelSummary
  firstModelRef?: React.Ref<HTMLTableRowElement>
  apiKey?: { id: number; name: string }
  billingModelKeyId?: number
  showPriceColumns?: boolean
  isAdmin?: boolean
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
  const tokenId = props.apiKey?.id ?? filter.token_id
  const params = new URLSearchParams({
    billing: 'true',
    billingUserId: String(props.userId),
    billingMode: props.model.billing_mode,
    model: props.model.model_name,
    startTime: String(filter.start_timestamp * 1000),
    endTime: String(filter.end_timestamp * 1000),
  })
  if (props.apiKey && !props.isAdmin) {
    params.set('billingView', 'events')
    if (props.billingModelKeyId != null) {
      params.set('billingModelKeyId', String(props.billingModelKeyId))
    }
  }
  if (props.dimension === 'channel') {
    params.set('channel', String(filter.channel_id ?? ''))
  }
  if (props.dimension === 'api_key' && tokenId != null) {
    params.set('tokenId', String(tokenId))
  }
  if (props.reportVersion) {
    params.set('billingReportVersion', props.reportVersion)
  }
  const detailHref = `/usage-logs/common?${params.toString()}`
  const usage = props.model.usage
  const versionDetailFilter = {
    ...(props.dimension === 'channel' ? { channel_id: filter.channel_id } : {}),
    ...(props.dimension === 'api_key' && tokenId != null
      ? { token_id: tokenId }
      : {}),
    model_name: props.model.model_name,
    billing_mode: props.model.billing_mode,
  }
  const detailsAction =
    props.versionBound && props.onViewDetails ? (
      <Button
        variant={props.apiKey ? 'ghost' : 'link'}
        className={props.apiKey ? 'text-primary' : undefined}
        size={props.apiKey ? 'sm' : 'xs'}
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
        variant={props.apiKey ? 'ghost' : 'link'}
        className={props.apiKey ? 'text-primary' : undefined}
        size={props.apiKey ? 'sm' : 'xs'}
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
    )

  return (
    <TableRow
      ref={props.firstModelRef}
      className={props.apiKey ? undefined : 'bg-muted/20'}
    >
      {!props.apiKey && <TableCell />}
      <TableCell>
        <div className={props.apiKey ? undefined : 'pl-4'}>
          <div
            className={cn(
              'flex items-center gap-2',
              props.apiKey && 'flex-wrap'
            )}
          >
            <span
              className={
                props.apiKey
                  ? 'max-w-64 font-medium wrap-anywhere whitespace-normal'
                  : undefined
              }
            >
              {props.model.model_name || t('Unknown model')}
            </span>
            <Badge variant='secondary'>
              {t(billingModeLabel(props.model.billing_mode))}
            </Badge>
          </div>
          {props.model.billing_mode === 'per_image' && (
            <div className='text-muted-foreground mt-1 text-xs'>
              {t('Billable image count')} ·{' '}
              {usage.billable_images == null
                ? t('Not recorded')
                : formatInteger(usage.billable_images)}
            </div>
          )}
          {(!props.apiKey || props.isAdmin) &&
            (props.model.billing_mode === 'token' ||
              props.model.billing_mode === 'per_call') && (
              <div
                data-table-text='secondary'
                className='text-muted-foreground mt-1 text-xs'
              >
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
      {props.apiKey && (
        <TableCell>
          <div className='max-w-56 wrap-anywhere whitespace-normal'>
            {props.apiKey.name}
          </div>
          <div className='text-muted-foreground text-xs'>
            #{props.apiKey.id}
          </div>
        </TableCell>
      )}
      <TableCell className='text-right'>
        {formatInteger(usage.requests)}
      </TableCell>
      {props.apiKey && <CustomerModelDiscounts model={props.model} />}
      {props.showPriceColumns !== false && (
        <>
          <TableCell className='text-right'>
            {statementAmountCell(props.model.original_quota, t, props.version)}
          </TableCell>
          <TableCell className='text-right'>
            {statementAmountCell(props.model.discount_quota, t, props.version)}
          </TableCell>
        </>
      )}
      <TableCell
        className={cn(
          'text-right',
          props.apiKey && 'bg-muted/20 font-semibold!'
        )}
      >
        <div className='font-medium'>
          {formatCustomerStatementQuota(usage.net_quota)}
        </div>
        {!props.apiKey && detailsAction}
      </TableCell>
      {props.apiKey && (
        <TableCell data-column='actions' className='w-px text-right'>
          {detailsAction}
        </TableCell>
      )}
    </TableRow>
  )
}

const summaryGridClassName =
  'grid gap-2 @min-[22rem]:grid-cols-2 @min-[58rem]:grid-cols-5'
const summaryLastCardClassName =
  '@min-[22rem]:col-span-2 @min-[58rem]:col-span-1'

function SummaryCard(props: {
  icon: typeof Wallet01Icon
  title: string
  value: string
  description?: string
  emphasized?: boolean
}) {
  return (
    <ReportMetricCard
      className={props.emphasized ? summaryLastCardClassName : undefined}
      emphasized={props.emphasized}
      icon={<HugeiconsIcon icon={props.icon} size={14} strokeWidth={1.75} />}
      label={props.title}
      value={props.value}
      caption={props.description}
    />
  )
}

function StatementSkeleton() {
  return (
    <div className='space-y-3'>
      <div className={summaryGridClassName}>
        {[0, 1, 2, 3].map((item) => (
          <Skeleton key={item} className='h-9 rounded-lg' />
        ))}
        <Skeleton className={cn('h-9 rounded-lg', summaryLastCardClassName)} />
      </div>
      <Skeleton className='h-40 rounded-lg' />
    </div>
  )
}

function statementAmountCell(
  value: number | undefined,
  translate: (key: string) => string,
  version?: BillingStatementVersionInfo | null
) {
  return value == null
    ? translate('Not recorded')
    : formatVersionQuota(value, version)
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
