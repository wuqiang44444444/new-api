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
import { ArrowDown01Icon, ArrowRight01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
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

import { putAdminUpstreamDiscount } from '../api'
import { billingModeLabel, formatCustomerStatementQuota } from '../lib'
import type {
  ProviderUrlChannelGroupSummary,
  ProviderUrlChannelModelSummary,
  ProviderUrlGroupSummary,
} from '../types'
import { useUpstreamPage } from '../upstream-page-query'
import {
  upstreamDiscountLabel,
  upstreamDiscountSourceLabel,
} from '../upstream-reconciliation-utils'
import {
  cacheMeterUnavailableLabel,
  channelRowKey,
  formatStatementUsage,
  upstreamModelLabel,
} from '../upstream-statement-utils'
import { BillingPagination } from './billing-pagination'
import { UpstreamDataStatus } from './upstream-data-status'

// 金额单元格：未知保持“未知”，绝不把缺失显示成 0。
export function AmountCell(props: {
  value?: number | null
  knownSubtotal?: number | null
  parent?: boolean
}) {
  const { t } = useTranslation()
  const amount =
    props.value == null ? (
      <span className='text-muted-foreground'>—</span>
    ) : (
      formatCustomerStatementQuota(props.value)
    )
  return (
    <TableCell
      className={`text-right tabular-nums ${props.parent ? 'font-semibold' : ''}`}
    >
      {amount}
      {props.value == null && props.knownSubtotal != null && (
        <span className='text-muted-foreground block text-xs font-normal'>
          {t('Known subtotal: {{amount}}', {
            amount: formatCustomerStatementQuota(props.knownSubtotal),
          })}
        </span>
      )}
    </TableCell>
  )
}

export function UsageCell(props: {
  value: number | null
  unavailable?: boolean
  unavailableLabel?: string
  showKnownSubtotal?: boolean
  parent?: boolean
}) {
  const { t } = useTranslation()
  let content =
    props.unavailable || props.unavailableLabel
      ? (props.unavailableLabel ?? t('Not recorded'))
      : formatStatementUsage(props.value, document.documentElement.lang || 'en')
  if (props.showKnownSubtotal && props.unavailable && props.value != null) {
    content = t('Known subtotal: {{amount}}', {
      amount: formatStatementUsage(
        props.value,
        document.documentElement.lang || 'en'
      ),
    })
  }

  return (
    <TableCell
      className={`text-right tabular-nums ${props.parent ? 'font-semibold' : ''}`}
    >
      {content}
    </TableCell>
  )
}

export type UpstreamDetailViewSelection = {
  evidenceFilter?: string
  evidenceLabel?: string
  urlKey: string
  channelId?: number
  channelName?: string
  providerModel?: string
  providerModelFallback?: boolean
  billingMode?: string
}

type UpstreamReconciliationTableProps = {
  group: ProviderUrlGroupSummary
  periodStart: number
  periodEnd: number
  expandedChannels: Set<string>
  onToggleChannel: (channelKey: string) => void
  onViewDetails: (props: UpstreamDetailViewSelection) => void
}

type DraftState = {
  channelId: number
  periodStart: number
  expectedVersion: number
  percent: string
}

// 信息层级：上游（卡片）→ 渠道父行 → 该渠道的上游模型 × 计费方式叶子。
// 折扣系数、来源、版本和编辑动作都在渠道父行上，页面不再有第二张折扣表；
// 金额、用量和数据质量均来自后端，前端不重算。
export function UpstreamReconciliationTable(
  props: UpstreamReconciliationTableProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<DraftState | null>(null)

  const saveMutation = useMutation({
    mutationFn: async (payload: {
      period_start: number
      channel_id: number
      discount: string
      expected_version: number
    }) => putAdminUpstreamDiscount(payload),
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Unable to save the discount.'))
        return
      }
      toast.success(t('Channel discount saved.'))
      setDraft(null)
      queryClient.invalidateQueries({
        queryKey: ['billing-upstream-reconciliation'],
      })
    },
    onError: (error: unknown) => {
      const message = error instanceof Error ? error.message : ''
      if (message.includes('409') || message.includes('conflict')) {
        toast.error(
          t('The discount was changed by someone else. Refresh and try again.')
        )
      } else {
        toast.error(message || t('Unable to save the discount.'))
      }
    },
  })

  const startEdit = (channel: ProviderUrlChannelGroupSummary) => {
    setDraft({
      channelId: channel.channel_id,
      periodStart: props.periodStart,
      expectedVersion: channel.discount?.version ?? 0,
      percent: channel.discount
        ? String(Number(channel.discount.value) * 100)
        : '',
    })
  }

  const submitDraft = () => {
    // 切换月份（含命中缓存）后旧草稿立即失效，提交前再校验账期归属。
    if (!draft || draft.periodStart !== props.periodStart) return
    const percent = Number(draft.percent)
    if (!Number.isFinite(percent) || percent <= 0 || percent > 100) {
      toast.error(t('Enter a discount between 0% and 100% (exclusive of 0).'))
      return
    }
    const coefficient = String(Number((percent / 100).toFixed(8)))
    saveMutation.mutate({
      period_start: draft.periodStart,
      channel_id: draft.channelId,
      discount: coefficient,
      expected_version: draft.expectedVersion,
    })
  }

  return (
    // 约定滚动容器（对齐 DataTablePage fixedHeight）：双轴滚动 + sticky 表头，
    // 让横向滚动条始终停在可视区域内，而不是整张表底部的屏幕外。
    <div className='max-h-[45vh] min-h-0 overflow-auto'>
      <Table withContainer={false} className='min-w-300'>
        <TableHeader className='sticky top-0 z-10 bg-(--table-header)'>
          <TableRow>
            <TableHead className='min-w-56'>
              {t('Upstream channel / model')}
            </TableHead>
            <TableHead>{t('Billing mode')}</TableHead>
            <TableHead className='text-right'>{t('Requests')}</TableHead>
            <TableHead className='text-right'>{t('Input tokens')}</TableHead>
            <TableHead className='text-right'>
              {t('Cache read tokens')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Cache write tokens')}
            </TableHead>
            <TableHead className='text-right'>{t('Output tokens')}</TableHead>
            <TableHead className='text-right'>{t('Billable calls')}</TableHead>
            <TableHead className='text-right'>
              {t('Billable seconds')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Original amount (local official price)')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Calculated amount (after channel discount)')}
            </TableHead>
            <TableHead>{t('Channel discount')}</TableHead>
            <TableHead>{t('Data status')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.group.channels.flatMap((channel) => {
            const channelKey = channelRowKey(props.group.url_key, channel)
            const isExpanded = props.expandedChannels.has(channelKey)
            return [
              <ChannelRow
                key={`channel-${channelKey}`}
                channel={channel}
                draft={draft?.channelId === channel.channel_id ? draft : null}
                editing={saveMutation.isPending}
                expanded={isExpanded}
                group={props.group}
                onDraftChange={setDraft}
                onExpandToggle={() => props.onToggleChannel(channelKey)}
                onSubmitDraft={submitDraft}
                onStartEdit={() => startEdit(channel)}
                onViewDetails={props.onViewDetails}
              />,
              isExpanded ? (
                <ChannelModelPage
                  key={`models-${channelKey}`}
                  channel={channel}
                  urlKey={props.group.url_key}
                  period={{
                    start_timestamp: props.periodStart,
                    end_timestamp: props.periodEnd,
                  }}
                  onViewDetails={props.onViewDetails}
                />
              ) : null,
            ]
          })}
        </TableBody>
      </Table>
    </div>
  )
}

