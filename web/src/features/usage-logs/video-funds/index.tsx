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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getVideoFund,
  getVideoFunds,
  refundVideoFunds,
  type VideoFundFilters,
  type VideoFundItem,
} from './api'

const EMPTY: VideoFundItem[] = []

export function VideoFundLogs() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const verification = useSecureVerification()
  const isRoot = useAuthStore((s) => s.auth.user?.role === ROLE.SUPER_ADMIN)
  const [filters, setFilters] = useState<VideoFundFilters>({
    p: 1,
    page_size: 20,
  })
  const [selected, setSelected] = useState<VideoFundItem | null>(null)
  const [note, setNote] = useState('')
  const query = useQuery({
    queryKey: ['video-funds', filters],
    queryFn: () => getVideoFunds(filters),
    retry: false,
    refetchInterval: 15000,
  })
  const detail = useQuery({
    queryKey: ['video-fund', selected?.kind, selected?.id],
    queryFn: () => (selected ? getVideoFund(selected) : null),
    enabled: !!selected,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.fund_state === 'pending' ? 15000 : false,
  })
  const refund = useMutation({
    mutationFn: async () => {
      if (!detail.data?.can_refund || !note.trim()) {
        throw new Error(t('Refresh the refund preview before continuing'))
      }
      const instruction = {
        kind: detail.data.kind,
        id: detail.data.id,
        version: detail.data.version,
        note: note.trim(),
      }
      const proof = await verification.requestVerification({
        scope: 'video.funds.refund',
        context: instruction,
        title: t('Confirm video refund'),
        description: t(
          'This returns collected funds and waives unpaid debt. Provider execution continues; the customer will not be charged again.'
        ),
      })
      if (!proof) return null
      return refundVideoFunds(instruction, proof.proof_token)
    },
    onSuccess: async (result) => {
      if (!result) return
      toast.success(
        t(
          'Refund instruction accepted. Check the funding status for completion.'
        )
      )
      setNote('')
      await Promise.all([
        client.invalidateQueries({ queryKey: ['video-funds'] }),
        client.invalidateQueries({ queryKey: ['video-fund'] }),
      ])
    },
    onError: (err) => {
      handleServerError(err)
      void detail.refetch()
    },
  })
  const date = (value: number) =>
    value > 0 ? new Date(value * 1000).toLocaleString() : '—'
  const state = (value: string) => {
    switch (value) {
      case 'abnormal':
        return t('Needs attention')
      case 'held':
        return t('Funds held')
      case 'closed':
        return t('No current charge')
      case 'charged':
        return t('Charged')
      case 'pending':
        return t('Refund pending')
      case 'refunded':
        return t('Refunded')
      case 'transferred':
        return t('Transferred to task')
      default:
        return value
    }
  }
  const columns: ColumnDef<VideoFundItem>[] = [
    {
      accessorKey: 'task_id',
      header: t('Task ID'),
      cell: ({ row }) => (
        <Button
          variant='link'
          className='h-auto max-w-56 truncate p-0'
          onClick={() => {
            setSelected(row.original)
            setNote('')
          }}
        >
          {row.original.task_id}
        </Button>
      ),
    },
    { accessorKey: 'user_id', header: t('User ID') },
    { accessorKey: 'model', header: t('Model') },
    { accessorKey: 'business_status', header: t('Task status') },
    {
      accessorKey: 'fund_state',
      header: t('Funding status'),
      cell: ({ row }) => state(row.original.fund_state),
    },
    { accessorKey: 'quota', header: t('Current amount (quota)') },
    {
      accessorKey: 'refunded_quota',
      header: t('Returned amount (quota)'),
      cell: ({ row }) =>
        row.original.refund_amount_known
          ? row.original.refunded_quota.toLocaleString()
          : t('Unknown'),
    },
    {
      accessorKey: 'created_at',
      header: t('Created at'),
      cell: ({ row }) => date(row.original.created_at),
    },
    {
      accessorKey: 'deadline_at',
      header: t('Automatic refund deadline'),
      cell: ({ row }) => date(row.original.deadline_at),
    },
  ]
  const { table } = useDataTable({
    columns,
    data: query.isError ? EMPTY : (query.data?.items ?? EMPTY),
    totalCount: query.data?.total ?? 0,
    getRowId: (row) => `${row.kind}-${row.id}`,
    pagination: { pageIndex: filters.p - 1, pageSize: filters.page_size },
    onPaginationChange: (updater) =>
      setFilters((prev) => {
        const current = { pageIndex: prev.p - 1, pageSize: prev.page_size }
        const next = typeof updater === 'function' ? updater(current) : updater
        return {
          ...prev,
          p: next.pageSize === prev.page_size ? next.pageIndex + 1 : 1,
          page_size: next.pageSize,
        }
      }),
    manualPagination: true,
    manualFiltering: true,
    enableSorting: false,
    enableRowSelection: false,
  })
  const update = (patch: Partial<VideoFundFilters>) =>
    setFilters((prev) => ({ ...prev, ...patch, p: 1 }))
  const item = detail.data
  const deliveryLabel = (value: string) => {
    if (value === 'write_failed') return t('Task response delivery failed')
    if (value === 'result_unavailable') return t('Video result is unavailable')
    return t('Customer receipt has not been verified')
  }
  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={query.isPending}
        isFetching={query.isFetching}
        paginationInFooter
        emptyTitle={
          query.isError
            ? t('Failed to load video fund records')
            : t('No records')
        }
        className='min-h-0 flex-1'
        toolbar={
          <div className='space-y-3'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Video requests only. Creation holds are released after 24 hours. Task results and customer refunds are tracked separately.'
              )}
            </p>
            {query.data?.summary && !query.isError && (
              <dl className='grid grid-cols-2 gap-3 text-sm lg:grid-cols-5'>
                <div>
                  <dt>{t('Funds held')}</dt>
                  <dd>
                    {query.data.summary.held_count} /{' '}
                    {query.data.summary.held_quota.toLocaleString()} quota
                  </dd>
                </div>
                <div>
                  <dt>{t('Needs attention')}</dt>
                  <dd>
                    {query.data.summary.abnormal_count} /{' '}
                    {query.data.summary.abnormal_quota.toLocaleString()} quota
                  </dd>
                </div>
                <div>
                  <dt>{t('Overdue creation holds')}</dt>
                  <dd>
                    {query.data.summary.overdue_count} /{' '}
                    {query.data.summary.overdue_quota.toLocaleString()} quota
                  </dd>
                </div>
                <div>
                  <dt>{t('Refund failures')}</dt>
                  <dd>
                    {query.data.summary.failed_count} /{' '}
                    {query.data.summary.failed_quota.toLocaleString()} quota
                  </dd>
                </div>
                <div>
                  <dt>{t('Returned amount (quota)')}</dt>
                  <dd>{query.data.summary.returned_quota.toLocaleString()}</dd>
                </div>
                <div className='col-span-2'>
                  <dt>{t('Oldest hold created at')}</dt>
                  <dd>{date(query.data.summary.oldest_held_at)}</dd>
                </div>
              </dl>
            )}
            <div className='flex flex-wrap gap-2'>
              <Input
                className='w-52'
                aria-label={t('Task ID')}
                placeholder={t('Task ID')}
                value={filters.task_id ?? ''}
                onChange={(e) => update({ task_id: e.target.value })}
              />
              <Input
                className='w-28'
                aria-label={t('User ID')}
                placeholder={t('User ID')}
                inputMode='numeric'
                value={filters.user_id ?? ''}
                onChange={(e) => update({ user_id: e.target.value })}
              />
              <Input
                className='w-28'
                aria-label={t('Application ID')}
                placeholder={t('Application ID')}
                inputMode='numeric'
                value={filters.app_id ?? ''}
                onChange={(e) => update({ app_id: e.target.value })}
              />
              <Input
                className='w-28'
                aria-label={t('Channel ID')}
                placeholder={t('Channel ID')}
                inputMode='numeric'
                value={filters.channel_id ?? ''}
                onChange={(e) => update({ channel_id: e.target.value })}
              />
              {(
                [
                  '',
                  'abnormal',
                  'held',
                  'pending',
                  'charged',
                  'refunded',
                ] as const
              ).map((value) => (
                <Button
                  key={value}
                  variant={
                    filters.state === value || (!filters.state && !value)
                      ? 'default'
                      : 'outline'
                  }
                  onClick={() => update({ state: value })}
                >
                  {value ? state(value) : t('All')}
                </Button>
              ))}
              <Button variant='outline' onClick={() => void query.refetch()}>
                {t('Refresh')}
              </Button>
            </div>
            <div className='flex flex-wrap gap-2'>
              <Label>
                {t('Refunds from')}
                <Input
                  type='datetime-local'
                  onChange={(e) =>
                    update({
                      refund_from: e.target.value
                        ? Math.floor(new Date(e.target.value).getTime() / 1000)
                        : undefined,
                    })
                  }
                />
              </Label>
              <Label>
                {t('Refunds until')}
                <Input
                  type='datetime-local'
                  onChange={(e) =>
                    update({
                      refund_to: e.target.value
                        ? Math.floor(new Date(e.target.value).getTime() / 1000)
                        : undefined,
                    })
                  }
                />
              </Label>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Refund dates affect returned totals; current holds include all creation dates.'
                )}
              </p>
            </div>
            {query.isError && (
              <p role='alert' className='text-destructive'>
                {t('Failed to load video fund records')}
              </p>
            )}
          </div>
        }
      />
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && !refund.isPending) setSelected(null)
        }}
        title={t('Video funding details')}
        description={t(
          'Amounts use quota units. Token quota is not an additional customer charge.'
        )}
        footer={
          isRoot && item?.can_refund ? (
            <Button
              disabled={
                !note.trim() ||
                refund.isPending ||
                detail.isFetching ||
                detail.isError
              }
              onClick={() => refund.mutate()}
            >
              {refund.isPending
                ? t('Processing...')
                : t('Confirm video refund')}
            </Button>
          ) : undefined
        }
      >
        {detail.isPending && <p role='status'>{t('Loading...')}</p>}
        {detail.isError && (
          <Button onClick={() => void detail.refetch()}>{t('Retry')}</Button>
        )}
        {item && !detail.isError && (
          <div className='space-y-4'>
            <dl className='grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-2 text-sm break-words'>
              <dt>{t('Task ID')}</dt>
              <dd>{item.task_id}</dd>
              <dt>{t('User ID')}</dt>
              <dd>{item.user_id}</dd>
              <dt>{t('Application ID')}</dt>
              <dd>{item.app_id || '—'}</dd>
              <dt>{t('Channel ID')}</dt>
              <dd>{item.channel_id || '—'}</dd>
              <dt>{t('Request ID')}</dt>
              <dd>{item.request_id || '—'}</dd>
              <dt>{t('Funding status')}</dt>
              <dd>{state(item.fund_state)}</dd>
              <dt>{t('Funding source')}</dt>
              <dd>
                {item.source === 'wallet' && t('Wallet')}
                {item.source === 'subscription' && t('Subscription')}
                {!['wallet', 'subscription'].includes(item.source) &&
                  t('Unknown')}
              </dd>
              <dt>{t('Current amount (quota)')}</dt>
              <dd>{item.quota.toLocaleString()}</dd>
              <dt>{t('Returned amount (quota)')}</dt>
              <dd>
                {item.refund_amount_known
                  ? item.refunded_quota.toLocaleString()
                  : t('Unknown')}
              </dd>
              <dt>{t('Waived debt (quota)')}</dt>
              <dd>{item.waived_quota.toLocaleString()}</dd>
              <dt>{t('Automatic refund deadline')}</dt>
              <dd>{date(item.deadline_at)}</dd>
              <dt>{t('Refund completed at')}</dt>
              <dd>{date(item.refunded_at)}</dd>
              <dt>{t('Next refund retry')}</dt>
              <dd>{date(item.retry_at)}</dd>
              <dt>{t('Delivery status')}</dt>
              <dd>{deliveryLabel(item.delivery)}</dd>
              <dt>{t('Operator ID')}</dt>
              <dd>{item.operator_id || '—'}</dd>
              <dt>{t('Audit note')}</dt>
              <dd>{item.note || '—'}</dd>
            </dl>
            {item.timeline.length > 0 && (
              <section aria-label={t('Funding history')}>
                <h3 className='font-medium'>{t('Funding history')}</h3>
                <ul className='space-y-1 text-sm'>
                  {item.timeline.map((event) => (
                    <li key={`${event.event}-${event.at}`}>
                      {date(event.at)} ·{' '}
                      {t(`videoFunds.event.${event.event}`, {
                        defaultValue: event.event,
                      })}{' '}
                      · {event.before_quota.toLocaleString()} →{' '}
                      {event.after_quota.toLocaleString()} quota
                    </li>
                  ))}
                </ul>
              </section>
            )}
            {item.failure && (
              <p role='status' className='text-destructive'>
                {t(
                  'Refund requires attention. Verify the funding evidence or wait for the scheduled retry.'
                )}{' '}
                <code>{item.failure}</code>
              </p>
            )}
            {isRoot && item.can_refund && (
              <div className='space-y-2'>
                <p className='text-sm'>
                  {t(
                    'This returns collected funds and waives unpaid debt. Provider execution continues; the customer will not be charged again.'
                  )}
                </p>
                <Label htmlFor='video-refund-note'>
                  {t('Refund reason (required)')}
                </Label>
                <Textarea
                  id='video-refund-note'
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  maxLength={1000}
                  disabled={refund.isPending}
                />
              </div>
            )}
          </div>
        )}
      </Dialog>
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}
