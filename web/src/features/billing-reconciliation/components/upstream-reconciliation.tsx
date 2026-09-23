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
import {
  DownloadIcon,
  InformationCircleIcon,
  PencilIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
} from '@/components/ui/drawer'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'

import { postAdminUpstreamDiscountInit, putAdminUpstreamURLName } from '../api'
import { createUpstreamExport } from '../export-api'
import { ExportJobsDrawer } from '../export-jobs-drawer'
import { formatCustomerStatementQuota } from '../lib'
import type { ProviderUrlGroupSummary } from '../types'
import { useInitializeUpstreamDiscounts } from '../upstream-discount-init'
import {
  useUpstreamPage,
  collectUpstreamChannelPages,
} from '../upstream-page-query'
import {
  formatShanghaiTimestamp,
  upstreamUrlGroupLabel,
  upstreamUrlGroupSubtitle,
} from '../upstream-reconciliation-utils'
import {
  type BillingEvidenceEntry,
  billingAccountingEntries,
  billingEvidenceGroups,
} from '../upstream-statement-utils'
import { BillingPagination } from './billing-pagination'
import { UpstreamDataStatus } from './upstream-data-status'
import {
  UpstreamDetailPanel,
  type UpstreamDetailSelection,
} from './upstream-detail-panel'
import { UpstreamEvidenceCoverage } from './upstream-evidence-coverage'
import { UpstreamEvidenceLink } from './upstream-evidence-link'
import { UpstreamReconciliationTable } from './upstream-reconciliation-table'
import { UpstreamURLFilter } from './upstream-url-filter'

const maxUpstreamNameLength = 255

type UpstreamReconciliationViewProps = {
  month: string
  onMonthChange: (month: string) => void
  period: { start_timestamp: number; end_timestamp: number }
}

export function UpstreamReconciliationView(
  props: UpstreamReconciliationViewProps
) {
  return <UpstreamReconciliationContent key={props.month} {...props} />
}

