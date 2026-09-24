import { useQueryClient } from '@tanstack/react-query'
import {
  getCoreRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'

import type { BalanceConnection, BalanceResult } from './api'

export type BalanceQueryState = {
  data?: BalanceResult
  fetching: boolean
  error: boolean
}
export type BalanceRow = BalanceConnection & { query?: BalanceQueryState }

export function BalanceKeyTable(props: { rows: BalanceRow[] }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const reasons = useMemo<Record<string, string>>(
    () => ({
      not_connected: t('No verified balance API is connected'),
      unsupported_credentials: t(
        'The channel credentials cannot query account balances'
      ),
      missing_key: t('No channel key is configured'),
      invalid_connection: t('The channel connection settings are invalid'),
      connection_changed: t(
        'The channel changed. Refresh the connection list.'
      ),
      unlimited_key: t('Unlimited key quota; account balance unavailable'),
      account_balance_unavailable: t(
        'Account balance unavailable; the upstream only returned a quota limit'
      ),
      invalid_response: t('The upstream did not return a valid balance'),
      authentication_failed: t('The upstream rejected the channel credentials'),
      endpoint_unavailable: t('The balance endpoint is unavailable'),
      rate_limited: t('The upstream rate limit was reached'),
      upstream_error: t('The upstream balance query failed'),
      network_error: t('Unable to connect to the upstream'),
      timeout: t('The balance query timed out'),
    }),
    [t]
  )

  const columns = useMemo<ColumnDef<BalanceRow, unknown>[]>(
    () => [
      {
        accessorKey: 'key_label',
        header: t('Key'),
        cell: ({ row }) => (
          <code className='text-xs'>{row.original.key_label}</code>
        ),
      },
      {
        id: 'channels',
        accessorFn: (row) =>
          row.channels
            .map((channel) => `${channel.id} ${channel.name}`)
            .join(' '),
        header: t('Associated channels'),
        cell: ({ row }) => (
          <div className='space-y-1'>
            {row.original.channels.map((channel) => (
              <div
                key={`${channel.id}-${channel.key_index}`}
                className='flex flex-wrap items-center gap-1 text-xs'
              >
                <span className='break-all'>
                  #{channel.id} {channel.name}
                </span>
                {!channel.enabled && (
                  <Badge variant='outline'>{t('Disabled')}</Badge>
                )}
              </div>
            ))}
          </div>
        ),
      },
      {
        id: 'amount',
        header: t('Upstream balance'),
        cell: ({ row }) => {
          const query = row.original.query
          const result = query?.data
          if (query?.fetching || query?.error || result?.status !== 'ok') {
            return <span className='text-muted-foreground'>—</span>
          }
          return (
            <div className='space-y-1'>
              {result.amounts?.map((amount) => (
                <div
                  key={`${amount.unit}:${amount.category ?? ''}`}
                  className='font-mono tabular-nums'
                >
                  <span className='font-semibold break-all'>
                    {amount.amount}
                  </span>{' '}
                  <span className='text-muted-foreground text-xs'>
                    {amount.unit === 'credits'
                      ? t('Credits')
                      : amount.unit || t('Unit unconfirmed')}
                    {amount.category ? ` · ${amount.category}` : ''}
                  </span>
                </div>
              ))}
            </div>
          )
        },
      },
      {
        id: 'status',
        header: t('Query status'),
        cell: ({ row }) => {
          const { query, reason, queryable } = row.original
          const result = query?.data
          if (queryable && (query?.fetching || (!result && !query?.error))) {
            return <Badge variant='secondary'>{t('Querying balance...')}</Badge>
          }
          if (query?.error) {
            return (
              <span className='text-destructive text-xs'>
                {t('Balance query failed. Try refreshing.')}
              </span>
            )
          }
          if (result?.status === 'ok') {
            return (
              <div className='space-y-1'>
                <Badge variant='secondary'>{t('Balance retrieved')}</Badge>
                {result.scope === 'unconfirmed' && (
                  <p className='text-muted-foreground text-xs'>
                    {t('Account or key scope unconfirmed')}
                  </p>
                )}
              </div>
            )
          }
          return (
            <p
              className={
                result?.status === 'error'
                  ? 'text-destructive text-xs'
                  : 'text-muted-foreground text-xs'
              }
            >
              {reasons[result?.reason ?? reason ?? ''] ??
                t('Balance unavailable')}
            </p>
          )
        },
      },
      {
        id: 'checked_at',
        header: t('Last checked'),
        cell: ({ row }) => {
          const checked = row.original.query?.data?.checked_at
          return (
            <span className='text-muted-foreground text-xs'>
              {checked ? formatTimestampToDate(checked) : '—'}
            </span>
          )
        },
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <Button
            variant='outline'
            size='sm'
            disabled={!row.original.queryable || row.original.query?.fetching}
            onClick={() =>
              void client.refetchQueries({
                queryKey: ['upstream-balance', row.original.id],
              })
            }
            aria-label={t('Refresh balance for {{origin}} {{key}}', {
              origin: row.original.origin,
              key: row.original.key_label,
            })}
          >
            {t('Refresh')}
          </Button>
        ),
      },
    ],
    [t, client, reasons]
  )
  const table = useReactTable({
    data: props.rows,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getRowId: (row) => row.id,
    initialState: { pagination: { pageIndex: 0, pageSize: 20 } },
    enableRowSelection: false,
  })
  return (
    <DataTablePage
      table={table}
      columns={columns}
      toolbarProps={null}
      fixedHeight={false}
      paginationInFooter={false}
      showPagination={props.rows.length > 20}
    />
  )
}
