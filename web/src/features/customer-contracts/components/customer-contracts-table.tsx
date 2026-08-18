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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { UserContractDrawer } from '@/features/users/components/user-contract-drawer'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState, type NavigateFn } from '@/hooks/use-table-url-state'

import { getCustomerContracts } from '../api'
import {
  EMPTY_CUSTOMER_CONTRACT_SUMMARY,
  type CustomerContractAdminListItem,
  type CustomerContractsSearch,
} from '../types'
import { useCustomerContractColumns } from './customer-contract-columns'
import { CustomerContractSummary } from './customer-contract-summary'

interface CustomerContractsTableProps {
  search: CustomerContractsSearch
  onSearchChange: (
    patch: Partial<CustomerContractsSearch>,
    replace?: boolean
  ) => void
}

export function CustomerContractsTable(props: CustomerContractsTableProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const search = props.search
  const onSearchChange = props.onSearchChange
  const [selectedContract, setSelectedContract] =
    useState<CustomerContractAdminListItem | null>(null)

  const navigate = useCallback<NavigateFn>(
    (options) => {
      if (options.search === true) return
      const current = search as Record<string, unknown>
      const next =
        typeof options.search === 'function'
          ? options.search(current)
          : options.search
      onSearchChange(next as Partial<CustomerContractsSearch>, options.replace)
    },
    [onSearchChange, search]
  )

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: search as unknown as Record<string, unknown>,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'contract_status', searchKey: 'status', type: 'array' },
    ],
  })
  const statusFilter =
    (columnFilters.find((filter) => filter.id === 'contract_status')?.value as
      | CustomerContractsSearch['status']
      | undefined) ?? []

  const query = useQuery({
    queryKey: [
      'customer-contracts',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      statusFilter,
    ],
    queryFn: async () => {
      const response = await getCustomerContracts({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter,
        status: statusFilter[0] ?? '',
      })
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to load customer contracts')
        )
      }
      return response.data
    },
  })

  useEffect(() => {
    if (!query.error) return
    toast.error(query.error.message || t('Failed to load customer contracts'))
  }, [query.error, t])

  const handleManage = useCallback((item: CustomerContractAdminListItem) => {
    setSelectedContract(item)
  }, [])
  const columns = useCustomerContractColumns({ onManage: handleManage })
  const items = query.data?.items ?? []

  const { table } = useDataTable({
    data: items,
    columns,
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: query.data?.total ?? 0,
    ensurePageInRange,
  })

  const statusOptions = [
    { label: t('Active contracts'), value: 'active' },
    { label: t('No model access'), value: 'zero_access' },
    { label: t('Inactive contracts'), value: 'inactive' },
  ]

  const handleContractSaved = () => {
    void queryClient.invalidateQueries({ queryKey: ['customer-contracts'] })
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <CustomerContractSummary
        summary={query.data?.summary ?? EMPTY_CUSTOMER_CONTRACT_SUMMARY}
        isLoading={query.isLoading}
      />

      <div className='min-h-0 flex-1'>
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={query.isLoading}
          isFetching={query.isFetching}
          emptyTitle={t('No customer contracts')}
          emptyDescription={t(
            'Create a contract from the Users page or adjust your search and filters.'
          )}
          skeletonKeyPrefix='customer-contracts-skeleton'
          applyHeaderSize
          toolbarProps={{
            searchPlaceholder: t('Search customers or public models...'),
            searchDebounceMs: 500,
            filters: [
              {
                columnId: 'contract_status',
                title: t('Contract status'),
                options: statusOptions,
                singleSelect: true,
              },
            ],
          }}
        />
      </div>

      {selectedContract && (
        <UserContractDrawer
          open
          onOpenChange={(open) => !open && setSelectedContract(null)}
          user={{
            id: selectedContract.user_id,
            username: selectedContract.username,
          }}
          onSuccess={handleContractSaved}
        />
      )}
    </div>
  )
}
