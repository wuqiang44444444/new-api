/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { fireEvent, render, screen } from '@testing-library/react'
import { toast } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  ApiResponse,
  ContractEntityAdminView,
  ContractEntityUpdatePayload,
  ContractEntityWritePayload,
  CustomerContractAuditPage,
  CustomerContractChannelGroupOption,
  CustomerContractGroupOption,
  User,
  UserContractEntities,
} from '../../types'
import { UserContractDrawer } from '../user-contract-drawer'

const getUserContracts =
  vi.fn<() => Promise<ApiResponse<UserContractEntities>>>()
const getCustomerContractChannels =
  vi.fn<() => Promise<ApiResponse<CustomerContractChannelGroupOption[]>>>()
const getCustomerContractOptions =
  vi.fn<() => Promise<ApiResponse<CustomerContractGroupOption[]>>>()
const getContractEntityAudits =
  vi.fn<
    (
      contractId: number,
      page?: number
    ) => Promise<ApiResponse<CustomerContractAuditPage>>
  >()
const createUserContract =
  vi.fn<
    (
      userId: number,
      payload: ContractEntityWritePayload
    ) => Promise<ApiResponse<UserContractEntities>>
  >()
const updateContractEntity =
  vi.fn<
    (
      contractId: number,
      payload: ContractEntityUpdatePayload
    ) => Promise<ApiResponse<UserContractEntities>>
  >()

const translate = (key: string) => key

