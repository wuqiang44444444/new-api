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
import { hashKey, useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatLogQuota } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getLogStats, getUserLogStats } from '../api'
import { DEFAULT_LOG_STATS } from '../constants'
import { buildApiParams } from '../lib/utils'
import type { GetLogStatsParams } from '../types'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function StatBadge(props: {
  label: string
  value: string | number
  accent: string
}) {
  return (
    <span className='border-border/60 bg-muted/25 inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs shadow-xs'>
      <span className={cn('h-3.5 w-0.5 rounded-full', props.accent)} />
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
        {props.value}
      </span>
    </span>
  )
}

export function CommonLogsStats(props: {
  window?: { start: number; end: number }
  windowPending?: boolean
}) {
  const {
    page: _page,
    pageSize: _pageSize,
    ...searchParams
  } = route.useSearch()
  const { viewAccess, isAdminView } = useLogsViewScope()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const scope = hashKey([userId, viewAccess, searchParams, props.window])
  return (
    <RangeStatistics
      key={searchParams.billing ? 'billing' : scope}
      scope={scope}
      isAdmin={isAdminView}
      searchParams={searchParams}
      window={props.window}
      windowPending={props.windowPending}
    />
  )
}

function RangeStatistics({
  scope,
  isAdmin,
  searchParams,
  window,
  windowPending,
}: {
  scope: string
  isAdmin: boolean
  searchParams: Omit<ReturnType<typeof route.useSearch>, 'page' | 'pageSize'>
  window?: { start: number; end: number }
  windowPending?: boolean
}) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useUsageLogsContext()
  const billing = !!searchParams.billing
  const [requested, setRequested] = useState(false)
  const {
    data: stats,
    isFetching,
    isError,
    refetch,
  } = useQuery({
    enabled: billing,
    ...(billing ? {} : { retry: false }),
    queryKey: ['usage-logs-stats', scope],
    queryFn: async ({ signal }) => {
      const params: GetLogStatsParams = buildApiParams({
        page: 1,
        pageSize: 1,
        searchParams,
        columnFilters: [],
        isAdmin,
      })

      if (!searchParams.billing) {
        if (!window) throw new Error('Missing usage log statistics window')
        params.range_count = true
        params.start_timestamp = window.start
        params.end_timestamp = window.end
        params.resolved_log_range = true
      }
      const result = isAdmin
        ? requireServerSuccess(await getLogStats(params, signal))
        : requireServerSuccess(await getUserLogStats(params, signal))

      return result.success
        ? result.data || DEFAULT_LOG_STATS
        : DEFAULT_LOG_STATS
    },
    placeholderData: billing ? (previousData) => previousData : undefined,
  })

  const showRetry = (billing || requested) && isError && !isFetching
  if ((!billing && !requested) || showRetry) {
    return (
      <div className='flex flex-wrap items-center gap-2'>
        <Button
          variant='outline'
          size='sm'
          disabled={windowPending || (!billing && !window)}
          onClick={() => {
            setRequested(true)
            void refetch({ cancelRefetch: false })
          }}
        >
          {showRetry ? t('Retry') : t('View range statistics')}
        </Button>
        {!billing && showRetry && (
          <span role='alert' className='text-destructive text-xs'>
            {t('Range statistics are temporarily unavailable.')}
          </span>
        )}
      </div>
    )
  }

  if (billing ? !stats && isFetching : isFetching) {
    return (
      <div className='flex items-center gap-2' aria-busy='true'>
        <Skeleton className='h-7 w-[150px] rounded-md' />
        <Skeleton className='h-7 w-[100px] rounded-md' />
        <Skeleton className='h-7 w-[120px] rounded-md' />
      </div>
    )
  }

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <StatBadge
        label={searchParams.billing ? t('Net amount') : t('Usage')}
        value={sensitiveVisible ? formatLogQuota(stats?.quota || 0) : '••••'}
        accent='bg-sky-500/70'
      />
      {stats && 'total' in stats && typeof stats.total === 'number' && (
        <StatBadge
          label={t('Total Count')}
          value={stats.total}
          accent='bg-slate-400/70'
        />
      )}
      <StatBadge
        label={t('RPM')}
        value={stats?.rpm || 0}
        accent='bg-rose-500/65'
      />
      <StatBadge
        label={t('TPM')}
        value={stats?.tpm || 0}
        accent='bg-slate-400/70'
      />
    </div>
  )
}
