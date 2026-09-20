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
  ArrowDown01Icon,
  ArrowRight01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { billingModeLabel, formatCustomerStatementQuota } from '../lib'
import type {
  BillingDataQuality,
  ProviderUrlChannelSummary,
  ProviderUrlGroupSummary,
  ProviderUrlModelSummary,
} from '../types'
import {
  upstreamDiscountLabel,
  upstreamUrlGroupLabel,
} from '../upstream-reconciliation-utils'
import {
  billingDataQualityLabel,
  billingDataQualityReasons,
  formatStatementUsage,
  upstreamModelLabel,
} from '../upstream-statement-utils'

// 金额单元格：未知保持“未知”，绝不把缺失显示成 0。
export function AmountCell(props: { value?: number | null; parent?: boolean }) {
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
    </TableCell>
  )
}

export function QualityBadge(props: {
  quality: BillingDataQuality | undefined
}) {
  const { t } = useTranslation()
  const reasons = billingDataQualityReasons(props.quality, t)
  const badge = (
    <Badge
      variant={
        props.quality?.status === 'partial' || props.quality?.status === 'unavailable'
          ? 'destructive'
          : 'outline'
      }
    >
      {billingDataQualityLabel(props.quality, t)}
    </Badge>
  )
  if (reasons.length === 0) return badge
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={badge} />
        <TooltipContent className='max-w-80'>
          <ul className='list-disc space-y-1 pl-4 text-xs'>
            {reasons.map((reason) => (
              <li key={reason}>{reason}</li>
            ))}
          </ul>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export function UsageCell(props: {
  value: number | null
  unavailable?: boolean
  parent?: boolean
}) {
  const { t } = useTranslation()
  return (
    <TableCell
      className={`text-right tabular-nums ${props.parent ? 'font-semibold' : ''}`}
    >
      {props.unavailable
        ? t('Not recorded')
        : formatStatementUsage(props.value, document.documentElement.lang || 'en')}
    </TableCell>
  )
}

type UpstreamReconciliationTableProps = {
  groups: ProviderUrlGroupSummary[]
  expandedGroups: Set<string>
  expandedModels: Set<string>
  onToggleGroup: (urlKey: string) => void
  onToggleModel: (modelKey: string) => void
  onViewDetails: (props: { urlKey: string; channelId?: number; providerModel?: string; billingMode?: string }) => void
}

// 信息层级：上游 URL → 上游模型 × 计费方式 → 渠道贡献；金额列在用量列之后。
export function UpstreamReconciliationTable(props: UpstreamReconciliationTableProps) {
  const { t } = useTranslation()
  return (
    <Table className='min-w-300'>
      <TableHeader>
        <TableRow>
          <TableHead className='min-w-56'>
            {t('Upstream base URL / model / channel')}
          </TableHead>
          <TableHead>{t('Billing mode')}</TableHead>
          <TableHead className='text-right'>{t('Requests')}</TableHead>
          <TableHead className='text-right'>{t('Input tokens')}</TableHead>
          <TableHead className='text-right'>{t('Cache read tokens')}</TableHead>
          <TableHead className='text-right'>{t('Cache write tokens')}</TableHead>
          <TableHead className='text-right'>{t('Output tokens')}</TableHead>
          <TableHead className='text-right'>{t('Billable calls')}</TableHead>
          <TableHead className='text-right'>
            {t('Original amount (local official price)')}
          </TableHead>
          <TableHead className='text-right'>
            {t('Reference amount (after channel discount)')}
          </TableHead>
          <TableHead>{t('Channel discount')}</TableHead>
          <TableHead>{t('Data status')}</TableHead>
          <TableHead className='text-right'>{t('Actions')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.groups.flatMap((group) => {
          const isExpanded = props.expandedGroups.has(group.url_key)
          return [
            <UrlGroupRow
              key={`group-${group.url_key}`}
              expanded={isExpanded}
              group={group}
              onToggle={() => props.onToggleGroup(group.url_key)}
              onViewDetails={props.onViewDetails}
            />,
            ...(isExpanded
              ? group.models.map((model) => {
                  const modelKey = urlModelKey(group.url_key, model)
                  const modelExpanded = props.expandedModels.has(modelKey)
                  return [
                    <UrlModelRow
                      key={`model-${modelKey}`}
                      expanded={modelExpanded}
                      model={model}
                      onToggle={() => props.onToggleModel(modelKey)}
                      onViewDetails={props.onViewDetails}
                      urlKey={group.url_key}
                    />,
                    ...(modelExpanded
                      ? model.channels.map((channel) => (
                          <UrlChannelRow
                            key={`channel-${modelKey}-${channel.channel_id}`}
                            channel={channel}
                            model={model}
                            onViewDetails={props.onViewDetails}
                            urlKey={group.url_key}
                          />
                        ))
                      : []),
                  ]
                })
              : []),
          ]
        })}
      </TableBody>
    </Table>
  )
}

function urlModelKey(urlKey: string, model: ProviderUrlModelSummary) {
  return `${urlKey}|${model.provider_model}|${model.billing_mode}|${model.provider_model_fallback ? 1 : 0}`
}

type ViewDetailsProps = {
  urlKey: string
  channelId?: number
  providerModel?: string
  billingMode?: string
}

function UrlGroupRow(props: {
  group: ProviderUrlGroupSummary
  expanded: boolean
  onToggle: () => void
  onViewDetails: (props: ViewDetailsProps) => void
}) {
  const { t } = useTranslation()
  const usage = props.group.usage
  return (
    <TableRow className='bg-muted/30 hover:bg-muted/30'>
      <TableCell>
        <div className='flex items-center gap-2'>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-expanded={props.expanded}
            aria-label={props.expanded ? t('Collapse models') : t('Expand models')}
            onClick={props.onToggle}
          >
            <HugeiconsIcon
              icon={props.expanded ? ArrowDown01Icon : ArrowRight01Icon}
              strokeWidth={2}
            />
          </Button>
          <div>
            <div className='font-semibold wrap-break-word'>
              {upstreamUrlGroupLabel(props.group, t)}
            </div>
            <div className='text-muted-foreground text-xs'>
              {props.group.unidentified
                ? t('Unidentified URL — kept per channel')
                : t('Current channel base URL')}
              {' · '}
              {t('{{count}} channels', { count: props.group.channel_count })}
              {props.group.discount_pending_channels > 0
                ? ` · ${t('{{count}} channels pending discount', { count: props.group.discount_pending_channels })}`
                : null}
            </div>
          </div>
        </div>
      </TableCell>
      <TableCell>—</TableCell>
      <UsageCell value={usage.requests} parent />
      <UsageCell value={usage.input_tokens} parent />
      <UsageCell value={usage.cache_read_tokens} parent />
      <UsageCell
        parent
        unavailable={!!props.group.data_quality?.cache_write_unavailable_requests}
        value={usage.cache_write_tokens}
      />
      <UsageCell value={usage.output_tokens} parent />
      <UsageCell value={usage.billable_calls} parent />
      <AmountCell parent value={props.group.original_amount} />
      <AmountCell parent value={props.group.reference_amount} />
      <TableCell>
        {props.group.reference_known ? null : (
          <span className='text-muted-foreground text-xs'>
            {t('Incomplete')}
          </span>
        )}
      </TableCell>
      <TableCell>
        <QualityBadge quality={props.group.data_quality} />
      </TableCell>
      <TableCell className='text-right'>
        <Button
          variant='link'
          size='xs'
          onClick={() => props.onViewDetails({ urlKey: props.group.url_key })}
        >
          {t('View details')}
        </Button>
      </TableCell>
    </TableRow>
  )
}

function UrlModelRow(props: {
  urlKey: string
  model: ProviderUrlModelSummary
  expanded: boolean
  onToggle: () => void
  onViewDetails: (props: ViewDetailsProps) => void
}) {
  const { t } = useTranslation()
  const tokenBilling = props.model.billing_mode === 'token'
  const perCallBilling = props.model.billing_mode === 'per_call'
  return (
    <TableRow>
      <TableCell className='relative pl-14'>
        <span
          aria-hidden='true'
          className='border-muted-foreground/25 absolute top-0 bottom-0 left-7 w-5 border-b border-l'
        />
        <div className='flex items-center gap-2'>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-expanded={props.expanded}
            aria-label={props.expanded ? t('Collapse channels') : t('Expand channels')}
            onClick={props.onToggle}
          >
            <HugeiconsIcon
              icon={props.expanded ? ArrowDown01Icon : ArrowRight01Icon}
              strokeWidth={2}
            />
          </Button>
          <span className='inline-block max-w-lg font-medium wrap-break-word whitespace-normal'>
            {props.model.provider_model_fallback ? '—' : props.model.provider_model}
          </span>
        </div>
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
      />
      <UsageCell
        unavailable={
          tokenBilling && !!props.model.data_quality?.cache_write_unavailable_requests
        }
        value={tokenBilling ? props.model.usage.cache_write_tokens : null}
      />
      <UsageCell value={tokenBilling ? props.model.usage.output_tokens : null} />
      <UsageCell
        value={perCallBilling ? props.model.usage.billable_calls : null}
      />
      <AmountCell value={props.model.original_amount} />
      <AmountCell value={props.model.reference_amount} />
      <TableCell>—</TableCell>
      <TableCell>
        <QualityBadge quality={props.model.data_quality} />
      </TableCell>
      <TableCell className='text-right'>
        <Button
          variant='link'
          size='xs'
          onClick={() =>
            props.onViewDetails({
              urlKey: props.urlKey,
              providerModel: props.model.provider_model_fallback
                ? undefined
                : props.model.provider_model,
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

function UrlChannelRow(props: {
  urlKey: string
  channel: ProviderUrlChannelSummary
  model: ProviderUrlModelSummary
  onViewDetails: (props: ViewDetailsProps) => void
}) {
  const { t } = useTranslation()
  const tokenBilling = props.model.billing_mode === 'token'
  const perCallBilling = props.model.billing_mode === 'per_call'
  const discountStatus = {
    channel_id: props.channel.channel_id,
    channel_name: props.channel.channel_name,
    discount: props.channel.discount,
  }
  return (
    <TableRow>
      <TableCell className='relative pl-24'>
        <span
          aria-hidden='true'
          className='border-muted-foreground/25 absolute top-0 bottom-0 left-14 w-5 border-b border-l'
        />
        <div className='flex flex-col gap-0.5'>
          <span className='text-sm'>{props.channel.channel_name}</span>
          <span className='text-muted-foreground text-xs wrap-break-word'>
            {upstreamModelLabel(props.channel)}
          </span>
        </div>
      </TableCell>
      <TableCell>
        <Badge variant='outline'>
          {t(billingModeLabel(props.model.billing_mode))}
        </Badge>
      </TableCell>
      <UsageCell value={props.channel.usage.requests} />
      <UsageCell
        value={tokenBilling ? props.channel.usage.input_tokens : null}
      />
      <UsageCell
        value={tokenBilling ? props.channel.usage.cache_read_tokens : null}
      />
      <UsageCell
        unavailable={
          tokenBilling &&
          !!props.channel.data_quality?.cache_write_unavailable_requests
        }
        value={tokenBilling ? props.channel.usage.cache_write_tokens : null}
      />
      <UsageCell
        value={tokenBilling ? props.channel.usage.output_tokens : null}
      />
      <UsageCell
        value={perCallBilling ? props.channel.usage.billable_calls : null}
      />
      <AmountCell value={props.channel.original_amount} />
      <AmountCell value={props.channel.reference_amount} />
      <TableCell className='text-xs'>
        {upstreamDiscountLabel(discountStatus, t)}
      </TableCell>
      <TableCell>
        <QualityBadge quality={props.channel.data_quality} />
      </TableCell>
      <TableCell className='text-right'>
        <Button
          variant='link'
          size='xs'
          onClick={() =>
            props.onViewDetails({
              urlKey: props.urlKey,
              channelId: props.channel.channel_id,
              providerModel: props.channel.provider_model_fallback
                ? undefined
                : props.channel.provider_model,
              billingMode: props.channel.billing_mode,
            })
          }
        >
          {t('View details')}
        </Button>
      </TableCell>
    </TableRow>
  )
}
