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
    onSuccess: () => void
    onOpenChange: (open: boolean) => void
  }) => (
    <div role='dialog' aria-label='Mock contract drawer'>
      <span>{props.user.username}</span>
      <button type='button' onClick={props.onSuccess}>
        Mock contract saved
      </button>
      <button type='button' onClick={() => props.onOpenChange(false)}>
        Mock close
      </button>
    </div>
  ),
}))

const activeResponse: CustomerContractAdminListResponse = {
  success: true,
  data: {
    page: 1,
    page_size: 20,
    total: 1,
    summary: { total: 3, active: 1, zero_access: 1, inactive: 1 },
    items: [
      {
        user_id: 7,
        username: 'customer-a',
        display_name: 'Customer A',
        contract_mode: true,
        contract_status: 'active',
        contract_version: 3,
        rule_count: 2,
        unavailable_rule_count: 1,
        updated_at: 1_786_982_400,
        admin_user_id: 1,
        admin_username: 'root',
      },
    ],
  },
}

function renderTable(children: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
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
    expect(screen.getByText('Active contracts')).toBeTruthy()
    expect(screen.getByText('No model access')).toBeTruthy()
    expect(screen.getByText('Inactive contracts')).toBeTruthy()
    expect(screen.getByText('Unavailable: 1')).toBeTruthy()
    expect(screen.getByText('Contract active · 2 rules')).toBeTruthy()

    fireEvent.click(
      screen.getByRole('button', { name: 'Manage model contract' })
    )
    expect(
      screen.getByRole('dialog', { name: 'Mock contract drawer' })
    ).toBeTruthy()
    expect(screen.getAllByText('customer-a').length).toBeGreaterThan(1)
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
        summary: { total: 0, active: 0, zero_access: 0, inactive: 0 },
        items: [],
      },
    })
    renderTable(<CustomerContractsTable search={{}} onSearchChange={vi.fn()} />)

    expect(await screen.findByText('No customer contracts')).toBeTruthy()
    expect(
      screen.getByText(
        'Create a contract from the Users page or adjust your search and filters.'
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
  })
})
