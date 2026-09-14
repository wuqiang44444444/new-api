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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { CustomerContractAdminListResponse } from '../../types'
import { CustomerContractsTable } from '../customer-contracts-table'

const { getCustomerContracts, toastError } = vi.hoisted(() => ({
  getCustomerContracts:
    vi.fn<() => Promise<CustomerContractAdminListResponse>>(),
  toastError: vi.fn(),
}))

vi.mock('../../api', () => ({
  getCustomerContracts: () => getCustomerContracts(),
  getCustomerContractMigrationPreview: vi.fn(),
  migrateCustomerContract: vi.fn(),
}))

vi.mock('@/hooks', () => ({
  useMediaQuery: () => false,
  useDebounce: (value: unknown) => value,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) =>
      key
        .replace('{{count}}', String(options?.count ?? ''))
        .replace('{{username}}', String(options?.username ?? '')),
  }),
}))

vi.mock('sonner', () => ({
  toast: { error: toastError },
}))

vi.mock('@/features/users/components/user-contract-drawer', () => ({
  UserContractDrawer: (props: {
    user: { id: number; username: string }
    contractId?: number
    onSuccess: () => void
    onOpenChange: (open: boolean) => void
  }) => (
    <div role='dialog' aria-label='Mock contract drawer'>
      <span>{props.user.username}</span>
      <span>contract:{props.contractId}</span>
      <button type='button' onClick={props.onSuccess}>
        Mock contract saved
      </button>
      <button type='button' onClick={() => props.onOpenChange(false)}>
        Mock close
      </button>
    </div>
  ),
}))

const activeResponse = {
  success: true,
  data: {
    page: 1,
    page_size: 20,
    total: 1,
    summary: { total: 3, active: 2, inactive: 1 },
    items: [
      {
        contract_id: 31,
        contract_name: 'Team contract',
        user_id: 7,
        username: 'customer-a',
        display_name: 'Customer A',
        contract_enabled: true,
        contract_status: 'active',
        contract_version: 3,
        rule_count: 2,
        unavailable_rule_count: 1,
        bound_token_count: 5,
        updated_at: 1_786_982_400,
        admin_user_id: 1,
        admin_username: 'root',
      },
    ],
  },
} satisfies CustomerContractAdminListResponse

function renderTable(children: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const routeTree = createRootRoute({ component: () => children })
  const router = createRouter({
    routeTree,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('customer contracts admin table', () => {
  beforeEach(() => {
    getCustomerContracts.mockReset().mockResolvedValue(activeResponse)
    toastError.mockReset()
  })

  it('shows the global summary, contract risk and existing editor entry', async () => {
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)

    expect(await screen.findByText('customer-a')).toBeTruthy()
    expect(screen.getByText('All contracts')).toBeTruthy()
    // The status filter and the summary cards repeat these labels.
    expect(screen.getAllByText('Active contracts').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Inactive contracts').length).toBeGreaterThan(0)
    expect(screen.getByText('Team contract')).toBeTruthy()
    expect(screen.getByText('Unavailable: 1')).toBeTruthy()
    expect(screen.getByText('5')).toBeTruthy()

    fireEvent.click(
      screen.getByRole('button', { name: 'Manage model contract' })
    )
    expect(
      screen.getByRole('dialog', { name: 'Mock contract drawer' })
    ).toBeTruthy()
    expect(screen.getAllByText('customer-a').length).toBeGreaterThan(1)
    expect(screen.getByText('contract:31')).toBeTruthy()
  })

  it('refreshes the aggregate list after the existing contract editor saves', async () => {
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)
    fireEvent.click(
      await screen.findByRole('button', { name: 'Manage model contract' })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Mock contract saved' }))

    await vi.waitFor(() =>
      expect(getCustomerContracts).toHaveBeenCalledTimes(2)
    )
  })

  it('shows the contract-specific empty state', async () => {
    getCustomerContracts.mockResolvedValueOnce({
      success: true,
      data: {
        page: 1,
        page_size: 20,
        total: 0,
        summary: { total: 0, active: 0, inactive: 0 },
        items: [],
      },
    })
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)

    expect(await screen.findByText('No customer contracts')).toBeTruthy()
    expect(
      screen.getByText(
        'Create a contract for a customer from the Users page. Legacy user-level contracts appear here only after migration.'
      )
    ).toBeTruthy()
  })

  it('reports a failed aggregate read without presenting contract rows', async () => {
    getCustomerContracts.mockRejectedValueOnce(new Error('list unavailable'))
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)

    await vi.waitFor(() =>
      expect(toastError).toHaveBeenCalledWith('list unavailable')
    )
    expect(screen.queryByText('customer-a')).toBeNull()
    expect(screen.queryByText('No customer contracts')).toBeNull()
    expect(screen.queryByText('All contracts')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Team contract')).toBeTruthy()
  })
  it('offers clearing filters when a search has no results while keeping the global totals', async () => {
    getCustomerContracts.mockResolvedValueOnce({
      success: true,
      data: { ...activeResponse.data, total: 0, items: [] },
    })
    const onSearchChange = vi.fn()
    renderTable(
      <CustomerContractsTable
        search={{ filter: 'missing', page: 2 }}
        onSearchChange={onSearchChange}
      />
    )
    expect(await screen.findByText('No matching contracts')).toBeTruthy()
    expect(screen.queryByText('No customer contracts')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(onSearchChange).toHaveBeenCalledWith({
      filter: '',
      status: [],
      page: 1,
    })
    expect(await screen.findByText('Team contract')).toBeTruthy()
    expect(
      screen.getByPlaceholderText('Search customers, contracts or models...')
    ).toHaveValue('')
  })

  it('provides a direct users entry for creating a customer contract', async () => {
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)
    expect(
      await screen.findByRole('link', { name: 'Go to Users' })
    ).toHaveAttribute('href', '/users')
  })
  it('does not report missing contracts while the first request is still pending', async () => {
    let resolveResponse!: (value: CustomerContractAdminListResponse) => void
    const response = new Promise<CustomerContractAdminListResponse>(
      (resolve) => {
        resolveResponse = resolve
      }
    )
    getCustomerContracts.mockReturnValueOnce(response)
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)
    await screen.findByText('All contracts')
    expect(screen.queryByText('No customer contracts')).toBeNull()
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeDisabled()
    resolveResponse(activeResponse)
    expect(await screen.findByText('Team contract')).toBeTruthy()
  })
  it('clears a typed search without restoring the previous search draft', async () => {
    getCustomerContracts
      .mockResolvedValueOnce(activeResponse)
      .mockResolvedValueOnce({
        success: true,
        data: { ...activeResponse.data, total: 0, items: [] },
      })
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)
    await screen.findByText('Team contract')
    const input = screen.getByPlaceholderText(
      'Search customers, contracts or models...'
    )
    fireEvent.change(input, { target: { value: 'missing' } })
    await screen.findByText('No matching contracts')
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(
      screen.getByPlaceholderText('Search customers, contracts or models...')
    ).toHaveValue('')
    expect(await screen.findByText('Team contract')).toBeTruthy()
  })
})
