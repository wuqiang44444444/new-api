/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  ApiResponse,
  CustomerContract,
  CustomerContractAuditPage,
  CustomerContractGroupOption,
  CustomerContractWritePayload,
  User,
} from '../../types'
import { UserContractDrawer } from '../user-contract-drawer'

const getCustomerContract =
  vi.fn<() => Promise<ApiResponse<CustomerContract>>>()
const getCustomerContractOptions =
  vi.fn<() => Promise<ApiResponse<CustomerContractGroupOption[]>>>()
const getCustomerContractAudits =
  vi.fn<() => Promise<ApiResponse<CustomerContractAuditPage>>>()
const updateCustomerContract =
  vi.fn<
    (
      userId: number,
      payload: CustomerContractWritePayload
    ) => Promise<ApiResponse<CustomerContract>>
  >()

const translate = (key: string) => key

vi.mock('../../api', () => ({
  getCustomerContract: () => getCustomerContract(),
  getCustomerContractOptions: () => getCustomerContractOptions(),
  getCustomerContractAudits: () => getCustomerContractAudits(),
  updateCustomerContract: (
    userId: number,
    payload: CustomerContractWritePayload
  ) => updateCustomerContract(userId, payload),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: translate,
  }),
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const user = {
  id: 7,
  username: 'customer-a',
  display_name: 'Customer A',
  quota: 1000,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  aff_code: '',
  status: 1,
  role: 1,
  contract_mode: true,
  contract_version: 3,
} as User

const contract: CustomerContract = {
  user_id: user.id,
  username: user.username,
  contract_mode: true,
  contract_version: 3,
  rules: [
    {
      model: 'claude-sonnet-5',
      route_group: 'contract-route',
      discount: '0.8',
      available: true,
      native_group_ratio: '0.87',
      effective_multiplier: '0.696',
      special_group_ratio: true,
      price: {
        price_type: 'model_ratio',
        current_discounted_price: '0.696',
      },
    },
  ],
  disable_warning: 'All keys return to native access.',
}

describe('admin customer contract drawer', () => {
  beforeEach(() => {
    getCustomerContract
      .mockReset()
      .mockResolvedValue({ success: true, data: contract })
    getCustomerContractOptions.mockReset().mockResolvedValue({
      success: true,
      data: [
        {
          group: 'contract-route',
          models: ['claude-sonnet-5'],
          prices: {
            'claude-sonnet-5': {
              price_type: 'model_ratio',
              current_discounted_price: '0.87',
            },
          },
          native_group_ratio: '0.87',
          special_group_ratio: true,
        },
      ],
    })
    getCustomerContractAudits.mockReset().mockResolvedValue({
      success: true,
      data: { items: [], total: 0, page: 1, page_size: 20 },
    })
    updateCustomerContract.mockReset().mockResolvedValue({
      success: true,
      data: { ...contract, contract_version: 4 },
    })
  })

  it('shows internal route facts only to the administrator and recalculates draft pricing', async () => {
    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={vi.fn()}
      />
    )

    expect(await screen.findByText('claude-sonnet-5')).toBeTruthy()
    expect(
      (await screen.findAllByText('contract-route')).length
    ).toBeGreaterThan(0)
    expect(screen.getByText(/0\.87 × 0\.8 =/).textContent).toContain('0.696')
    expect(
      screen.getByText('A special native group ratio also applies')
    ).toBeTruthy()

    const discount = screen.getByDisplayValue('0.8')
    fireEvent.change(discount, { target: { value: '50%' } })
    expect(screen.getByText(/0\.87 × 50% =/).textContent).toContain('0.435')
  })

  it('submits one atomic full replacement with the expected version and audit reason', async () => {
    const onSuccess = vi.fn()
    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={onSuccess}
      />
    )

    const reason = await screen.findByLabelText('Change reason')
    fireEvent.change(reason, { target: { value: 'renewed annual contract' } })
    const saveButton = screen.getByRole('button', { name: 'Save contract' })
    await vi.waitFor(() =>
      expect(saveButton.hasAttribute('disabled')).toBe(false)
    )
    fireEvent.click(saveButton)

    await vi.waitFor(() =>
      expect(updateCustomerContract).toHaveBeenCalledTimes(1)
    )
    expect(updateCustomerContract).toHaveBeenCalledWith(7, {
      expected_version: 3,
      enabled: true,
      reason: 'renewed annual contract',
      rules: [
        {
          model: 'claude-sonnet-5',
          route_group: 'contract-route',
          discount: '0.8',
        },
      ],
    })
    await vi.waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
  })

  it('rejects an invalid or over-precise discount before the atomic write', async () => {
    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={vi.fn()}
      />
    )

    const discount = await screen.findByDisplayValue('0.8')
    fireEvent.change(discount, { target: { value: '0.123456789' } })
    fireEvent.change(screen.getByLabelText('Change reason'), {
      target: { value: 'invalid precision must not reach billing' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save contract' }))

    expect(screen.getByText('Invalid contract discount')).toBeTruthy()
    expect(updateCustomerContract).not.toHaveBeenCalled()
  })

  it('keeps configuration usable when audit history fails to load', async () => {
    getCustomerContractAudits.mockRejectedValueOnce(
      new Error('audit temporarily unavailable')
    )

    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={vi.fn()}
      />
    )

    expect(await screen.findByText('claude-sonnet-5')).toBeTruthy()
    expect(screen.getByDisplayValue('0.8')).toBeTruthy()
  })

  it('reloads the winning contract after an optimistic-lock conflict', async () => {
    const latest = {
      ...contract,
      contract_version: 4,
      rules: [{ ...contract.rules[0], discount: '0.6' }],
    }
    getCustomerContract
      .mockResolvedValueOnce({ success: true, data: contract })
      .mockResolvedValueOnce({ success: true, data: latest })
    updateCustomerContract.mockRejectedValueOnce({
      isAxiosError: true,
      response: { status: 409 },
    })
    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={vi.fn()}
      />
    )

    fireEvent.change(await screen.findByLabelText('Change reason'), {
      target: { value: 'stale edit' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save contract' }))

    expect(await screen.findByDisplayValue('0.6')).toBeTruthy()
    expect(getCustomerContract).toHaveBeenCalledTimes(2)
  })

  it('warns before discarding unsaved changes', async () => {
    const onOpenChange = vi.fn()
    render(
      <UserContractDrawer
        open
        onOpenChange={onOpenChange}
        user={user}
        onSuccess={vi.fn()}
      />
    )
    fireEvent.change(await screen.findByLabelText('Change reason'), {
      target: { value: 'unsaved' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(screen.getByText('Discard unsaved contract changes?')).toBeTruthy()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })

  it('requires confirmation before disabling contract mode', async () => {
    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={vi.fn()}
      />
    )

    fireEvent.click(
      await screen.findByRole('button', { name: 'Disable contract mode' })
    )
    expect(screen.getByText('Disable contract mode?')).toBeTruthy()
    expect(screen.getByText('All keys return to native access.')).toBeTruthy()
  })

  it('shows the fail-closed zero-permission state and supports re-enabling', async () => {
    getCustomerContract.mockResolvedValueOnce({
      success: true,
      data: {
        ...contract,
        contract_mode: false,
        contract_version: 4,
        rules: [],
      },
    })
    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={vi.fn()}
      />
    )

    expect(await screen.findByText('No contract models')).toBeTruthy()
    fireEvent.click(
      screen.getByRole('button', { name: 'Enable contract mode' })
    )
    expect(
      screen.getByText(
        'The contract is active, so all model calls are currently denied.'
      )
    ).toBeTruthy()
  })
})