function ChannelRow(props: {
  group: ProviderUrlGroupSummary
  channel: ProviderUrlChannelGroupSummary
  draft: DraftState | null
  editing: boolean
  expanded: boolean
  onDraftChange: (draft: DraftState | null) => void
  onExpandToggle: () => void
  onSubmitDraft: () => void
  onStartEdit: () => void
  onViewDetails: (props: UpstreamDetailViewSelection) => void
}) {
  const { t } = useTranslation()
  const channel = props.channel
  const usage = channel.usage
  return (
    <TableRow className='bg-muted/30 hover:bg-muted/30'>
      <TableCell>
        <div className='flex items-center gap-2'>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-expanded={props.expanded}
            aria-label={
              props.expanded ? t('Collapse models') : t('Expand models')
            }
            onClick={props.onExpandToggle}
          >
            <HugeiconsIcon
              icon={props.expanded ? ArrowDown01Icon : ArrowRight01Icon}
              strokeWidth={2}
            />
          </Button>
          <div>
            <div className='font-semibold wrap-break-word'>
              {channel.channel_name}
            </div>
            <div className='text-muted-foreground text-xs'>
              #{channel.channel_id}
            </div>
          </div>
        </div>
      </TableCell>
      <TableCell>—</TableCell>
      <UsageCell parent value={usage.requests} />
      <UsageCell parent value={usage.input_tokens} />
      <UsageCell
        parent
        value={usage.cache_read_tokens}
        unavailableLabel={cacheMeterUnavailableLabel(
          channel.data_quality,
          'read',
          t
        )}
      />
      <UsageCell
        parent
        unavailableLabel={cacheMeterUnavailableLabel(
          channel.data_quality,
          'write',
          t
        )}
        value={usage.cache_write_tokens}
      />
      <UsageCell parent value={usage.output_tokens} />
      <UsageCell parent value={usage.billable_calls} />
      <UsageCell
        parent
        showKnownSubtotal
        value={usage.seconds == null ? null : Number(usage.seconds)}
        unavailable={!!usage.seconds_unavailable_rows}
      />
      <AmountCell
        parent
        value={channel.original_amount}
        knownSubtotal={channel.known_original_amount}
      />
      <AmountCell
        parent
        value={channel.reference_amount}
        knownSubtotal={channel.known_reference_amount}
      />
      <TableCell>
        <div className='flex flex-col'>
          <span className='text-xs'>
            {upstreamDiscountLabel(channel.discount, t)}
          </span>
          <span className='text-muted-foreground text-xs'>
            {upstreamDiscountSourceLabel(channel.discount, t)}
            {channel.discount
              ? ` · ${t('v{{version}} · updated {{time}}', {
                  version: channel.discount.version,
                  time: channel.updated_at
                    ? new Date(channel.updated_at * 1000).toLocaleString()
                    : '—',
                })}`
              : ''}
          </span>
        </div>
      </TableCell>
      <TableCell>
        <UpstreamDataStatus
          onViewEvidence={(entry) =>
            props.onViewDetails({
              urlKey: props.group.url_key,
              channelId: channel.channel_id,
              channelName: `${channel.channel_name} #${channel.channel_id}`,
              evidenceFilter: entry.filter,
              evidenceLabel: entry.text,
            })
          }
          quality={channel.data_quality}
          originalAmount={channel.original_amount}
          usageOnly={channel.usage_only}
        />
      </TableCell>
      <TableCell className='text-right'>
        {props.draft ? (
          <div className='flex flex-col items-stretch gap-1 sm:flex-row sm:items-center sm:justify-end'>
            <Input
              aria-label={t('Discount percent')}
              className='w-24'
              inputMode='decimal'
              placeholder={t('Percent, e.g. 80')}
              value={props.draft.percent}
              onChange={(event) => {
                if (!props.draft) return
                props.onDraftChange({
                  ...props.draft,
                  percent: event.target.value,
                })
              }}
            />
            <Button
              size='xs'
              disabled={props.editing}
              onClick={props.onSubmitDraft}
            >
              {t('Save')}
            </Button>
            <Button
              variant='ghost'
              size='xs'
              onClick={() => props.onDraftChange(null)}
            >
              {t('Cancel')}
            </Button>
          </div>
        ) : (
          <div className='flex justify-end gap-1'>
            <Button variant='link' size='xs' onClick={props.onStartEdit}>
              {channel.discount ? t('Edit') : t('Fill in')}
            </Button>
            <Button
              variant='link'
              size='xs'
              onClick={() =>
                props.onViewDetails({
                  urlKey: props.group.url_key,
                  channelId: channel.channel_id,
                  channelName: `${channel.channel_name} #${channel.channel_id}`,
                })
              }
            >
              {t('View details')}
            </Button>
          </div>
        )}
      </TableCell>
    </TableRow>
  )
}

