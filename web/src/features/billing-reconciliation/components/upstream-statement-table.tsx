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
  LinkSquare02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
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

import { billingModeLabel } from '../lib'
import type {
  BillingDataQuality,
  ProviderChannelSummary,
  ProviderModelSummary,
} from '../types'
import {
  billingDataQualityLabel,
  billingDataQualityReasons,
  formatStatementUsage,
} from '../upstream-statement-utils'

type UpstreamStatementTableProps = {
  channels: ProviderChannelSummary[]
  expandedChannels: Set<number>
  onToggleChannel: (channelId: number) => void
}

export function UpstreamStatementTable(props: UpstreamStatementTableProps) {
  const { t } = useTranslation()
  return (
    <Table className='min-w-260'>
      <TableHeader>
        <TableRow>
          <TableHead className='min-w-56'>{t('Channel / model')}</TableHead>
          <TableHead>{t('Billing mode')}</TableHead>
          <TableHead className='text-right'>{t('Input tokens')}</TableHead>
          <TableHead className='text-right'>{t('Cache read tokens')}</TableHead>
          <TableHead className='text-right'>
            {t('Cache write tokens')}
          </TableHead>
          <TableHead className='text-right'>{t('Output tokens')}</TableHead>
          <TableHead className='text-right'>{t('Billable calls')}</TableHead>
          <TableHead>{t('Data status')}</TableHead>
          <TableHead className='text-right'>{t('Actions')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.channels.flatMap((channel) => {
          const isExpanded = props.expandedChannels.has(channel.channel_id)
          return [
            <ChannelRow
              key={`channel-${channel.channel_id}`}
              channel={channel}
              expanded={isExpanded}
              onToggle={() => props.onToggleChannel(channel.channel_id)}
            />,
            ...(isExpanded
              ? channel.models.map((model, modelIndex) => (
                  <ModelRow
                    key={`model-${channel.channel_id}-${model.provider_model}-${model.billing_mode}-${model.provider_model_fallback ? 'fallback' : 'provider'}`}
                    last={modelIndex === channel.models.length - 1}
                    model={model}
                  />
                ))
              : []),
          ]
        })}
      </TableBody>
    </Table>
  )
}

function ChannelRow(props: {
  channel: ProviderChannelSummary
  expanded: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  const usage = props.channel.usage
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
            onClick={props.onToggle}
          >
            <HugeiconsIcon
              icon={props.expanded ? ArrowDown01Icon : ArrowRight01Icon}
              strokeWidth={2}
            />
          </Button>
          <div>
            <div className='font-semibold'>{props.channel.channel_name}</div>
            <div className='text-muted-foreground text-xs'>
              #{props.channel.channel_id} ·{' '}
              {t('{{count}} models', { count: props.channel.models.length })}
            </div>
          </div>
        </div>
      </TableCell>
      <TableCell>—</TableCell>
      <UsageCell value={usage.input_tokens} parent />
      <UsageCell value={usage.cache_read_tokens} parent />
      <UsageCell value={usage.cache_write_tokens} parent />
      <UsageCell value={usage.output_tokens} parent />
      <UsageCell value={usage.billable_calls} parent />
      <TableCell>
        <QualityBadge quality={props.channel.data_quality} />
      </TableCell>
      <TableCell />
    </TableRow>
  )
}

function ModelRow(props: { last: boolean; model: ProviderModelSummary }) {
  const { t } = useTranslation()
  const detailParams = new URLSearchParams({
    channel: String(props.model.channel_id),
    startTime: String(props.model.detail_filter.start_timestamp * 1000),
    endTime: String(props.model.detail_filter.end_timestamp * 1000),
  })
  if (props.model.detail_filter.model_name) {
    detailParams.set('model', props.model.detail_filter.model_name)
  }
  const tokenBilling = props.model.billing_mode === 'token'
  const perCallBilling = props.model.billing_mode === 'per_call'
  return (
    <TableRow>
      <TableCell className='relative pl-14'>
        <span
          aria-hidden='true'
          className={
            props.last
              ? 'border-muted-foreground/25 absolute top-0 bottom-1/2 left-7 w-5 border-b border-l'
              : 'border-muted-foreground/25 absolute top-0 bottom-0 left-7 w-5 border-b border-l'
          }
        />
        <span className='font-medium'>{props.model.provider_model}</span>
      </TableCell>
      <TableCell>
        <Badge variant='secondary'>
          {t(billingModeLabel(props.model.billing_mode))}
        </Badge>
      </TableCell>
      <UsageCell value={tokenBilling ? props.model.usage.input_tokens : null} />
      <UsageCell
        value={tokenBilling ? props.model.usage.cache_read_tokens : null}
      />
      <UsageCell
        value={tokenBilling ? props.model.usage.cache_write_tokens : null}
      />
      <UsageCell
        value={tokenBilling ? props.model.usage.output_tokens : null}
      />
      <UsageCell
        value={perCallBilling ? props.model.usage.billable_calls : null}
      />
      <TableCell>
        <QualityBadge quality={props.model.data_quality} />
      </TableCell>
      <TableCell className='text-right'>
        <a
          className={buttonVariants({ variant: 'link', size: 'xs' })}
          href={`/usage-logs/common?${detailParams.toString()}`}
        >
          {t('View details')}
          <HugeiconsIcon
            icon={LinkSquare02Icon}
            strokeWidth={2}
            data-icon='inline-end'
          />
        </a>
      </TableCell>
    </TableRow>
  )
}

function UsageCell(props: { value: number | null; parent?: boolean }) {
  const { i18n } = useTranslation()
  return (
    <TableCell
      className={props.parent ? 'text-right font-semibold' : 'text-right'}
    >
      {formatStatementUsage(props.value, i18n.language)}
    </TableCell>
  )
}

function QualityBadge(props: { quality?: BillingDataQuality }) {
  const { t } = useTranslation()
  let variant: 'secondary' | 'warning' | 'outline' = 'secondary'
  if (props.quality?.status === 'partial') variant = 'warning'
  if (props.quality?.status === 'unavailable') variant = 'outline'
  const reasons = billingDataQualityReasons(props.quality, t)
  const badge = (
    <Badge
      variant={variant}
      className={reasons.length > 0 ? 'cursor-help' : undefined}
      tabIndex={reasons.length > 0 ? 0 : undefined}
    >
      {billingDataQualityLabel(props.quality, t)}
    </Badge>
  )
  if (reasons.length === 0) return badge
  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger render={badge} />
        <TooltipContent className='max-w-sm items-start'>
          <div className='space-y-1.5'>
            <p className='font-medium'>{t('Partial data reasons')}</p>
            <ul className='list-disc space-y-1 pl-4'>
              {reasons.map((reason) => (
                <li key={reason}>{reason}</li>
              ))}
            </ul>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
