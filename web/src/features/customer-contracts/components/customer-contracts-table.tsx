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
import { Link } from '@tanstack/react-router'
import { BadgePercent, FileStack, History, RefreshCw, Users } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
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
import { CustomerContractMigrationDialog } from './customer-contract-migration-dialog'
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
  const [migrationOpen, setMigrationOpen] = useState(false)
  const [filterResetKey, setFilterResetKey] = useState(0)

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
    { label: t('Inactive contracts'), value: 'inactive' },
  ]

  const handleContractSaved = () => {
    void queryClient.invalidateQueries({ queryKey: ['customer-contracts'] })
  }

  const hasFilters = Boolean(globalFilter || statusFilter.length)
  const emptyTitle = hasFilters
    ? t('No matching contracts')
    : t('No customer contracts')
  const emptyDescription = hasFilters
    ? t(
        'Try a different customer, contract name or model, or clear the filters.'
      )
    : t(
        'Create a contract for a customer from the Users page. Legacy user-level contracts appear here only after migration.'
      )
  const emptyAction = hasFilters ? (
    <Button
      variant='outline'
      onClick={() => {
        setFilterResetKey((key) => key + 1)
        table.setGlobalFilter('')
        onSearchChange({ filter: '', status: [], page: 1 })
      }}
    >
      {t('Clear filters')}
    </Button>
  ) : (
    <Button variant='outline' onClick={() => setMigrationOpen(true)}>
      <History aria-hidden='true' data-icon='inline-start' />
      {t('Review legacy contracts')}
    </Button>
  )

  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col gap-4 overflow-y-auto'>
      <div className='flex shrink-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Manage customer model access, contract status and API key bindings.'
          )}
        </p>
        <div className='flex shrink-0 flex-wrap items-center gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            render={<Link to='/admin/contract-templates' />}
          >
            <FileStack aria-hidden='true' data-icon='inline-start' />
            {t('Contract templates')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => setMigrationOpen(true)}
          >
            <History aria-hidden='true' data-icon='inline-start' />
            {t('Legacy contract migrations')}
          </Button>
          <Button
            size='sm'
            nativeButton={false}
            role='link'
            render={<Link to='/users' />}
          >
            <Users aria-hidden='true' data-icon='inline-start' />
            {t('Go to Users')}
          </Button>
        </div>
      </div>

      {query.isError ? (
        <div role='alert' className='min-h-0 flex-1 rounded-xl border'>
          <ErrorState
            title={t('Failed to load customer contracts')}
            description={t(
              'Contract data could not be loaded. Retry to check the current contracts.'
            )}
            onRetry={() => void query.refetch()}
          />
        </div>
      ) : (
        <>
          <CustomerContractSummary
            summary={query.data?.summary ?? EMPTY_CUSTOMER_CONTRACT_SUMMARY}
            isLoading={query.isPending}
          />
          <div className='min-h-80 flex-1'>
            <DataTablePage
              key={filterResetKey}
              table={table}
              columns={columns}
              isLoading={query.isPending}
              isFetching={query.isFetching}
              emptyTitle={emptyTitle}
              emptyDescription={emptyDescription}
              emptyIcon={<BadgePercent />}
              emptyAction={emptyAction}
              mobile={
                query.isSuccess && items.length === 0 ? (
                  <EmptyState
                    bordered
                    icon={BadgePercent}
                    title={emptyTitle}
                    description={emptyDescription}
                    action={emptyAction}
                  />
                ) : undefined
              }
              skeletonKeyPrefix='customer-contracts-skeleton'
              applyHeaderSize
              toolbarProps={{
                searchPlaceholder: t(
                  'Search customers, contracts or models...'
                ),
                searchDebounceMs: 500,
                preActions: (
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={query.isFetching}
                    onClick={() => void query.refetch()}
                  >
                    <RefreshCw aria-hidden='true' data-icon='inline-start' />
                    {t('Refresh')}
                  </Button>
                ),
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
        </>
      )}

      {selectedContract && (
        <UserContractDrawer
          open
          onOpenChange={(open) => !open && setSelectedContract(null)}
          user={{
            id: selectedContract.user_id,
            username: selectedContract.username,
          }}
          contractId={selectedContract.contract_id}
          onSuccess={handleContractSaved}
        />
      )}

      <CustomerContractMigrationDialog
        open={migrationOpen}
        onOpenChange={setMigrationOpen}
      />
    </div>
  )
}