function UpstreamReconciliationContent(props: UpstreamReconciliationViewProps) {
  const { t } = useTranslation()
  const [urlFilter, setUrlFilter] = useState('all')
  const [expandedChannels, setExpandedChannels] = useState<Set<string>>(
    new Set()
  )
  const [detailSelection, setDetailSelection] =
    useState<UpstreamDetailSelection | null>(null)
  const detailTrigger = useRef<HTMLElement | null>(null)
  const detailTitle = useRef<HTMLHeadingElement | null>(null)
  const [exportOpen, setExportOpen] = useState(false)

  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [urlSelection, setUrlSelection] = useState<{
    value: string
    label: string
  } | null>(null)
  const initialization = useInitializeUpstreamDiscounts(props.period)
  const query = useUpstreamPage(
    {
      ...props.period,
      level: 'groups',
      url_key: urlFilter === 'all' ? undefined : urlFilter,
      page,
      page_size: pageSize,
    },
    initialization.isSuccess
  )
  const groups = query.data?.result.url_groups ?? []
  const visibleGroups = groups

  if (query.isError || initialization.isError) {
    return (
      <div className='min-h-0 flex-1 overflow-y-auto'>
        <ErrorState
          title={t('Unable to load upstream reconciliation')}
          description={
            query.error instanceof Error
              ? query.error.message
              : t('Please try again later.')
          }
          onRetry={() =>
            initialization.isError ? initialization.refetch() : query.refetch()
          }
        />
      </div>
    )
  }
  if (initialization.isPending || query.isPending || !query.data) {
    return (
      <div className='min-h-0 flex-1 overflow-y-auto'>
        <Skeleton className='h-96 rounded-xl' />
      </div>
    )
  }

  const modelCount = query.data.result.model_count
  const visibleQuality = query.data.result.data_quality
  const openDetails = (selection: UpstreamDetailSelection) => {
    detailTrigger.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null
    setDetailSelection(selection)
  }
  const viewEvidence = (entry: BillingEvidenceEntry) =>
    openDetails({
      urlKey: urlFilter === 'all' ? undefined : urlFilter,
      evidenceFilter: entry.filter,
      evidenceLabel: entry.text,
    })
  const evidenceGroups = billingEvidenceGroups(visibleQuality, t)
  const accountingNotes = billingAccountingEntries(visibleQuality, t)
  const selectedGroup = groups.find(
    (group) => group.url_key === detailSelection?.urlKey
  )
  const detailScope = [
    props.month,
    selectedGroup ? upstreamUrlGroupLabel(selectedGroup, t) : '',
    detailSelection?.channelName ??
      (detailSelection?.channelId ? `#${detailSelection.channelId}` : ''),
    detailSelection?.providerModel,
    detailSelection?.billingMode,
    detailSelection?.evidenceLabel,
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='flex shrink-0 flex-col gap-3 xl:flex-row xl:items-end xl:justify-between'>
        <div className='grid flex-1 gap-3 sm:grid-cols-2 xl:max-w-xl'>
          <Field>
            <FieldLabel htmlFor='upstream-billing-month'>
              {t('Billing month')}
            </FieldLabel>
            <Input
              id='upstream-billing-month'
              type='month'
              value={props.month}
              onChange={(event) => {
                setPage(1)
                setExpandedChannels(new Set())
                setDetailSelection(null)
                props.onMonthChange(event.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor='upstream-url-filter'>
              {t('Upstream base URL')}
            </FieldLabel>
            <UpstreamURLFilter
              period={props.period}
              value={urlSelection}
              onChange={(value) => {
                setUrlSelection(value)
                setUrlFilter(value?.value ?? 'all')
                setPage(1)
                setExpandedChannels(new Set())
                setDetailSelection(null)
              }}
            />
          </Field>
        </div>
        <Button variant='outline' onClick={() => setExportOpen(true)}>
          {t('Export jobs')}
        </Button>
      </div>

      <div className='text-muted-foreground shrink-0 space-y-1 text-sm'>
        <p>
          {t(
            '{{groups}} URL groups · {{models}} models · Generated at {{time}} (Asia/Shanghai, {{start}} – {{end}})',
            {
              groups: query.data.result.total,
              models: modelCount,
              time: formatShanghaiTimestamp(query.data.generated_at),
              start: formatShanghaiTimestamp(props.period.start_timestamp),
              end: formatShanghaiTimestamp(props.period.end_timestamp),
            }
          )}
        </p>
        <p className='text-xs'>
          {t(
            "Usage is grouped by each channel's current base URL. Amounts include customer settlement and channel tests with verified pricing evidence. Missing amounts keep totals incomplete; known subtotals are shown separately."
          )}
        </p>
        <p className='text-xs'>
          {t(
            'Calculated amounts use local list prices and the channel-month coefficient. Confirm supplier prices and discounts against the supplier bill.'
          )}
        </p>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto'>
        <div className='flex flex-col gap-4'>
          {evidenceGroups.length > 0 ||
          accountingNotes.length > 0 ||
          visibleQuality?.evidence_coverage ? (
            <Alert>
              <HugeiconsIcon icon={InformationCircleIcon} strokeWidth={2} />
              <AlertTitle>{t('Reconciliation evidence')}</AlertTitle>
              <AlertDescription>
                <UpstreamEvidenceCoverage
                  quality={visibleQuality}
                  onViewEvidence={viewEvidence}
                />
                {evidenceGroups.length > 0 ? (
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Except for the mutually exclusive channel-test breakdown, categories may overlap on the same record; do not add these counts together.'
                    )}
                  </p>
                ) : null}
                {evidenceGroups.map((group) => (
                  <div key={group.key} className='space-y-1 text-xs'>
                    {group.title ? (
                      <p className='font-medium'>{group.title}</p>
                    ) : null}
                    <ul className='list-disc space-y-1 pl-4'>
                      {group.entries.map((entry) => (
                        <li key={entry.filter || entry.text}>
                          {entry.text}
                          <UpstreamEvidenceLink
                            entry={entry}
                            onViewEvidence={viewEvidence}
                          />
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
                {accountingNotes.length > 0 ? (
                  <div className='space-y-1 text-xs'>
                    <p className='font-medium'>
                      {t('Confirmed accounting results')}
                    </p>
                    {accountingNotes.map((note) => (
                      <p key={note.filter || note.text}>
                        {note.text}
                        <UpstreamEvidenceLink
                          entry={note}
                          onViewEvidence={viewEvidence}
                        />
                      </p>
                    ))}
                  </div>
                ) : null}
              </AlertDescription>
            </Alert>
          ) : null}

          {visibleGroups.length === 0 ? (
            <Empty className='min-h-64 border-0'>
              <EmptyHeader>
                <EmptyTitle>{t('No upstream usage')}</EmptyTitle>
                <EmptyDescription>
                  {t(
                    'No upstream usage matches the current billing period and filters.'
                  )}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <>
              {visibleGroups.map((group) => (
                <UpstreamGroupCard
                  key={`${props.month}:${group.url_key}`}
                  expandedChannels={expandedChannels}
                  group={group}
                  onToggleChannel={(channelKey) =>
                    setExpandedChannels((current) => {
                      const next = new Set(current)
                      if (next.has(channelKey)) next.delete(channelKey)
                      else next.add(channelKey)
                      return next
                    })
                  }
                  onViewDetails={openDetails}
                  onShowExports={() => setExportOpen(true)}
                  period={props.period}
                />
              ))}
            </>
          )}

          <BillingPagination
            label={t('Upstream pages')}
            page={page}
            pageSize={pageSize}
            total={query.data.result.total}
            disabled={query.isFetching}
            onChange={(p, size) => {
              setPage(p)
              setPageSize(size)
              setExpandedChannels(new Set())
            }}
          />
        </div>
      </div>
      <Drawer
        open={detailSelection != null}
        onOpenChange={(open) => {
          if (!open) setDetailSelection(null)
        }}
      >
        <DrawerContent
          className='data-[vaul-drawer-direction=bottom]:max-h-[90vh]'
          onOpenAutoFocus={(event) => {
            event.preventDefault()
            detailTitle.current?.focus({ preventScroll: true })
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault()
            if (!exportOpen) detailTrigger.current?.focus()
          }}
        >
          <DrawerHeader className='shrink-0 text-left'>
            <DrawerTitle ref={detailTitle} tabIndex={-1}>
              {t('Upstream evidence details')}
            </DrawerTitle>
            <DrawerDescription>{detailScope}</DrawerDescription>
          </DrawerHeader>
          <div className='min-h-0 overflow-y-auto px-4 pb-4'>
            {detailSelection ? (
              <UpstreamDetailPanel
                key={`${props.month}:${JSON.stringify(detailSelection)}`}
                onClose={() => setDetailSelection(null)}
                period={props.period}
                selection={detailSelection}
                onShowExports={() => {
                  setDetailSelection(null)
                  setExportOpen(true)
                }}
              />
            ) : null}
          </div>
        </DrawerContent>
      </Drawer>
      <ExportJobsDrawer open={exportOpen} onOpenChange={setExportOpen} />
    </div>
  )
}

// 一张上游卡片：标题为上游名称（未设置时为安全 URL），下方保留安全 URL，
// 卡片内只有一张“渠道 → 模型 × 计费方式”表；折扣编辑在渠道父行上，
// “从上月初始化”保留在卡片工具区。展开状态由视图层持有，跨卡片共享。
function UpstreamGroupCard(props: {
  group: ProviderUrlGroupSummary
  period: { start_timestamp: number; end_timestamp: number }
  expandedChannels: Set<string>
  onToggleChannel: (channelKey: string) => void
  onViewDetails: (selection: UpstreamDetailSelection) => void
  onShowExports: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [naming, setNaming] = useState(false)
  const [nameDraft, setNameDraft] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const channelsQuery = useUpstreamPage({
    ...props.period,
    level: 'channels',
    url_key: props.group.url_key,
    page,
    page_size: pageSize,
  })
  const channels = channelsQuery.data?.result.channels ?? []

  const invalidate = () => {
    queryClient.invalidateQueries({
      queryKey: ['billing-upstream-reconciliation'],
    })
  }

  const exportMutation = useMutation({
    mutationFn: () =>
      createUpstreamExport({
        job_type: 'upstream_summary',
        start_timestamp: props.period.start_timestamp,
        // Page queries include the last second; export jobs use an exclusive end.
        end_timestamp: props.period.end_timestamp + 1,
        url_key: props.group.url_key,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['customer-export-jobs'] })
      toast.success(t('Export submitted. Track it in export jobs.'))
      props.onShowExports()
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Unable to export statement.'))
    },
  })

  const initMutation = useMutation({
    mutationFn: async () => {
      const all = await collectUpstreamChannelPages({
        ...props.period,
        url_key: props.group.url_key,
      })
      const outcomes = [] as { channel_id: number; outcome: string }[]
      for (let offset = 0; offset < all.length; offset += 100) {
        const response = await postAdminUpstreamDiscountInit({
          period_start: props.period.start_timestamp,
          channel_ids: all
            .slice(offset, offset + 100)
            .map((channel) => channel.channel_id),
        })
        if (!response.success) return response
        outcomes.push(...(response.data?.outcomes ?? []))
      }
      return { success: true, data: { outcomes }, message: '' }
    },
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(
          response.message || t('Unable to initialize monthly discounts.')
        )
        return
      }
      const created = response.data?.outcomes?.filter(
        (outcome) => outcome.outcome === 'created'
      ).length
      const missing = response.data?.outcomes?.filter(
        (outcome) => outcome.outcome === 'defaulted'
      ).length
      if (created) {
        toast.success(
          t('Inherited {{count}} channel discounts from last month.', {
            count: created,
          })
        )
      }
      if (missing) {
        toast.info(
          t('{{count}} channels use the default coefficient 1.', {
            count: missing,
          })
        )
      }
      if (!created && !missing) {
        toast.info(t('Monthly discounts are already initialized.'))
      }
      invalidate()
    },
    onError: () => {
      toast.error(t('Unable to initialize monthly discounts.'))
    },
  })

  const nameMutation = useMutation({
    mutationFn: async (name: string) =>
      putAdminUpstreamURLName({ url_key: props.group.url_key, name }),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Unable to save the upstream name.'))
        return
      }
      toast.success(t('Upstream name saved.'))
      setNaming(false)
      invalidate()
    },
    onError: () => {
      toast.error(t('Unable to save the upstream name.'))
    },
  })

  const startNaming = () => {
    setNameDraft(props.group.custom_name ?? '')
    setNaming(true)
  }
  const submitName = () => {
    const name = nameDraft.trim()
    // 与后端一致：按 Unicode 码点计数，不按 UTF-16 单元或 UTF-8 字节。
    if ([...name].length > maxUpstreamNameLength) {
      toast.error(t('The upstream name is too long.'))
      return
    }
    nameMutation.mutate(name)
  }

  return (
    <Card>
      <CardHeader>
        <div className='flex flex-wrap items-start justify-between gap-2'>
          <div className='min-w-0'>
            <CardTitle className='wrap-break-word'>
              {upstreamUrlGroupLabel(props.group, t)}
            </CardTitle>
            <p className='text-muted-foreground mt-1 text-xs wrap-break-word'>
              {upstreamUrlGroupSubtitle(props.group, t)}
              {' · '}
              {t('{{count}} channels', {
                count: props.group.channel_count,
              })}
              {props.group.discount_pending_channels > 0
                ? ` · ${t('{{count}} channels pending discount', { count: props.group.discount_pending_channels })}`
                : null}
            </p>
          </div>
          <div className='flex flex-wrap items-center justify-end gap-1'>
            {naming ? (
              <div className='flex items-center gap-1'>
                <Input
                  aria-label={t('Upstream name')}
                  className='w-52'
                  placeholder={t('Name shown as the card title')}
                  value={nameDraft}
                  onChange={(event) => setNameDraft(event.target.value)}
                />
                <Button
                  size='xs'
                  disabled={nameMutation.isPending}
                  onClick={submitName}
                >
                  {t('Save')}
                </Button>
                <Button
                  variant='ghost'
                  size='xs'
                  onClick={() => setNaming(false)}
                >
                  {t('Cancel')}
                </Button>
              </div>
            ) : (
              <Button variant='ghost' size='xs' onClick={startNaming}>
                <HugeiconsIcon icon={PencilIcon} strokeWidth={2} />
                {props.group.custom_name
                  ? t('Edit upstream name')
                  : t('Add upstream name')}
              </Button>
            )}
            <Button
              variant='outline'
              size='xs'
              onClick={() =>
                props.onViewDetails({ urlKey: props.group.url_key })
              }
            >
              {t('View details')}
            </Button>
            <Button
              variant='outline'
              size='xs'
              disabled={exportMutation.isPending}
              onClick={() => exportMutation.mutate()}
            >
              <HugeiconsIcon icon={DownloadIcon} strokeWidth={2} />
              {t('Export our statement')}
            </Button>
            <Button
              variant='outline'
              size='xs'
              disabled={initMutation.isPending}
              onClick={() => initMutation.mutate()}
            >
              {t('Initialize from last month')}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className='flex min-h-0 flex-col gap-2'>
        <dl className='grid shrink-0 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Original amount (local official price)')}
            </dt>
            <dd className='font-semibold tabular-nums'>
              {props.group.original_amount == null
                ? t('Unknown')
                : formatCustomerStatementQuota(props.group.original_amount)}
              {props.group.original_amount == null &&
                props.group.known_original_amount != null && (
                  <span className='text-muted-foreground block text-xs font-normal'>
                    {t('Known subtotal: {{amount}}', {
                      amount: formatCustomerStatementQuota(
                        props.group.known_original_amount
                      ),
                    })}
                  </span>
                )}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Calculated amount (after channel discount)')}
            </dt>
            <dd className='font-semibold tabular-nums'>
              {props.group.reference_known &&
              props.group.reference_amount != null
                ? formatCustomerStatementQuota(props.group.reference_amount)
                : t('Incomplete')}
              {props.group.reference_amount == null &&
                props.group.known_reference_amount != null && (
                  <span className='text-muted-foreground block text-xs font-normal'>
                    {t('Known subtotal: {{amount}}', {
                      amount: formatCustomerStatementQuota(
                        props.group.known_reference_amount
                      ),
                    })}
                  </span>
                )}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('Requests')}</dt>
            <dd className='font-semibold tabular-nums'>
              {props.group.usage.requests.toLocaleString()}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>
              {t('Data status')}
            </dt>
            <dd>
              <UpstreamDataStatus
                onViewEvidence={(entry) =>
                  props.onViewDetails({
                    urlKey: props.group.url_key,
                    evidenceFilter: entry.filter,
                    evidenceLabel: entry.text,
                  })
                }
                quality={props.group.data_quality}
                originalAmount={props.group.original_amount}
                usageOnly={props.group.usage_only}
              />
            </dd>
          </div>
        </dl>
        <p className='text-muted-foreground shrink-0 text-xs'>
          {t(
            'One comprehensive coefficient per channel per month; it applies to every model and billing mode and never changes customer charges.'
          )}
        </p>
        {channelsQuery.isPending && <Skeleton className='h-32' />}
        {channelsQuery.isError && (
          <ErrorState
            title={t('Unable to load upstream reconciliation')}
            onRetry={() => void channelsQuery.refetch()}
          />
        )}
        {channelsQuery.isSuccess && (
          <>
            <UpstreamReconciliationTable
              expandedChannels={props.expandedChannels}
              group={{ ...props.group, channels }}
              onToggleChannel={props.onToggleChannel}
              onViewDetails={props.onViewDetails}
              periodStart={props.period.start_timestamp}
              periodEnd={props.period.end_timestamp}
            />
            <BillingPagination
              label={t('Channel pages')}
              page={page}
              pageSize={pageSize}
              total={channelsQuery.data?.result.total ?? 0}
              onChange={(p, size) => {
                setPage(p)
                setPageSize(size)
              }}
            />
          </>
        )}
      </CardContent>
    </Card>
  )
}
