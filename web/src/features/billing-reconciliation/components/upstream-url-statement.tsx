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

import { getAdminUpstreamUrlStatement } from '../api'
import { billingDataQualityLabel } from '../upstream-statement-utils'
import {
  downloadUpstreamUrlStatementCsv,
  upstreamUrlGroupLabel,
} from '../upstream-url-statement-utils'
import { UpstreamUrlStatementTable } from './upstream-url-statement-table'

type UpstreamUrlStatementProps = {
  month: string
  onMonthChange: (month: string) => void
  period: { start_timestamp: number; end_timestamp: number }
}

export function UpstreamUrlStatementView(props: UpstreamUrlStatementProps) {
  const { t } = useTranslation()
  const [urlFilter, setUrlFilter] = useState('all')
  const [expandedGroups, setExpandedGroups] = useState<Set<string> | null>(null)
  const [expandedModels, setExpandedModels] = useState<Set<string>>(new Set())

  const allQuery = useQuery({
    queryKey: [
      'billing-upstream-url-reconciliation',
      props.period.start_timestamp,
      props.period.end_timestamp,
    ],
    queryFn: async () => {
      const response = await getAdminUpstreamUrlStatement(props.period)
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

  const hasFilters = urlFilter !== 'all'
  const filteredQuery = useQuery({
    queryKey: [
      'billing-upstream-url-reconciliation',
      props.period.start_timestamp,
      props.period.end_timestamp,
      urlFilter,
    ],
    queryFn: async () => {
      const response = await getAdminUpstreamUrlStatement({
        ...props.period,
        ...(urlFilter !== 'all' && { url_key: urlFilter }),
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Unable to load upstream reconciliation.')
        )
      }
      return response.data
    },
    enabled: hasFilters,
    staleTime: 30_000,
    retry: false,
  })
  const query = hasFilters ? filteredQuery : allQuery

  const groups = useMemo(
    () => allQuery.data?.result.url_groups ?? [],
    [allQuery.data?.result.url_groups]
  )
  const urlItems = useMemo(
    () => [
      { value: 'all', label: t('All upstream URLs') },
      ...groups.map((group) => ({
        value: group.url_key,
        label: upstreamUrlGroupLabel(group, t),
      })),
    ],
    [groups, t]
  )
  const visibleGroups = useMemo(
    () => query.data?.result.url_groups ?? [],
    [query.data?.result.url_groups]
  )
  const defaultExpandedGroups = useMemo(
    () => new Set(visibleGroups.slice(0, 2).map((item) => item.url_key)),
    [visibleGroups]
  )
  const effectiveExpandedGroups = expandedGroups ?? defaultExpandedGroups

  if (allQuery.isError) {
    return (
      <ErrorState
        title={t('Unable to load upstream reconciliation')}
        description={
          allQuery.error instanceof Error
            ? allQuery.error.message
            : t('Please try again later.')
        }
        onRetry={() => allQuery.refetch()}
      />
    )
  }
  if (allQuery.isPending || !allQuery.data) {
    return <Skeleton className='h-96 rounded-xl' />
  }

  const modelCount = visibleGroups.reduce(
    (total, group) => total + group.model_count,
    0
  )
  const qualityLabel = billingDataQualityLabel(
    query.data?.result.data_quality,
    t
  )
  const handleExport = () => {
    if (!query.data || query.isFetching || query.isError) return
    try {
      downloadUpstreamUrlStatementCsv({
        groups: visibleGroups,
        generatedAt: query.data.generated_at,
        month: props.month,
        t,
      })
      toast.success(t('Statement CSV exported.'))
    } catch {
      toast.error(t('Unable to export statement.'))
    }
  }

  let statementContent
  if (query.isError) {
    statementContent = (
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
  } else if (query.isPending) {
    statementContent = (
      <div aria-busy='true'>
        <Skeleton className='h-96 rounded-xl' />
      </div>
    )
  } else if (visibleGroups.length === 0) {
    statementContent = (
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
    )
  } else {
    statementContent = (
      <UpstreamUrlStatementTable
        groups={visibleGroups}
        expandedGroups={effectiveExpandedGroups}
        expandedModels={expandedModels}
        onToggleGroup={(urlKey) =>
          setExpandedGroups((current) =>
            toggleSet(current ?? defaultExpandedGroups, urlKey)
          )
        }
        onToggleModel={(modelKey) =>
          setExpandedModels((current) => toggleSet(current, modelKey))
        }
      />
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between'>
        <div className='grid flex-1 gap-3 sm:grid-cols-2 xl:max-w-xl'>
          <Field>
            <FieldLabel htmlFor='upstream-url-billing-month'>
              {t('Billing month')}
            </FieldLabel>
            <Input
              id='upstream-url-billing-month'
              type='month'
              value={props.month}
              onChange={(event) => props.onMonthChange(event.target.value)}
            />
          </Field>
          <StatementSelect
            id='upstream-url-filter'
            label={t('Upstream base URL')}
            items={urlItems}
            value={urlFilter}
            onValueChange={(value) => {
              setUrlFilter(value)
              setExpandedGroups(null)
              setExpandedModels(new Set())
            }}
          />
        </div>
        <Button
          disabled={
            visibleGroups.length === 0 || query.isFetching || query.isError
          }
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

      {query.data && (
        <div className='text-muted-foreground space-y-1 text-sm'>
          <p>
            {t(
              '{{groups}} URL groups · {{models}} models · {{quality}} · Generated at {{time}} (Asia/Shanghai)',
              {
                groups: visibleGroups.length,
                models: modelCount,
                quality: qualityLabel,
                time: formatTimestampToDate(query.data.generated_at),
              }
            )}
          </p>
          <p className='text-xs'>
            {t(
              "Usage is grouped by each channel's current base URL; regrouping follows later channel edits and does not represent the URL at request time."
            )}
          </p>
        </div>
      )}

      {query.data?.result.data_quality?.status === 'partial' ? (
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
          <CardTitle>{t('Upstream URL statement details')}</CardTitle>
        </CardHeader>
        <CardContent className='px-0'>{statementContent}</CardContent>
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

function toggleSet<T>(current: Set<T>, value: T) {
  const next = new Set(current)
  if (next.has(value)) next.delete(value)
  else next.add(value)
  return next
}