function ChannelModelRow(props: {
  urlKey: string
  model: ProviderUrlChannelModelSummary
  firstModelRef?: React.Ref<HTMLTableRowElement>
  discount: ProviderUrlChannelGroupSummary['discount']
  onViewDetails: (props: UpstreamDetailViewSelection) => void
}) {
  const { t } = useTranslation()
  const tokenBilling = props.model.billing_mode === 'token'
  const perCallBilling = props.model.billing_mode === 'per_call'
  return (
    <TableRow ref={props.firstModelRef}>
      <TableCell className='relative pl-14'>
        <span
          aria-hidden='true'
          className='border-muted-foreground/25 absolute top-0 bottom-0 left-7 w-5 border-b border-l'
        />
        <span className='inline-block max-w-lg font-medium wrap-break-word whitespace-normal'>
          {upstreamModelLabel(props.model)}
        </span>
      </TableCell>
      <TableCell>
        <Badge variant='secondary'>
          {t(billingModeLabel(props.model.billing_mode))}
        </Badge>
      </TableCell>
      <UsageCell value={props.model.usage.requests} />
      <UsageCell value={tokenBilling ? props.model.usage.input_tokens : null} />
      <UsageCell
        value={tokenBilling ? props.model.usage.cache_read_tokens : null}
        unavailableLabel={
          tokenBilling
            ? cacheMeterUnavailableLabel(props.model.data_quality, 'read', t)
            : undefined
        }
      />
      <UsageCell
        unavailableLabel={
          tokenBilling
            ? cacheMeterUnavailableLabel(props.model.data_quality, 'write', t)
            : undefined
        }
        value={tokenBilling ? props.model.usage.cache_write_tokens : null}
      />
      <UsageCell
        value={tokenBilling ? props.model.usage.output_tokens : null}
      />
      <UsageCell
        value={perCallBilling ? props.model.usage.billable_calls : null}
      />
      <UsageCell
        showKnownSubtotal
        value={
          props.model.usage.seconds == null
            ? null
            : Number(props.model.usage.seconds)
        }
        unavailable={
          props.model.billing_mode === 'per_second' &&
          (props.model.usage.seconds == null ||
            !!props.model.usage.seconds_unavailable_rows)
        }
      />
      <AmountCell
        value={props.model.original_amount}
        knownSubtotal={props.model.known_original_amount}
      />
      <AmountCell
        value={props.model.reference_amount}
        knownSubtotal={props.model.known_reference_amount}
      />
      <TableCell className='text-muted-foreground text-xs'>
        {t('Inherited channel discount: {{discount}}', {
          discount: upstreamDiscountLabel(props.discount, t),
        })}
      </TableCell>
      <TableCell>
        <UpstreamDataStatus
          onViewEvidence={(entry) =>
            props.onViewDetails({
              urlKey: props.urlKey,
              channelId: props.model.detail_filter.channel_id,
              providerModel: props.model.provider_model,
              providerModelFallback:
                props.model.provider_model_fallback ?? false,
              billingMode: props.model.billing_mode,
              evidenceFilter: entry.filter,
              evidenceLabel: entry.text,
            })
          }
          quality={props.model.data_quality}
          originalAmount={props.model.original_amount}
          usageOnly={props.model.usage_only}
        />
      </TableCell>
      <TableCell className='text-right'>
        <Button
          variant='link'
          size='xs'
          onClick={() =>
            props.onViewDetails({
              urlKey: props.urlKey,
              channelId: props.model.detail_filter.channel_id,
              providerModel: props.model.provider_model,
              providerModelFallback:
                props.model.provider_model_fallback ?? false,
              billingMode: props.model.billing_mode,
            })
          }
        >
          {t('View details')}
        </Button>
      </TableCell>
    </TableRow>
  )
}

