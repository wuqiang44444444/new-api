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
import type { ColumnDef } from '@tanstack/react-table'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  getAdminPlans,
  getSelfSubscriptionFull,
} from '@/features/subscriptions/api'
import { useMediaQuery } from '@/hooks'
import { useStatus } from '@/hooks/use-status'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { createServerError } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import {
  DEFAULT_LOGS_DATA,
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_ENUM,
} from '../constants'
import { shouldShowBillingSource } from '../lib/billing-source'
import { useColumnsByCategory } from '../lib/columns'
import { parseLogOther } from '../lib/format'
import { fetchLogsByCategory } from '../lib/utils'
import type { GetLogsResponse, LogCategory } from '../types'
import { BillingFundGroups } from './billing-fund-groups'
import { CommonLogsFilterBar } from './common-logs-filter-bar'
import { CommonLogsStats } from './common-logs-stats'
import { TaskLogsFilterBar } from './task-logs-filter-bar'
import { UsageLogsMobileList } from './usage-logs-mobile-card'
import { useLogsViewScope, type LogsViewAccess } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const logTypeRowTint: Record<number, string> = {
  [LOG_TYPE_ENUM.ERROR]: 'bg-rose-50/40 dark:bg-rose-950/20',
  [LOG_TYPE_ENUM.REFUND]: 'bg-blue-50/30 dark:bg-blue-950/15',
}

// Warning tint for logs where a quota conversion saturated (admin-only marker).
// Takes precedence over the per-type tint since it flags a billing anomaly.
const quotaSaturationRowTint = 'bg-amber-50/60 dark:bg-amber-950/25'

function getColumnVisibilityStorageKey(
  logCategory: LogCategory,
  viewAccess: LogsViewAccess
): string {
  return `usage-logs:${logCategory}:${viewAccess}:column-visibility`
}

function deserializeLogTypeFilter(value: unknown): unknown[] {
  let values: unknown[] = []
  if (Array.isArray(value)) {
    values = value
  } else if (value) {
    values = [value]
  }
  return values.filter((item) => String(item) !== LOG_TYPE_ALL_VALUE)
}

interface UsageLogsTableProps {
  logCategory: LogCategory
}

