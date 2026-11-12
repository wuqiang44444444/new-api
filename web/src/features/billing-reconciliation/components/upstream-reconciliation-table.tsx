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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { billingModeLabel, formatCustomerStatementQuota } from '../lib'
import type {
  BillingMode,
  ProviderUrlChannelGroupSummary,
  ProviderUrlChannelModelSummary,
} from '../types'
import { useUpstreamPage } from '../upstream-page-query'
import {
  upstreamDiscountLabel,
  upstreamDiscountSourceLabel,
} from '../upstream-reconciliation-utils'
import {
  cacheMeterUnavailableLabel,
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
  unverified?: boolean
}) {
  const { t } = useTranslation()
  const value = props.unverified ? null : props.value
  const subtotal = props.unverified ? null : props.knownSubtotal
  const amount =
    value == null ? (
      <span className='text-muted-foreground'>{t('Cannot calculate')}</span>
    ) : (
      formatCustomerStatementQuota(value)
    )
  return (
    <TableCell
      className={`text-right tabular-nums ${props.parent ? 'font-semibold' : ''}`}
    >
      {amount}
      {value == null && subtotal != null && (
        <span className='text-muted-foreground block text-xs font-normal'>
          {t('Known subtotal: {{amount}}', {
            amount: formatCustomerStatementQuota(subtotal),
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
  channelId?: number
  channelName?: string
  providerModel?: string
  providerModelFallback?: boolean
  billingMode?: BillingMode
}

type UpstreamReconciliationTableProps = {
  reportVersion?: string
  urlKey?: string
  channels: ProviderUrlChannelGroupSummary[]
  periodStart: number
  periodEnd: number
  expandedChannels: Set<string>
  onToggleChannel: (channelKey: string) => void
  onViewDetails: (props: UpstreamDetailViewSelection) => void
}

// Read-only channel rows; manual configuration belongs in channel management.
export function UpstreamReconciliationTable(
  props: UpstreamReconciliationTableProps
) {
  const { t } = useTranslation()
  return (
    // 约定滚动容器（对齐 DataTablePage fixedHeight）：双轴滚动 + sticky 表头，
    // 让横向滚动条始终停在可视区域内，而不是整张表底部的屏幕外。
    <div className='max-h-[45vh] min-h-0 overflow-auto'>
      <Table withContainer={false} className='min-w-300'>
        <TableHeader className='sticky top-0 z-10 bg-(--table-header)'>
          <TableRow>
            <TableHead className='min-w-40 sm:min-w-56'>
              {t('Upstream channel / model')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Discounted reference amount')}
            </TableHead>
            <TableHead className='text-right'>
              {t('Original amount (local official price)')}
            </TableHead>
            <TableHead>{t('Original amount status')}</TableHead>
            <TableHead className='text-right'>{t('Actions')}</TableHead>
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
            <TableHead>{t('Channel discount')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.channels.flatMap((channel) => {
            const channelKey = props.urlKey
              ? `${props.urlKey}:${channel.channel_id}`
              : String(channel.channel_id)
            const isExpanded = props.expandedChannels.has(channelKey)
            return [
              <ChannelRow
                key={`channel-${channelKey}`}
                channel={channel}
                expanded={isExpanded}
                onExpandToggle={() => props.onToggleChannel(channelKey)}
                onViewDetails={props.onViewDetails}
              />,
              isExpanded ? (
                <ChannelModelPage
                  key={`models-${channelKey}`}
                  channel={channel}
                  reportVersion={props.reportVersion}
                  urlKey={props.urlKey}
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
  channel: ProviderUrlChannelGroupSummary
  expanded: boolean
  onExpandToggle: () => void
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
            <div className='font-semibold wrap-break-word max-sm:max-w-36 max-sm:whitespace-normal'>
              {channel.upstream_name ||
                channel.channel_name ||
                t('Deleted channel')}
            </div>
            <div className='text-muted-foreground text-xs'>
              {channel.upstream_name ? `${channel.channel_name} · ` : ''}#
              {channel.channel_id}
              {channel.deleted ? ` · ${t('Deleted channel')}` : ''}
            </div>
          </div>
        </div>
      </TableCell>
      <AmountCell
        parent
        unverified={channel.data_quality?.has_estimated_original}
        value={channel.reference_amount}
        knownSubtotal={channel.known_reference_amount}
      />
      <AmountCell
        parent
        value={channel.original_amount}
        knownSubtotal={channel.known_original_amount}
        unverified={channel.data_quality?.has_estimated_original}
      />
      <TableCell>
        <UpstreamDataStatus
          onViewEvidence={(entry) =>
            props.onViewDetails({
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
        <Button
          variant='link'
          size='xs'
          onClick={() =>
            props.onViewDetails({
              channelId: channel.channel_id,
              channelName: `${channel.channel_name} #${channel.channel_id}`,
            })
          }
        >
          {t('View details')}
        </Button>
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
    </TableRow>
  )
}

function ChannelModelRow(props: {
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
        <span className='inline-block max-w-28 font-medium wrap-break-word whitespace-normal sm:max-w-lg'>
          {upstreamModelLabel(props.model)}
        </span>
      </TableCell>
      <AmountCell
        unverified={props.model.data_quality?.has_estimated_original}
        value={props.model.reference_amount}
        knownSubtotal={props.model.known_reference_amount}
      />
      <AmountCell
        value={props.model.original_amount}
        knownSubtotal={props.model.known_original_amount}
        unverified={props.model.data_quality?.has_estimated_original}
      />
      <TableCell>
        <UpstreamDataStatus
          onViewEvidence={(entry) =>
            props.onViewDetails({
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
      <TableCell className='text-muted-foreground text-xs'>
        {t('Inherited channel discount: {{discount}}', {
          discount: upstreamDiscountLabel(props.discount, t),
        })}
      </TableCell>
    </TableRow>
  )
}

function ChannelModelPage(props: {
  reportVersion?: string
  urlKey?: string
  channel: ProviderUrlChannelGroupSummary
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
    ...(props.reportVersion
      ? { report_version: props.reportVersion, url_key: props.urlKey }
      : {}),
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
