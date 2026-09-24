import {
  useQueries,
  useQuery,
  type UseQueryResult,
} from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import {
  Accordion,
  AccordionItem,
  AccordionTrigger,
  AccordionContent,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { getBalance, getBalanceConnections, type BalanceResult } from './api'
import {
  BalanceKeyTable,
  type BalanceQueryState,
  type BalanceRow,
} from './key-table'

function combineBalances(
  queries: UseQueryResult<BalanceResult>[]
): BalanceQueryState[] {
  return queries.map((query) => ({
    data: query.data,
    fetching: query.isFetching,
    error: query.isError,
  }))
}

export function UpstreamBalances() {
  const { t } = useTranslation()
  const [filter, setFilter] = useState('')
  const [expanded, setExpanded] = useState<string[]>([])
  const inventory = useQuery({
    queryKey: ['upstream-balance-connections'],
    queryFn: ({ signal }) => getBalanceConnections(signal),
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnMount: 'always',
  })
  const connections = inventory.data
  const balanceQueries = useQueries({
    queries: (connections ?? []).map((connection) => ({
      queryKey: ['upstream-balance', connection.id, inventory.dataUpdatedAt],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        getBalance(connection, signal),
      enabled:
        connection.queryable && !inventory.isFetching && !inventory.isError,
      staleTime: Infinity,
      gcTime: 0,
      retry: false,
      refetchOnWindowFocus: false,
      refetchOnMount: 'always' as const,
    })),
    combine: combineBalances,
  })
  const busy =
    inventory.isFetching || balanceQueries.some((query) => query.fetching)
  const rows = useMemo(
    () =>
      (connections ?? []).map((connection, index) => ({
        ...connection,
        query: balanceQueries[index],
      })),
    [connections, balanceQueries]
  )

  const groups = useMemo(() => {
    const byURL = new Map<
      string,
      { key: string; name: string; url: string; rows: BalanceRow[] }
    >()
    for (const row of rows) {
      let group = byURL.get(row.url_key)
      if (!group) {
        group = {
          key: row.url_key,
          name: row.group_name,
          url: row.url_key.startsWith('channel:') ? '' : row.url_key,
          rows: [],
        }
        byURL.set(row.url_key, group)
      }
      group.rows.push(row)
    }
    const search = filter.trim().toLocaleLowerCase()
    return [...byURL.values()].filter(
      (group) =>
        !search ||
        [
          group.name,
          group.url,
          ...group.rows.flatMap((row) => [
            row.key_label,
            ...row.channels.map((channel) => `${channel.id} ${channel.name}`),
          ]),
        ].some((value) => value.toLocaleLowerCase().includes(search))
    )
  }, [rows, filter])

  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='space-y-1'>
          <h1 className='text-xl font-semibold'>{t('Upstream balances')}</h1>
          <p className='text-muted-foreground max-w-3xl text-sm'>
            {t(
              'Grouped by upstream URL. Expand a group to view each key balance in its original unit. Keys may share an account; amounts are not added together.'
            )}
          </p>
        </div>
        <Button
          variant='outline'
          disabled={busy}
          onClick={() => void inventory.refetch()}
        >
          {busy ? t('Querying balance...') : t('Refresh all balances')}
        </Button>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'With SMTP and report recipients configured, balances are checked every 10 minutes. Below USD 100, email reminders are sent once every 24 hours or after a new drop following recovery. CNY uses the system USD exchange rate; credits and unknown units are excluded.'
        )}
      </p>
      {inventory.isError ? (
        <ErrorState
          title={t('Unable to load upstream connections')}
          description={t(
            'Channel read and operate permissions are required. Check your connection and try again.'
          )}
          onRetry={() => void inventory.refetch()}
        />
      ) : (
        <>
          <Input
            className='max-w-sm'
            value={filter}
            onChange={(event) => setFilter(event.target.value)}
            aria-label={t('Search upstream connections')}
            placeholder={t('Search by upstream name, URL, channel or key...')}
          />
          {inventory.isPending && <LoadingState />}
          {!inventory.isPending && groups.length === 0 && (
            <EmptyState
              title={
                filter
                  ? t('No matching upstream connections')
                  : t('No upstream connections')
              }
              description={
                filter
                  ? t('Try a different upstream name, URL, channel or key.')
                  : t('Add channels to see their upstream balances here.')
              }
            />
          )}
          <Accordion
            multiple
            value={expanded}
            onValueChange={setExpanded}
            className='gap-3'
          >
            {groups.map((group) => (
              <AccordionItem
                key={group.key}
                value={group.key}
                className='rounded-lg border px-4'
              >
                <AccordionTrigger className='min-h-14 items-center gap-3 py-4 hover:no-underline'>
                  <span className='min-w-0 flex-1 space-y-1'>
                    <span className='block font-semibold break-all'>
                      {group.name}
                    </span>
                    {group.url && group.name !== group.url && (
                      <span className='text-muted-foreground block text-xs font-normal break-all'>
                        {group.url}
                      </span>
                    )}
                  </span>
                  <Badge variant='secondary'>
                    {t('{{count}} keys', { count: group.rows.length })}
                  </Badge>
                </AccordionTrigger>
                <AccordionContent>
                  <BalanceKeyTable rows={group.rows} />
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </>
      )}
    </div>
  )
}