export function UsageLogsTable({ logCategory }: UsageLogsTableProps) {
  const { t } = useTranslation()
  const getColumnClassName = useCallback(
    () => (logCategory === 'common' ? 'py-2' : 'py-3.5'),
    [logCategory]
  )
  const {
    isAdminView: isAdmin,
    isRootView: isRoot,
    viewAccess,
  } = useLogsViewScope()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const searchParams = route.useSearch()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const { status, loading: statusLoading } = useStatus()
  const { data: showBillingSource = false } = useQuery({
    queryKey: ['usage-log-billing-source', isAdmin, userId],
    enabled: logCategory === 'common' && userId != null,
    queryFn: async () => {
      if (isAdmin) {
        const plansResult = await getAdminPlans()
        return shouldShowBillingSource({
          isAdmin,
          plans: plansResult.success ? plansResult.data : undefined,
          subscriptions: undefined,
        })
      }

      const selfResult = await getSelfSubscriptionFull()
      return shouldShowBillingSource({
        isAdmin,
        plans: undefined,
        subscriptions: selfResult.success
          ? selfResult.data?.subscriptions
          : undefined,
      })
    },
  })

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 100 },
    globalFilter: { enabled: false },
    columnFilters: [
      {
        columnId: 'created_at',
        searchKey: 'type',
        type: 'array' as const,
        deserialize: deserializeLogTypeFilter,
      },
      { columnId: 'model_name', searchKey: 'model', type: 'string' as const },
      { columnId: 'token_name', searchKey: 'token', type: 'string' as const },
      { columnId: 'group', searchKey: 'group', type: 'string' as const },
      ...(isAdmin
        ? [
            {
              columnId: 'channel',
              searchKey: 'channel',
              type: 'string' as const,
            },
            {
              columnId: 'username',
              searchKey: 'username',
              type: 'string' as const,
            },
          ]
        : []),
    ],
  })

  const isCommon = logCategory === 'common'
  const cursorMode =
    isCommon && !searchParams.billing && status?.log_cursor_enabled !== false
  const filterKey = JSON.stringify([
    viewAccess,
    userId,
    pagination.pageSize,
    columnFilters,
    searchParams,
  ])
  const [navigation, setNavigation] = useState<{
    key: string
    pages: string[]
  }>()
  const cursorPages =
    navigation?.key === filterKey ? navigation.pages : ['start']
  const pageIndex = cursorMode ? cursorPages.length - 1 : pagination.pageIndex
  const pageCursor = cursorMode ? cursorPages[pageIndex] : undefined

  const billingMode = isCommon && !!searchParams.billing
  const directBillingEvents =
    billingMode && searchParams.billingView === 'events'
  const { page: _page, pageSize: _pageSize, ...billingFilters } = searchParams
  const billingFilterKey = hashKey([billingFilters])
  const [fundSelection, setFundSelection] = useState<{
    key: string
    group: string
    page: number
  }>()
  const fundGroup =
    fundSelection?.key === billingFilterKey ? fundSelection.group : undefined
  const groupFunds =
    billingMode &&
    !directBillingEvents &&
    !fundGroup &&
    !searchParams.requestId &&
    !searchParams.upstreamRequestId
  const { data, isLoading, isFetching, isError, refetch } = useQuery<
    NonNullable<GetLogsResponse['data']>
  >({
    enabled: !cursorMode || !statusLoading,
    queryKey: [
      'logs',
      logCategory,
      viewAccess,
      pageIndex + 1,
      pageCursor,
      groupFunds,
      fundGroup,
      userId,
      pagination.pageSize,
      columnFilters,
      searchParams,
      billingMode ? billingFilterKey : null,
      t,
    ],
    queryFn: async ({ signal }) => {
      const result = await fetchLogsByCategory({
        logCategory,
        isAdmin,
        page: pageIndex + 1,
        pageCursor,
        groupFunds,
        fundGroup,
        signal,
        pageSize: pagination.pageSize,
        searchParams,
        columnFilters,
      })

      if (!result?.success) {
        throw createServerError(result, t('Failed to load logs'))
      }

      return (
        result.data || {
          ...DEFAULT_LOGS_DATA,
          page: 1,
          page_size: pagination.pageSize,
        }
      )
    },
    placeholderData: (previousData, previousQuery) => {
      if (
        previousQuery?.queryKey[1] === logCategory &&
        previousQuery.queryKey[2] === viewAccess &&
        previousQuery.queryKey[5] === groupFunds &&
        previousQuery.queryKey[6] === fundGroup &&
        previousQuery.queryKey[7] === userId &&
        previousQuery.queryKey[11] === (billingMode ? billingFilterKey : null)
      ) {
        return previousData
      }
      return undefined
    },
  })

  const logs = data?.items || []
  const columns = useColumnsByCategory(
    logCategory,
    isAdmin,
    isRoot,
    showBillingSource
  )
  const isLoadingData =
    (cursorMode && statusLoading) || isLoading || (isFetching && !data)

  const { table } = useDataTable({
    data: logs as Record<string, unknown>[],
    columns: columns as ColumnDef<Record<string, unknown>>[],
    columnFilters,
    columnVisibilityStorageKey: getColumnVisibilityStorageKey(
      logCategory,
      viewAccess
    ),
    pagination: { ...pagination, pageIndex },
    enableRowSelection: false,
    onPaginationChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange: cursorMode ? undefined : ensurePageInRange,
  })

  if (billingMode && isError) {
    return <ErrorState onRetry={() => void refetch()} />
  }
  if (groupFunds && data?.fund_groups) {
    return (
      <BillingFundGroups
        rows={data.fund_groups}
        quota={data.quota}
        previous={pageIndex > 0}
        next={(pageIndex + 1) * pagination.pageSize < data.total}
        loading={isFetching}
        onPage={(offset) =>
          onPaginationChange({ ...pagination, pageIndex: pageIndex + offset })
        }
        onSelect={(group) => {
          setFundSelection({
            key: billingFilterKey,
            group,
            page: pagination.pageIndex,
          })
          onPaginationChange({ ...pagination, pageIndex: 0 })
        }}
      />
    )
  }
  return (
    <DataTablePage
      table={table}
      compactPagination={isMobile && isCommon}
      showPagination={!cursorMode}
      afterTable={
        cursorMode && (
          <div className='flex shrink-0 items-center justify-end gap-2'>
            {isError && (
              <Button variant='ghost' onClick={() => void refetch()}>
                {t('Retry')}
              </Button>
            )}
            <Button
              variant='outline'
              disabled={pageIndex === 0 || isFetching}
              onClick={() =>
                setNavigation({
                  key: filterKey,
                  pages: cursorPages.slice(0, -1),
                })
              }
            >
              {t('Previous page')}
            </Button>
            <Button
              variant='outline'
              disabled={!data?.next_cursor || isFetching || isError}
              onClick={() => {
                if (data?.next_cursor) {
                  setNavigation({
                    key: filterKey,
                    pages: [...cursorPages, data.next_cursor],
                  })
                }
              }}
            >
              {t('Next page')}
            </Button>
          </div>
        )
      }
      columns={columns as ColumnDef<Record<string, unknown>>[]}
      isLoading={isLoadingData}
      isFetching={isFetching}
      emptyTitle={t('No Logs Found')}
      emptyDescription={t(
        'No usage logs available. Logs will appear here once API calls are made.'
      )}
      skeletonKeyPrefix='usage-log-skeleton'
      applyHeaderSize
      tableClassName={cn(
        '[&_[data-slot=table]]:text-[13px] [&_[data-slot=table]_td]:text-[13px] [&_[data-slot=table]_td_*]:text-[13px] [&_[data-slot=table]_th]:text-[13px] [&_[data-slot=table]_th_*]:text-[13px]'
      )}
      mobile={
        <UsageLogsMobileList
          table={table}
          isLoading={isLoadingData}
          logCategory={logCategory}
        />
      }
      toolbar={
        <>
          {isCommon && searchParams.billing && (
            <div className='space-y-2'>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Customer billing details exclude channel tests and use the statement filters.'
                )}
              </p>
              {fundGroup ? (
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => {
                    onPaginationChange({
                      ...pagination,
                      pageIndex: fundSelection?.page ?? 0,
                    })
                    setFundSelection(undefined)
                  }}
                >
                  {t('Back')}
                </Button>
              ) : (
                !groupFunds && !directBillingEvents && <CommonLogsStats />
              )}
            </div>
          )}
          {isCommon && !searchParams.billing && (
            <CommonLogsFilterBar table={table} />
          )}
          {!isCommon && (
            <TaskLogsFilterBar table={table} logCategory={logCategory} />
          )}
        </>
      }
      renderRow={(row) => {
        const logType = (row.original as Record<string, unknown>).type as
          | number
          | undefined
        let tintClass =
          isCommon && logType != null ? (logTypeRowTint[logType] ?? '') : ''
        if (isCommon && isAdmin) {
          const other = parseLogOther(
            ((row.original as Record<string, unknown>).other as string) ?? ''
          )
          if (other?.admin_info?.quota_saturation) {
            tintClass = quotaSaturationRowTint
          }
        }

        return (
          <DataTableRow
            key={row.id}
            row={row}
            className={cn('transition-colors', tintClass)}
            getColumnClassName={getColumnClassName}
            cellRenderColumns={table.options.columns}
          />
        )
      }}
    />
  )
}
