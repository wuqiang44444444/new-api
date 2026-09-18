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
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { TruncatedCell } from '@/components/data-table'
import dayjs from '@/lib/dayjs'

import type { ErrorLogItem } from '../api'
import { errorEventTypeLabel } from './error-event-type'
import { ErrorLogDetailsDialog } from './error-log-details-dialog'
import { ErrorLogStatus } from './error-log-status'

export function useErrorLogColumns(): ColumnDef<ErrorLogItem>[] {
  const { t } = useTranslation()
  return useMemo(() => {
    const columns: ColumnDef<ErrorLogItem>[] = [
      {
        accessorKey: 'created_at',
        header: t('Time'),
        size: 170,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {dayjs.unix(row.original.created_at).format('YYYY-MM-DD HH:mm:ss')}
          </span>
        ),
        meta: { label: t('Time'), mobileTitle: true },
      },
      {
        accessorKey: 'event_type',
        header: t('Event Type'),
        size: 100,
        cell: ({ row }) => (
          <span className='text-muted-foreground'>
            {errorEventTypeLabel(row.original.event_type, t)}
          </span>
        ),
        meta: { label: t('Event Type') },
      },
      {
        accessorKey: 'status',
        header: 'HTTP',
        size: 180,
        cell: ({ row }) => <ErrorLogStatus entry={row.original} compact />,
        meta: { label: 'HTTP', mobileBadge: true },
      },
      {
        accessorKey: 'module',
        header: t('Module'),
        size: 90,
        cell: ({ row }) => (
          <span className='text-muted-foreground font-mono'>
            {row.original.module}
          </span>
        ),
        meta: { label: t('Module'), mobileHidden: true },
      },
      {
        accessorKey: 'model_name',
        header: t('Model'),
        size: 160,
        cell: ({ row }) => (
          <TruncatedCell className='max-w-40'>
            {row.original.model_name || '—'}
          </TruncatedCell>
        ),
        meta: { label: t('Model') },
      },
      {
        id: 'channel',
        header: t('Channel'),
        size: 120,
        accessorFn: (entry) =>
          entry.channel_name ||
          (entry.channel_id ? `#${entry.channel_id}` : ''),
        cell: ({ row }) => (
          <TruncatedCell className='max-w-28'>
            {row.original.channel_name ||
              (row.original.channel_id ? `#${row.original.channel_id}` : '—')}
          </TruncatedCell>
        ),
        meta: { label: t('Channel') },
      },
      {
        accessorKey: 'username',
        header: t('Username'),
        size: 110,
        cell: ({ row }) => (
          <TruncatedCell className='max-w-24'>
            {row.original.username || '—'}
          </TruncatedCell>
        ),
        meta: { label: t('Username') },
      },
      {
        accessorKey: 'token_name',
        header: t('Token Name'),
        size: 120,
        cell: ({ row }) => (
          <TruncatedCell className='max-w-28'>
            {row.original.token_name || '—'}
          </TruncatedCell>
        ),
        meta: { label: t('Token Name'), mobileHidden: true },
      },
      {
        id: 'reason',
        header: t('Reason'),
        size: 200,
        accessorFn: (entry) =>
          [entry.reason, entry.public_code].filter(Boolean).join(' · '),
        cell: ({ row }) => (
          <TruncatedCell className='max-w-48 font-mono'>
            {[row.original.reason, row.original.public_code]
              .filter(Boolean)
              .join(' · ') || '—'}
          </TruncatedCell>
        ),
        meta: { label: t('Reason') },
      },
      {
        accessorKey: 'request_id',
        header: t('Request ID'),
        size: 200,
        cell: ({ row }) => (
          <TruncatedCell className='max-w-44 font-mono'>
            {row.original.request_id || '—'}
          </TruncatedCell>
        ),
        meta: { label: t('Request ID'), mobileHidden: true },
      },
      {
        id: 'elapsed',
        header: t('Elapsed'),
        size: 90,
        accessorFn: (entry) => entry.elapsed_ms,
        cell: ({ row }) => (
          <span className='font-mono tabular-nums'>
            {row.original.elapsed_ms
              ? `${(row.original.elapsed_ms / 1000).toFixed(2)}s`
              : '—'}
          </span>
        ),
        meta: { label: t('Elapsed'), mobileHidden: true },
      },
      {
        id: 'details',
        header: t('Details'),
        size: 70,
        enableHiding: false,
        cell: ({ row }) => <ErrorLogDetailsDialog entry={row.original} />,
        meta: { label: t('Details') },
      },
    ]
    return columns
  }, [t])
}