vi.mock('../../api', () => ({
  getUserContracts: () => getUserContracts(),
  getCustomerContractChannels: () => getCustomerContractChannels(),
  getCustomerContractOptions: () => getCustomerContractOptions(),
  getContractEntityAudits: (contractId: number, page: number) =>
    getContractEntityAudits(contractId, page),
  createUserContract: (userId: number, payload: ContractEntityWritePayload) =>
    createUserContract(userId, payload),
  updateContractEntity: (
    contractId: number,
    payload: ContractEntityUpdatePayload
  ) => updateContractEntity(contractId, payload),
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
  status: 1,
  role: 1,
} as User

const channels: CustomerContractChannelGroupOption[] = [
  {
    group: 'contract-route',
    native_group_ratio: '0.87',
    special_group_ratio: true,
    models: [
      { model: 'claude-sonnet-5', channels: [{ id: 11, name: 'primary' }] },
      {
        model: 'gemini-3-pro',
        channels: [
          { id: 11, name: 'primary' },
          { id: 12, name: 'backup' },
        ],
      },
    ],
  },
]

const options: CustomerContractGroupOption[] = [
  {
    group: 'contract-route',
    models: ['claude-sonnet-5', 'gemini-3-pro'],
    prices: {
      'claude-sonnet-5': {
        price_type: 'model_ratio',
        current_discounted_price: '0.87',
      },
    },
    native_group_ratio: '0.87',
    special_group_ratio: true,
  },
]

const contracts: ContractEntityAdminView[] = [
  {
    id: 5,
    name: 'Main contract',
    enabled: true,
    version: 3,
    rules: [
      {
        model: 'claude-sonnet-5',
        channel_id: 11,
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
  },
  {
    id: 6,
    name: 'Aux contract',
    enabled: false,
    version: 1,
    rules: [
      {
        model: 'gemini-3-pro',
        route_group: 'contract-route',
        discount: '0.9',
        available: true,
        native_group_ratio: '0.87',
        effective_multiplier: '0.783',
        special_group_ratio: false,
        price: { price_type: 'model_ratio', current_discounted_price: '0.783' },
      },
    ],
  },
]

function contractsData(list: ContractEntityAdminView[]): UserContractEntities {
  return { user_id: 7, username: 'customer-a', contracts: list }
}

const auditItem = {
  id: 90,
  contract_id: 5,
  user_id: 7,
  contract_version: 1,
  admin_user_id: 1,
  admin_username: 'root',
  operation: 'migrate' as const,
  reason: 'migrated from legacy contract',
  before_enabled: false,
  after_enabled: true,
  before_rule_count: 0,
  after_rule_count: 2,
  created_at: 1_786_982_400,
}

function renderDrawer(props?: { contractId?: number }) {
  return render(
    <UserContractDrawer
      open
      onOpenChange={vi.fn()}
      user={user}
      contractId={props?.contractId}
      onSuccess={vi.fn()}
    />
  )
}

describe('admin customer contract entity drawer', () => {
  beforeEach(() => {
    getUserContracts
      .mockReset()
      .mockResolvedValue({ success: true, data: contractsData(contracts) })
    getCustomerContractChannels
      .mockReset()
      .mockResolvedValue({ success: true, data: channels })
    getCustomerContractOptions
      .mockReset()
      .mockResolvedValue({ success: true, data: options })
    getContractEntityAudits.mockReset().mockResolvedValue({
      success: true,
      data: {
        items: [auditItem],
        total: 1,
        page: 1,
        page_size: 20,
      },
    })
    createUserContract.mockReset().mockResolvedValue({
      success: true,
      data: contractsData([contracts[0]]),
    })
    updateContractEntity.mockReset().mockResolvedValue({
      success: true,
      data: contractsData([contracts[0]]),
    })
  })

  it('lists every contract and preselects the requested contract', async () => {
    renderDrawer({ contractId: 6 })

    expect(await screen.findByText('Aux contract')).toBeTruthy()
    expect(screen.getByText('Main contract')).toBeTruthy()
    expect(screen.getByText('gemini-3-pro')).toBeTruthy()
    // The gemini rule has two channel candidates, so none is assumed.
    expect(screen.getAllByText('Select channel').length).toBeGreaterThanOrEqual(
      2
    )
  })

  it('keeps internal channel facts admin-only and recalculates draft pricing', async () => {
    renderDrawer()

    expect(await screen.findByText('claude-sonnet-5')).toBeTruthy()
    expect(screen.getAllByText('contract-route').length).toBeGreaterThan(0)
    expect(screen.getByText('primary')).toBeTruthy()
    expect(screen.getByText(/0\.87 × 0\.8 =/).textContent).toContain('0.696')

    const discount = screen.getByDisplayValue('0.8')
    fireEvent.change(discount, { target: { value: '50%' } })
    expect(screen.getByText(/0\.87 × 50% =/).textContent).toContain('0.435')
  })

  it('preserves the saved channel among multiple candidates when saving', async () => {
    const bound = {
      ...contracts[1],
      rules: contracts[1].rules.map((rule) => ({ ...rule, channel_id: 12 })),
    }
    getUserContracts.mockResolvedValue({
      success: true,
      data: contractsData([bound]),
    })
    renderDrawer({ contractId: 6 })
    fireEvent.change(await screen.findByLabelText('Change reason'), {
      target: { value: 'renew terms' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save contract' }))
    await vi.waitFor(() =>
      expect(updateContractEntity).toHaveBeenCalledWith(
        6,
        expect.objectContaining({
          rules: [expect.objectContaining({ channel_id: 12 })],
        })
      )
    )
  })

  it('submits one atomic replacement with the expected version, channel and reason', async () => {
    renderDrawer()

    const reason = await screen.findByLabelText('Change reason')
    fireEvent.change(reason, { target: { value: 'renewed annual contract' } })
    const saveButton = screen.getByRole('button', { name: 'Save contract' })
    await vi.waitFor(() =>
      expect(saveButton.hasAttribute('disabled')).toBe(false)
    )
    fireEvent.click(saveButton)

    await vi.waitFor(() =>
      expect(updateContractEntity).toHaveBeenCalledTimes(1)
    )
    expect(updateContractEntity).toHaveBeenCalledWith(5, {
      expected_version: 3,
      name: 'Main contract',
      enabled: true,
      reason: 'renewed annual contract',
      rules: [
        {
          model: 'claude-sonnet-5',
          channel_id: 11,
          route_group: 'contract-route',
          discount: '0.8',
        },
      ],
    })
  })

  it('blocks saving while an ambiguous rule has no explicit channel', async () => {
    const onOpenChange = vi.fn()
    render(
      <UserContractDrawer
        open
        onOpenChange={onOpenChange}
        user={user}
        contractId={6}
        onSuccess={vi.fn()}
      />
    )

    fireEvent.change(await screen.findByLabelText('Change reason'), {
      target: { value: 'ambiguous channel must be decided' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save contract' }))

    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        'Select a channel for every contract rule'
      )
    )
    expect(updateContractEntity).not.toHaveBeenCalled()
    expect(onOpenChange).not.toHaveBeenCalled()
  })

  it('creates a contract entity through the dedicated POST endpoint', async () => {
    renderDrawer()

    fireEvent.click(await screen.findByRole('button', { name: 'New contract' }))
    fireEvent.change(screen.getByLabelText('Contract name'), {
      target: { value: 'Expansion contract' },
    })
    fireEvent.change(screen.getByLabelText('Change reason'), {
      target: { value: 'second contract for batch jobs' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create contract' }))

    await vi.waitFor(() => expect(createUserContract).toHaveBeenCalledTimes(1))
    expect(createUserContract).toHaveBeenCalledWith(7, {
      name: 'Expansion contract',
      enabled: false,
      reason: 'second contract for batch jobs',
      rules: [],
    })
  })

  it('reloads the winning contracts after an optimistic-lock conflict', async () => {
    getUserContracts
      .mockResolvedValueOnce({ success: true, data: contractsData(contracts) })
      .mockResolvedValueOnce({
        success: true,
        data: contractsData([
          {
            id: 5,
            name: 'Main contract',
            enabled: false,
            version: 4,
            rules: [],
          },
          contracts[1],
        ]),
      })
    updateContractEntity.mockRejectedValueOnce({
      isAxiosError: true,
      response: { status: 409 },
    })
    renderDrawer()

    fireEvent.change(await screen.findByLabelText('Change reason'), {
      target: { value: 'stale edit' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save contract' }))

    await vi.waitFor(() =>
      expect(screen.getByText('v4 · 0 Rules')).toBeTruthy()
    )
    expect(getUserContracts).toHaveBeenCalledTimes(2)
  })

  it('keeps a successful save successful when audit refresh fails', async () => {
    const onSuccess = vi.fn()
    render(
      <UserContractDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        onSuccess={onSuccess}
      />
    )
    fireEvent.change(await screen.findByLabelText('Change reason'), {
      target: { value: 'renewal' },
    })
    await vi.waitFor(() => expect(getContractEntityAudits).toHaveBeenCalled())
    getContractEntityAudits.mockRejectedValue(
      new Error('audit read unavailable')
    )
    vi.mocked(toast.error).mockClear()
    vi.mocked(toast.success).mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Save contract' }))
    await vi.waitFor(() => expect(onSuccess).toHaveBeenCalledTimes(1))
    expect(toast.success).toHaveBeenCalledWith('Customer contract saved')
    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('Loading failed')
    )
    expect(toast.error).not.toHaveBeenCalledWith('audit read unavailable')
    expect(
      (screen.getByLabelText('Change reason') as HTMLInputElement).value
    ).toBe('')
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

  it('loads audit history for the selected contract only', async () => {
    renderDrawer()
    await screen.findByText('Main contract')

    fireEvent.click(screen.getByRole('tab', { name: 'Audit history' }))
    expect(await screen.findByText('Migrated')).toBeTruthy()
    expect(screen.getByText('migrated from legacy contract')).toBeTruthy()
    expect(getContractEntityAudits).toHaveBeenCalledWith(5, 1)
  })
})
