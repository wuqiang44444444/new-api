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

import { billingModeLabel } from '../lib'
import type {
  ProviderUrlChannelSummary,
  ProviderUrlGroupSummary,
  ProviderUrlModelSummary,
} from '../types'
import { upstreamModelLabel } from '../upstream-statement-utils'
import { upstreamUrlGroupLabel } from '../upstream-url-statement-utils'
import { QualityBadge, UsageCell } from './upstream-statement-table'

type UpstreamUrlStatementTableProps = {
  groups: ProviderUrlGroupSummary[]
  expandedGroups: Set<string>
  expandedModels: Set<string>
  onToggleGroup: (urlKey: string) => void
  onToggleModel: (modelKey: string) => void
}

export function UpstreamUrlStatementTable(
  props: UpstreamUrlStatementTableProps
) {
  const { t } = useTranslation()
  return (
    <Table className='min-w-260'>
      <TableHeader>
        <TableRow>
          <TableHead className='min-w-56'>
            {t('Upstream base URL / model / channel')}
          </TableHead>
          <TableHead>{t('Billing mode')}</TableHead>
          <TableHead className='text-right'>{t('Requests')}</TableHead>
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
        {props.groups.flatMap((group) => {
          const isExpanded = props.expandedGroups.has(group.url_key)
          return [
            <UrlGroupRow
              key={`group-${group.url_key}`}
              group={group}
              expanded={isExpanded}
              onToggle={() => props.onToggleGroup(group.url_key)}
            />,
            ...(isExpanded
              ? group.models.map((model) => {
                  const modelKey = urlModelKey(group.url_key, model)
                  const modelExpanded = props.expandedModels.has(modelKey)
                  return [
                    <UrlModelRow
                      key={`model-${modelKey}`}
                      model={model}
                      expanded={modelExpanded}
                      onToggle={() => props.onToggleModel(modelKey)}
                    />,
                    ...(modelExpanded
                      ? model.channels.map((channel, index) => (
                          <UrlChannelRow
                            key={`channel-${modelKey}-${channel.channel_id}-${model.billing_mode}`}
                            channel={channel}
                            model={model}
                            last={index === model.channels.length - 1}
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

function UrlGroupRow(props: {
  group: ProviderUrlGroupSummary
  expanded: boolean
  onToggle: () => void
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
            <div className='font-semibold wrap-break-word'>
              {upstreamUrlGroupLabel(props.group, t)}
            </div>
            <div className='text-muted-foreground text-xs'>
              {props.group.unidentified
                ? t('Unidentified URL — kept per channel')
                : t('Current channel base URL')}
              {' · '}
              {t('{{count}} channels', { count: props.group.channel_count })}
            </div>
          </div>
        </div>
      </TableCell>
      <TableCell>—</TableCell>
      <UsageCell value={usage.requests} parent />
      <UsageCell value={usage.input_tokens} parent />
      <UsageCell value={usage.cache_read_tokens} parent />
      <UsageCell
        value={usage.cache_write_tokens}
        unavailable={
          !!props.group.data_quality?.cache_write_unavailable_requests
        }
        parent
      />
      <UsageCell value={usage.output_tokens} parent />
      <UsageCell value={usage.billable_calls} parent />
      <TableCell>
        <QualityBadge quality={props.group.data_quality} />
      </TableCell>
      <TableCell />
    </TableRow>
  )
}

function UrlModelRow(props: {
  model: ProviderUrlModelSummary
  expanded: boolean
  onToggle: () => void
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
            aria-label={
              props.expanded ? t('Collapse channels') : t('Expand channels')
            }
            onClick={props.onToggle}
          >
            <HugeiconsIcon
              icon={props.expanded ? ArrowDown01Icon : ArrowRight01Icon}
              strokeWidth={2}
            />
          </Button>
          <span className='inline-block max-w-lg font-medium wrap-break-word whitespace-normal'>
            {props.model.provider_model_fallback
              ? '—'
              : props.model.provider_model}
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
        value={tokenBilling ? props.model.usage.cache_write_tokens : null}
        unavailable={
          tokenBilling &&
          !!props.model.data_quality?.cache_write_unavailable_requests
        }
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
      <TableCell />
    </TableRow>
  )
}

function UrlChannelRow(props: {
  channel: ProviderUrlChannelSummary
  model: ProviderUrlModelSummary
  last: boolean
}) {
  const { t } = useTranslation()
  const detailParams = new URLSearchParams({
    channel: String(props.channel.detail_filter.channel_id),
    startTime: String(props.channel.detail_filter.start_timestamp * 1000),
    endTime: String(props.channel.detail_filter.end_timestamp * 1000),
  })
  if (props.channel.detail_filter.model_name) {
    detailParams.set('model', props.channel.detail_filter.model_name)
  }
  const tokenBilling = props.model.billing_mode === 'token'
  const perCallBilling = props.model.billing_mode === 'per_call'
  return (
    <TableRow>
      <TableCell className='relative pl-24'>
        <span
          aria-hidden='true'
          className={
            props.last
              ? 'border-muted-foreground/25 absolute top-0 bottom-1/2 left-14 w-5 border-b border-l'
              : 'border-muted-foreground/25 absolute top-0 bottom-0 left-14 w-5 border-b border-l'
          }
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
        value={tokenBilling ? props.channel.usage.cache_write_tokens : null}
        unavailable={
          tokenBilling &&
          !!props.channel.data_quality?.cache_write_unavailable_requests
        }
      />
      <UsageCell
        value={tokenBilling ? props.channel.usage.output_tokens : null}
      />
      <UsageCell
        value={perCallBilling ? props.channel.usage.billable_calls : null}
      />
      <TableCell>
        <QualityBadge quality={props.channel.data_quality} />
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
