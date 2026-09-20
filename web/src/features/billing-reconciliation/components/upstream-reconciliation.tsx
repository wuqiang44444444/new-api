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
import { useEffect, useMemo, useRef, useState } from 'react'
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

import {
  getAdminUpstreamReconciliation,
  postAdminUpstreamDiscountInit,
} from '../api'
import {
  downloadUpstreamReconciliationCsv,
  upstreamUrlGroupLabel,
} from '../upstream-reconciliation-utils'
import { billingDataQualityReasons } from '../upstream-statement-utils'
import {
  UpstreamDetailPanel,
  type UpstreamDetailSelection,
} from './upstream-detail-panel'
import { UpstreamDiscountEditor } from './upstream-discount-editor'
import { UpstreamReconciliationTable } from './upstream-reconciliation-table'

type UpstreamReconciliationViewProps = {
  month: string
  onMonthChange: (month: string) => void
  period: { start_timestamp: number; end_timestamp: number }
}

export function UpstreamReconciliationView(
  props: UpstreamReconciliationViewProps
) {
  const { t } = useTranslation()
  const [urlFilter, setUrlFilter] = useState('all')
  const [expandedGroups, setExpandedGroups] = useState<Set<string> | null>(null)
  const [expandedModels, setExpandedModels] = useState<Set<string>>(new Set())
  const [detailSelection, setDetailSelection] =
    useState<UpstreamDetailSelection | null>(null)
  const initializedMonths = useRef<Set<string>>(new Set())

  const query = useQuery({
    queryKey: [
      'billing-upstream-reconciliation',
      props.period.start_timestamp,
      props.period.end_timestamp,
    ],
    queryFn: async () => {
      const response = await getAdminUpstreamReconciliation(props.period)
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

  const groups = useMemo(
    () => query.data?.result.url_groups ?? [],
    [query.data?.result.url_groups]
  )
  const visibleGroups = useMemo(
    () =>
      urlFilter === 'all'
        ? groups
        : groups.filter((group) => group.url_key === urlFilter),
    [groups, urlFilter]
  )
  const defaultExpandedGroups = useMemo(
    () => new Set(visibleGroups.slice(0, 2).map((item) => item.url_key)),
    [visibleGroups]
  )
  const effectiveExpandedGroups = expandedGroups ?? defaultExpandedGroups

  // 月度初始化（方案 3）：进入月份后对当月有用量且待填写的渠道自动触发一次
  // 幂等初始化；失败仅提示，不把未完成初始化当作已确认折扣。
  const pendingChannelIds = useMemo(() => {
    const ids = new Set<number>()
    for (const group of groups) {
      for (const status of group.channel_discounts) {
        if (!status.discount || status.discount.version === 0) {
          ids.add(status.channel_id)
        }
      }
    }
    return [...ids]
  }, [groups])
  useEffect(() => {
    if (!query.data || pendingChannelIds.length === 0) return
    if (initializedMonths.current.has(props.month)) return
    initializedMonths.current.add(props.month)
    postAdminUpstreamDiscountInit({
      period_start: props.period.start_timestamp,
      channel_ids: pendingChannelIds,
    })
      .then((response) => {
        if (!response.success) {
          toast.warning(
            t(
              'Monthly discount initialization failed; pending channels stay pending.'
            )
          )
          return
        }
        const created = response.data?.outcomes.filter(
          (outcome) => outcome.outcome === 'created'
        ).length
        const defaulted = response.data?.outcomes.some(
          (outcome) => outcome.outcome === 'defaulted'
        )
        if (created) {
          toast.success(
            t('Inherited {{count}} channel discounts from last month.', {
              count: created,
            })
          )
        }
        if (created || defaulted) void query.refetch()
      })
      .catch(() => {
        toast.warning(
          t(
            'Monthly discount initialization failed; pending channels stay pending.'
          )
        )
      })
  }, [props.month, pendingChannelIds, props.period.start_timestamp, query, t])

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

  const modelCount = visibleGroups.reduce(
    (total, group) => total + group.model_count,
    0
  )
  const qualityReasons = billingDataQualityReasons(
    query.data.result.data_quality,
    t
  )
  const urlItems = [
    { value: 'all', label: t('All upstream URLs') },
    ...groups.map((group) => ({
      value: group.url_key,
      label: upstreamUrlGroupLabel(group, t),
    })),
  ]
  const handleExport = () => {
    if (!query.data || query.isFetching) return
    try {
      downloadUpstreamReconciliationCsv({
        groups: visibleGroups,
        generatedAt: query.data.generated_at,
        month: props.month,
        period: props.period,
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
        <div className='grid flex-1 gap-3 sm:grid-cols-2 xl:max-w-xl'>
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
          <Field>
            <FieldLabel htmlFor='upstream-url-filter'>
              {t('Upstream base URL')}
            </FieldLabel>
            <Select
              items={urlItems}
              value={urlFilter}
              onValueChange={(value) => {
                if (value == null) return
                setUrlFilter(value)
                setExpandedGroups(null)
                setExpandedModels(new Set())
                setDetailSelection(null)
              }}
            >
              <SelectTrigger id='upstream-url-filter' className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent align='start'>
                <SelectGroup>
                  {urlItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
        </div>
        <Button
          disabled={visibleGroups.length === 0 || query.isFetching}
          onClick={handleExport}
        >
          <HugeiconsIcon
            icon={DownloadIcon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Export our statement')}
        </Button>
      </div>

      <div className='text-muted-foreground space-y-1 text-sm'>
        <p>
          {t(
            '{{groups}} URL groups · {{models}} models · Generated at {{time}} (Asia/Shanghai, {{start}} – {{end}})',
            {
              groups: visibleGroups.length,
              models: modelCount,
              time: formatTimestampToDate(query.data.generated_at),
              start: formatTimestampToDate(props.period.start_timestamp),
              end: formatTimestampToDate(props.period.end_timestamp),
            }
          )}
        </p>
        <p className='text-xs'>
          {t(
            "Usage is grouped by each channel's current base URL and covers every recorded upstream call; the official-price amount only covers rows with customer settlement (channel tests excluded), so the two scopes differ on purpose."
          )}
        </p>
        <p className='text-xs'>
          {t(
            'Reference amounts are original price times the channel-month coefficient under the same-official-price premise; they are not verified supplier costs and never affect customer charges.'
          )}
        </p>
      </div>

      {qualityReasons.length > 0 ? (
        <Alert>
          <HugeiconsIcon icon={InformationCircleIcon} strokeWidth={2} />
          <AlertTitle>{t('Partial data')}</AlertTitle>
          <AlertDescription>
            <ul className='list-disc space-y-1 pl-4 text-xs'>
              {qualityReasons.map((reason) => (
                <li key={reason}>{reason}</li>
              ))}
            </ul>
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
            <Card key={group.url_key}>
              <CardHeader>
                <CardTitle className='wrap-break-word'>
                  {upstreamUrlGroupLabel(group, t)}
                </CardTitle>
              </CardHeader>
              <CardContent className='space-y-4'>
                <UpstreamDiscountEditor
                  discounts={group.channel_discounts}
                  month={props.month}
                  periodStart={props.period.start_timestamp}
                  urlKey={group.url_key}
                  onChanged={() => void query.refetch()}
                />
                <UpstreamReconciliationTable
                  expandedGroups={effectiveExpandedGroups}
                  expandedModels={expandedModels}
                  groups={[group]}
                  onToggleGroup={(urlKey) =>
                    setExpandedGroups((current) =>
                      toggleSet(current ?? defaultExpandedGroups, urlKey)
                    )
                  }
                  onToggleModel={(modelKey) =>
                    setExpandedModels((current) => toggleSet(current, modelKey))
                  }
                  onViewDetails={(selection) => setDetailSelection(selection)}
                />
              </CardContent>
            </Card>
          ))}
        </>
      )}

      {detailSelection ? (
        <Card>
          <CardHeader>
            <CardTitle>{t('Upstream evidence details')}</CardTitle>
          </CardHeader>
          <CardContent>
            <UpstreamDetailPanel
              key={`${props.month}:${JSON.stringify(detailSelection)}`}
              onClose={() => setDetailSelection(null)}
              period={props.period}
              selection={detailSelection}
            />
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}

function toggleSet<T>(current: Set<T>, value: T) {
  const next = new Set(current)
  if (next.has(value)) next.delete(value)
  else next.add(value)
  return next
}