function ChannelModelPage(props: {
  channel: ProviderUrlChannelGroupSummary
  urlKey: string
  period: { start_timestamp: number; end_timestamp: number }
  onViewDetails: (props: UpstreamDetailViewSelection) => void
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const firstModelRowRef = useRef<HTMLTableRowElement | null>(null)
  const query = useUpstreamPage({
    ...props.period,
    level: 'models',
    url_key: props.urlKey,
    channel_id: props.channel.channel_id,
    page,
    page_size: pageSize,
  })
  // 展开渠道后，模型行落在封顶滚动容器的折叠区内；加载态和加载完成时把
  // 首行滚入视野，避免展开后看不到明细。
  useEffect(() => {
    if (query.isPending || query.isSuccess) {
      firstModelRowRef.current?.scrollIntoView({ block: 'nearest' })
    }
  }, [query.isPending, query.isSuccess])
  if (query.isPending) {
    return (
      <TableRow ref={firstModelRowRef}>
        <TableCell colSpan={14}>
          <LoadingState />
        </TableCell>
      </TableRow>
    )
  }
  if (query.isError) {
    return (
      <TableRow>
        <TableCell colSpan={14}>
          <ErrorState
            title={t('Unable to load upstream reconciliation')}
            onRetry={() => void query.refetch()}
          />
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {query.data.result.models.map((model, index) => (
        <ChannelModelRow
          key={`${model.provider_model}:${model.billing_mode}:${model.provider_model_fallback ?? false}`}
          model={model}
          firstModelRef={index === 0 ? firstModelRowRef : undefined}
          discount={props.channel.discount}
          urlKey={props.urlKey}
          onViewDetails={(selection) =>
            props.onViewDetails({
              ...selection,
              channelName: `${props.channel.channel_name} #${props.channel.channel_id}`,
            })
          }
        />
      ))}
      <TableRow>
        <TableCell colSpan={14}>
          <BillingPagination
            label={t('Model pages')}
            page={page}
            pageSize={pageSize}
            total={query.data.result.total}
            onChange={(p, size) => {
              setPage(p)
              setPageSize(size)
            }}
          />
        </TableCell>
      </TableRow>
    </>
  )
}
