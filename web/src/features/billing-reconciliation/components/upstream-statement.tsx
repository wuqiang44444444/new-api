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
import { DownloadIcon, InformationCircleIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampToDate } from '@/lib/format'

import { getAdminUpstreamStatement } from '../api'
import {
  billingDataQualityLabel,
  downloadUpstreamStatementCsv,
  filterProviderChannels,
} from '../upstream-statement-utils'
import { UpstreamStatementTable } from './upstream-statement-table'

type UpstreamStatementProps = {
  month: string
  onMonthChange: (month: string) => void
  period: { start_timestamp: number; end_timestamp: number }
}

export function UpstreamStatementView(props: UpstreamStatementProps) {
  const { t } = useTranslation()
  const [channelFilter, setChannelFilter] = useState('all')
  const [modelFilter, setModelFilter] = useState('all')
  const [expandedChannels, setExpandedChannels] = useState<Set<number> | null>(
    null
  )
  const query = useQuery({
    queryKey: [
      'billing-upstream-reconciliation',
      props.period.start_timestamp,
      props.period.end_timestamp,
    ],
    queryFn: async () => {
      const response = await getAdminUpstreamStatement(props.period)
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load upstream reconciliation.')
        )
      }
      return response.data
    },
    staleTime: 30_000,
    retry: false,
  })

  const channels = useMemo(
    () => query.data?.result.channels ?? [],
    [query.data?.result.channels]
  )
  const channelItems = useMemo(
    () => [
      { value: 'all', label: t('All channels') },
      ...channels.map((channel) => ({
        value: String(channel.channel_id),
        label: channel.channel_name,
      })),
    ],
    [channels, t]
  )
  const modelItems = useMemo(() => {
    const models = new Set<string>()
    for (const channel of channels) {
      for (const model of channel.models) models.add(model.provider_model)
    }
    return [
      { value: 'all', label: t('All models') },
      ...[...models].sort().map((model) => ({ value: model, label: model })),
    ]
  }, [channels, t])
  const visibleChannels = useMemo(
    () =>
      filterProviderChannels(channels, {
        channel: channelFilter,
        model: modelFilter,
      }),
    [channelFilter, channels, modelFilter]
  )
  const defaultExpandedChannels = useMemo(
    () => new Set(visibleChannels.slice(0, 2).map((item) => item.channel_id)),
    [visibleChannels]
  )
  const effectiveExpandedChannels = expandedChannels ?? defaultExpandedChannels

  if (query.isError) {
    return (
      <ErrorState
        title={t('Unable to load upstream reconciliation')}
        description={
          query.error instanceof Error
            ? query.error.message
            : t('Please try again later.')
        }
        onRetry={() => query.refetch()}
      />
    )
  }
  if (query.isPending || !query.data) {
    return <Skeleton className='h-96 rounded-xl' />
  }

  const modelCount = channels.reduce(
    (total, channel) => total + channel.models.length,
    0
  )
  const qualityLabel = billingDataQualityLabel(
    query.data.result.data_quality,
    t
  )
  const handleExport = () => {
    try {
      downloadUpstreamStatementCsv({
        channels: visibleChannels,
        generatedAt: query.data.generated_at,
        month: props.month,
        t,
      })
      toast.success(t('Statement CSV exported.'))
    } catch {
      toast.error(t('Unable to export statement.'))
    }
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between'>
        <div className='grid flex-1 gap-3 sm:grid-cols-3 xl:max-w-3xl'>
          <Field>
            <FieldLabel htmlFor='upstream-billing-month'>
              {t('Billing month')}
            </FieldLabel>
            <Input
              id='upstream-billing-month'
              type='month'
              value={props.month}
              onChange={(event) => props.onMonthChange(event.target.value)}
            />
          </Field>
          <StatementSelect
            id='upstream-channel-filter'
            label={t('Upstream channel')}
            items={channelItems}
            value={channelFilter}
            onValueChange={(value) => {
              setChannelFilter(value)
              setExpandedChannels(null)
            }}
          />
          <StatementSelect
            id='upstream-model-filter'
            label={t('Model')}
            items={modelItems}
            value={modelFilter}
            onValueChange={(value) => {
              setModelFilter(value)
              setExpandedChannels(null)
            }}
          />
        </div>
        <Button disabled={visibleChannels.length === 0} onClick={handleExport}>
          <HugeiconsIcon
            icon={DownloadIcon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Export our statement')}
        </Button>
      </div>

      <div className='text-muted-foreground text-sm'>
        {t(
          '{{channels}} channels · {{models}} models · {{quality}} · Generated at {{time}} (Asia/Shanghai)',
          {
            channels: channels.length,
            models: modelCount,
            quality: qualityLabel,
            time: formatTimestampToDate(query.data.generated_at),
          }
        )}
      </div>

      {query.data.result.data_quality?.status === 'partial' ? (
        <Alert>
          <HugeiconsIcon icon={InformationCircleIcon} strokeWidth={2} />
          <AlertTitle>{t('Partial data')}</AlertTitle>
          <AlertDescription>
            {t(
              'Some usage details are unavailable or no longer match the settled statement snapshot. Refresh before exporting.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t('Channel statement details')}</CardTitle>
        </CardHeader>
        <CardContent className='px-0'>
          {visibleChannels.length === 0 ? (
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
            <UpstreamStatementTable
              channels={visibleChannels}
              expandedChannels={effectiveExpandedChannels}
              onToggleChannel={(channelId) =>
                setExpandedChannels((current) =>
                  toggleSet(current ?? defaultExpandedChannels, channelId)
                )
              }
            />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function StatementSelect(props: {
  id: string
  items: Array<{ value: string; label: string }>
  label: string
  onValueChange: (value: string) => void
  value: string
}) {
  return (
    <Field>
      <FieldLabel htmlFor={props.id}>{props.label}</FieldLabel>
      <Select
        items={props.items}
        value={props.value}
        onValueChange={(value) => value != null && props.onValueChange(value)}
      >
        <SelectTrigger id={props.id} className='w-full'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent align='start'>
          <SelectGroup>
            {props.items.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  )
}

function toggleSet(current: Set<number>, value: number) {
  const next = new Set(current)
  if (next.has(value)) next.delete(value)
  else next.add(value)
  return next
}
