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
import { BadgePercent } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { formatTimestampToDate } from '@/lib/format'

import type {
  CustomerContractAdminListItem,
  CustomerContractAdminStatus,
} from '../types'

interface CustomerContractColumnsOptions {
  onManage: (item: CustomerContractAdminListItem) => void
}

function contractStatusPresentation(status: CustomerContractAdminStatus, t: (key: string) => string) {
  if (status === 'active') {
    return { label: t('Active contracts'), variant: 'warning' as const }
  }
  if (status === 'zero_access') {
    return { label: t('No model access'), variant: 'danger' as const }
  }
  return { label: t('Inactive contracts'), variant: 'neutral' as const }
}

export function useCustomerContractColumns(
  options: CustomerContractColumnsOptions
): ColumnDef<CustomerContractAdminListItem>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'contract_name',
      header: t('Contract name'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <div className='min-w-[160px]'>
          <div className='truncate font-medium'>{row.original.contract_name}</div>
          <div className='text-muted-foreground truncate text-xs'>
            v{row.original.contract_version}
          </div>
        </div>
      ),
      size: 220,
    },
    {
      accessorKey: 'username',
      header: t('Customer'),
      cell: ({ row }) => (
        <div className='min-w-[160px]'>
          <div className='truncate font-medium'>{row.original.username}</div>
          <div className='text-muted-foreground truncate text-xs'>
            {row.original.display_name ? `${row.original.display_name} · ` : ''}
            {t('User ID')} {row.original.user_id}
          </div>
        </div>
      ),
      size: 240,
    },
    {
      accessorKey: 'contract_status',
      header: t('Contract status'),
      meta: { mobileBadge: true },
      cell: ({ row }) => {
        const status = contractStatusPresentation(
          row.original.contract_status,
          t
        )
        return (
          <StatusBadge
            label={status.label}
            variant={status.variant}
            copyable={false}
          />
        )
      },
      size: 190,
    },
    {
      accessorKey: 'rule_count',
      header: t('Rules'),
      meta: { mobileOrder: 10 },
      cell: ({ row }) => (
        <div className='flex flex-wrap items-center gap-2'>
          <span className='font-medium tabular-nums'>
            {t('{{count}} rules', { count: row.original.rule_count })}
          </span>
          {row.original.unavailable_rule_count > 0 && (
            <StatusBadge
              label={t('Unavailable: {{count}}', {
                count: row.original.unavailable_rule_count,
              })}
              variant='danger'
              copyable={false}
            />
          )}
        </div>
      ),
      size: 190,
    },
    {
      accessorKey: 'bound_token_count',
      header: t('Bound keys'),
      meta: { mobileOrder: 20 },
      cell: ({ row }) => (
        <span className='font-medium tabular-nums'>
          {row.original.bound_token_count}
        </span>
      ),
      size: 130,
    },
    {
      accessorKey: 'updated_at',
      header: t('Last modified'),
      meta: { mobileOrder: 30 },
      cell: ({ row }) => {
        if (!row.original.updated_at) {
          return (
            <span className='text-muted-foreground'>
              {t('No audit record')}
            </span>
          )
        }
        return (
          <div className='min-w-[170px]'>
            <div>{formatTimestampToDate(row.original.updated_at)}</div>
            {row.original.admin_username && (
              <div className='text-muted-foreground text-xs'>
                {t('Modified by {{username}}', {
                  username: row.original.admin_username,
                })}
              </div>
            )}
          </div>
        )
      },
      size: 210,
    },
    {
      id: 'actions',
      header: t('Actions'),
      enableHiding: false,
      cell: ({ row }) => (
        <Button
          variant='outline'
          size='sm'
          onClick={() => options.onManage(row.original)}
        >
          <BadgePercent aria-hidden='true' data-icon='inline-start' />
          {t('Manage model contract')}
        </Button>
      ),
      size: 190,
    },
  ]
}
