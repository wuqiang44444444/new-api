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
import { hashKey, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { ResultPagination } from '@/components/data-table/core/result-pagination'
import { ErrorState } from '@/components/error-state'
import { PageFooterPortal } from '@/components/layout/components/page-footer'
import { Button } from '@/components/ui/button'
import {
  getAdminPlans,
  getSelfSubscriptionFull,
} from '@/features/subscriptions/api'
import { useMediaQuery } from '@/hooks'
import { useStatus } from '@/hooks/use-status'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { formatTimestampToDate } from '@/lib/format'
import { ROLE } from '@/lib/roles'
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
import { fetchLogsByCategory, getDefaultTimeRange } from '../lib/utils'
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

function legacyLogWindow(
  search: Record<string, unknown>,
  maxLookupDays: number,
  previous?: NonNullable<GetLogsResponse['data']>
) {
  if (previous?.end_timestamp) {
    return {
      startTime: previous.start_timestamp
        ? previous.start_timestamp * 1000
        : 1000,
      endTime: previous.end_timestamp * 1000,
    }
  }
  const { start, end } = getDefaultTimeRange(true)
  const endTime =
    typeof search.endTime === 'number' ? search.endTime : end.getTime()
  return {
    startTime:
      search.startTime ??
      (search.endTime != null || !search.requestId
        ? endTime - (end.getTime() - start.getTime())
        : Math.max(1000, endTime - maxLookupDays * 86_400_000)),
    endTime,
  }
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
  const { status, statusConfirmed, error: statusError } = useStatus()
  const statusReady = statusConfirmed || !!statusError
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
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
    search: searchParams,
    navigate,
    pagination: {
      defaultPage: 1,
      defaultPageSize:
        isMobile || (logCategory === 'common' && !searchParams.billing)
          ? 20
          : 100,
    },
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
  const billingMode = isCommon && !!searchParams.billing
  const numberedLogs = isCommon && !billingMode
  const deferredTotal =
    numberedLogs &&
    statusConfirmed &&
    status?.log_deferred_total_enabled === true
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const maxLookupDays =
    role >= ROLE.ADMIN
      ? Infinity
      : Math.max(1, status?.log_max_range_days ?? 365)
  const pageIndex = pagination.pageIndex
  const directBillingEvents =
    billingMode && searchParams.billingView === 'events'
  const {
    page: _page,
    pageSize: _pageSize,
    upstreamRequestId,
    ...filters
  } = searchParams
  const effectiveUpstreamRequestId = isAdmin ? upstreamRequestId : undefined
  const billingFilterKey = hashKey([filters, effectiveUpstreamRequestId])
  const logSnapshots = useRef(new Map<string, string>())
  const legacyPagination = useRef(
    new Map<string, ReturnType<typeof legacyLogWindow> | undefined>()
  )
  const [legacyTotalScope, setLegacyTotalScope] = useState<string>()
  const resetLogSnapshot = useCallback(() => {
    logSnapshots.current.clear()
    setLegacyTotalScope(undefined)
    for (const scope of legacyPagination.current.keys()) {
      legacyPagination.current.set(scope, undefined)
    }
  }, [])
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
    !effectiveUpstreamRequestId
  const listScope = [
    logCategory,
    viewAccess,
    userId,
    groupFunds,
    fundGroup,
    deferredTotal,
    billingFilterKey,
    numberedLogs ? null : columnFilters,
  ]
  const listScopeKey = hashKey(listScope)
  const { data, isLoading, isFetching, isError, isPlaceholderData, refetch } =
    useQuery<NonNullable<GetLogsResponse['data']>>({
      enabled: !numberedLogs || statusReady,
      ...(numberedLogs ? { retry: false } : {}),
      queryKey: ['logs', listScopeKey, pageIndex, pagination.pageSize],
      ...(numberedLogs ? { staleTime: 10_000 } : {}),
      queryFn: async ({ signal }) => {
        let legacyWindow = legacyPagination.current.get(listScopeKey)
        const useSnapshot =
          deferredTotal && !legacyPagination.current.has(listScopeKey)
        if (!legacyWindow && numberedLogs && !useSnapshot) {
          legacyWindow = legacyLogWindow(searchParams, maxLookupDays)
          legacyPagination.current.set(listScopeKey, legacyWindow)
        }
        const pageSnapshot = useSnapshot
          ? (logSnapshots.current.get(listScopeKey) ?? 'start')
          : undefined
        const config = {
          logCategory,
          isAdmin,
          page: pageIndex + 1,
          groupFunds,
          fundGroup,
          signal,
          pageSize: pagination.pageSize,
          searchParams: {
            ...searchParams,
            ...legacyWindow,
          },
          columnFilters: numberedLogs ? [] : columnFilters,
        }
        const pageTotal = deferredTotal && useSnapshot ? 'defer' : undefined
        let result = await fetchLogsByCategory({
          ...config,
          pageSnapshot,
          pageTotal,
        })
        if (
          pageTotal &&
          result.code === 'log_console_route_unavailable' &&
          !signal.aborted
        ) {
          const previous = queryClient
            .getQueriesData<NonNullable<GetLogsResponse['data']>>({
              queryKey: ['logs', listScopeKey],
            })
            .find(([, cached]) => cached?.page_snapshot === pageSnapshot)?.[1]
          const window = legacyLogWindow(searchParams, maxLookupDays, previous)
          legacyPagination.current.set(listScopeKey, window)
          logSnapshots.current.delete(listScopeKey)
          queryClient.removeQueries({
            queryKey: ['logs', listScopeKey],
            type: 'inactive',
          })
          result = await fetchLogsByCategory({
            ...config,
            searchParams: { ...searchParams, ...window },
          })
        }

        if (!result?.success) {
          throw createServerError(result, 'Failed to load logs')
        }

        if (
          !legacyPagination.current.has(listScopeKey) &&
          result.data?.page_snapshot &&
          !signal.aborted
        ) {
          if (pageSnapshot !== result.data.page_snapshot) {
            queryClient.removeQueries({
              queryKey: ['logs', listScopeKey],
              type: 'inactive',
            })
          }
          logSnapshots.current.set(listScopeKey, result.data.page_snapshot)
        }

        return (
          result.data || {
            ...DEFAULT_LOGS_DATA,
            page: 1,
            page_size: pagination.pageSize,
          }
        )
      },
      placeholderData: (previousData, previousQuery) =>
        previousQuery?.queryKey[1] === listScopeKey ? previousData : undefined,
    })

  const positionSnapshot =
    deferredTotal && !isPlaceholderData ? data?.page_snapshot : undefined
  const readLogPosition = async (
    mode: 'count' | 'last',
    signal: AbortSignal
  ) => {
    const result = await fetchLogsByCategory({
      logCategory,
      isAdmin,
      page: 1,
      pageSize: pagination.pageSize,
      pageSnapshot: positionSnapshot,
      pageTotal: mode,
      signal,
      searchParams,
      columnFilters: [],
    })
    if (result.code === 'log_console_route_unavailable') {
      return { kind: 'legacy' } as const
    }
    if (result.code === 'log_snapshot_expired') {
      return { kind: 'expired' } as const
    }
    if (
      !result.success ||
      !result.data?.page_snapshot ||
      result.data.total_pending
    ) {
      throw createServerError(result, 'Failed to load logs')
    }
    return {
      kind: 'position',
      page: result.data,
      snapshot: result.data.page_snapshot,
    } as const
  }
  const positionKey = [
    listScopeKey,
    positionSnapshot,
    pageIndex,
    pagination.pageSize,
  ]
  const count = useQuery({
    enabled: false,
    retry: false,
    queryKey: ['usage-log-total', ...positionKey],
    queryFn: ({ signal }) => readLogPosition('count', signal),
  })
  const last = useQuery({
    enabled: false,
    retry: false,
    queryKey: ['usage-log-last-page', ...positionKey],
    queryFn: ({ signal }) => readLogPosition('last', signal),
  })
  const positionBusy = count.isFetching || last.isFetching
  const positionExpired =
    count.data?.kind === 'expired' || last.data?.kind === 'expired'

  const reloadLogSearch = () => {
    logSnapshots.current.delete(listScopeKey)
    queryClient.removeQueries({
      queryKey: ['logs', listScopeKey],
      type: 'inactive',
    })
    if (pageIndex === 0) void refetch()
    else onPaginationChange({ ...pagination, pageIndex: 0 })
  }

  const requestTotal = async (destination?: number | 'last') => {
    if (!positionSnapshot || positionBusy) return
    const operation = destination === 'last' ? last : count
    const result = await operation.refetch({ cancelRefetch: false })
    if (
      !result.isSuccess ||
      logSnapshots.current.get(listScopeKey) !== positionSnapshot
    ) {
      return
    }
    if (result.data.kind === 'expired') return
    if (result.data.kind === 'legacy') {
      legacyPagination.current.set(
        listScopeKey,
        legacyLogWindow(searchParams, maxLookupDays, data)
      )
      logSnapshots.current.delete(listScopeKey)
      queryClient.removeQueries({
        queryKey: ['logs', listScopeKey],
        type: 'inactive',
      })
      const fallback = await refetch()
      if (
        fallback.isSuccess &&
        !fallback.data.total_pending &&
        destination !== undefined
      ) {
        setLegacyTotalScope(
          hashKey([listScopeKey, pageIndex, pagination.pageSize])
        )
      }
      return
    }

    const page = result.data.page
    const snapshot = result.data.snapshot
    logSnapshots.current.set(listScopeKey, snapshot)
    if (destination === 'last') {
      const targetIndex = page.page - 1
      queryClient.removeQueries({
        queryKey: ['logs', listScopeKey],
        type: 'inactive',
      })
      queryClient.setQueryData(
        ['logs', listScopeKey, targetIndex, pagination.pageSize],
        page
      )
      onPaginationChange({ ...pagination, pageIndex: targetIndex })
      return
    }

    queryClient.setQueriesData<NonNullable<GetLogsResponse['data']>>(
      { queryKey: ['logs', listScopeKey] },
      (previous) =>
        previous?.page_snapshot === positionSnapshot
          ? {
              ...previous,
              total: page.total,
              total_pending: false,
              page_snapshot: snapshot,
            }
          : previous
    )
    if (destination !== undefined) {
      const lastPage = Math.max(1, Math.ceil(page.total / pagination.pageSize))
      onPaginationChange({
        ...pagination,
        pageIndex: Math.min(destination, lastPage) - 1,
      })
    }
  }

  const logs =
    numberedLogs && (isError || isPlaceholderData) ? undefined : data?.items
  const columns = useColumnsByCategory(
    logCategory,
    isAdmin,
    isRoot,
    showBillingSource
  )
  const isLoadingData =
    (numberedLogs && (!statusReady || isPlaceholderData)) ||
    isLoading ||
    (isFetching && !data)

  const hasCurrentPage = data && !isPlaceholderData && !isFetching && !isError
  const ensureLogPageInRange = (pageCount: number) => {
    if (hasCurrentPage && !data?.total_pending) {
      ensurePageInRange(Math.max(1, pageCount), { resetTo: 'last' })
    }
  }

  const { table } = useDataTable({
    data: (logs ?? DEFAULT_LOGS_DATA.items) as Record<string, unknown>[],
    columns: columns as ColumnDef<Record<string, unknown>>[],
    columnFilters,
    columnVisibilityStorageKey: getColumnVisibilityStorageKey(
      logCategory,
      viewAccess
    ),
    pagination,
    enableRowSelection: false,
    onPaginationChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange: numberedLogs ? ensureLogPageInRange : ensurePageInRange,
  })

  if (billingMode && isError) {
    return <ErrorState onRetry={() => void refetch()} />
  }
  if (groupFunds && data?.fund_groups) {
    return (
      <BillingFundGroups
        rows={data.fund_groups}
        quota={data.quota}
        page={pageIndex + 1}
        pageSize={pagination.pageSize}
        total={data.total}
        loading={isFetching}
        onPage={(page, pageSize) =>
          onPaginationChange({ pageIndex: page - 1, pageSize })
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
      showPagination={!numberedLogs}
      afterTable={
        numberedLogs && (
          <PageFooterPortal>
            {isError && (
              <div className='flex items-center justify-end gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={
                    deferredTotal ? reloadLogSearch : () => void refetch()
                  }
                >
                  {t('Retry')}
                </Button>
              </div>
            )}
            {data && !isError && (
              <>
                {legacyTotalScope ===
                  hashKey([listScopeKey, pageIndex, pagination.pageSize]) && (
                  <p
                    role='status'
                    className='text-muted-foreground text-right text-xs'
                  >
                    {t('Total loaded. Select the page again.')}
                  </p>
                )}
                {!data.total_pending && (positionExpired || last.isError) && (
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={
                      positionExpired
                        ? reloadLogSearch
                        : () => void requestTotal('last')
                    }
                  >
                    {positionExpired ? t('Refresh') : t('Retry')}
                  </Button>
                )}
                <ResultPagination
                  label={t('Usage Logs')}
                  page={pageIndex + 1}
                  pageSize={pagination.pageSize}
                  total={data.total}
                  disabled={isFetching || isPlaceholderData || positionBusy}
                  allowPageJump
                  onLastPage={
                    positionSnapshot
                      ? () => void requestTotal('last')
                      : undefined
                  }
                  pendingTotal={
                    data.total_pending
                      ? {
                          hasMore: data.has_more ?? false,
                          busy: positionBusy,
                          failed:
                            count.isError || last.isError || positionExpired,
                          onRefresh: positionExpired
                            ? reloadLogSearch
                            : undefined,
                          onRequest: (destination) =>
                            void requestTotal(destination),
                        }
                      : undefined
                  }
                  onChange={(page, pageSize) =>
                    onPaginationChange({ pageIndex: page - 1, pageSize })
                  }
                />
              </>
            )}
          </PageFooterPortal>
        )
      }
      columns={columns as ColumnDef<Record<string, unknown>>[]}
      isLoading={isLoadingData}
      isFetching={isFetching}
      emptyTitle={
        isError && numberedLogs ? t('Failed to load logs') : t('No Logs Found')
      }
      emptyDescription={
        isError && numberedLogs
          ? ''
          : t(
              'No usage logs available. Logs will appear here once API calls are made.'
            )
      }
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
          emptyTitle={
            isError && numberedLogs ? t('Failed to load logs') : undefined
          }
          emptyDescription={isError && numberedLogs ? '' : undefined}
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
            <>
              <CommonLogsFilterBar
                table={table}
                onSearch={resetLogSnapshot}
                statisticsWindow={
                  data?.end_timestamp && !isPlaceholderData && !isError
                    ? {
                        start: data.start_timestamp ?? 0,
                        end: data.end_timestamp,
                      }
                    : undefined
                }
                statisticsWindowPending={
                  isFetching || isPlaceholderData || !data?.end_timestamp
                }
              />
              {searchParams.requestId &&
                data?.end_timestamp &&
                !isPlaceholderData &&
                !isError && (
                  <p className='text-muted-foreground text-xs'>
                    {data.start_timestamp
                      ? t('Searched {{start}} to {{end}}', {
                          start: formatTimestampToDate(data.start_timestamp),
                          end: formatTimestampToDate(data.end_timestamp),
                        })
                      : t('Searched all logs through {{end}}', {
                          end: formatTimestampToDate(data.end_timestamp),
                        })}
                  </p>
                )}
            </>
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
