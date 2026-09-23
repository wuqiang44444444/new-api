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
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  ContractTemplateListResponse,
  ContractTemplateSnapshotResponse,
} from '@/features/customer-contracts/template-types'

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
const getContractTemplates =
  vi.fn<() => Promise<ContractTemplateListResponse>>()
const getContractTemplate =
  vi.fn<(templateId: number) => Promise<ContractTemplateSnapshotResponse>>()

// Mirrors i18next's default interpolation so assertions can expect the
// user-visible message instead of the raw key.
const translate = (key: string, params?: Record<string, unknown>) =>
  params
    ? key.replaceAll(/\{\{\s*(\w+)\s*\}\}/g, (raw: string, name: string) =>
        name in params ? String(params[name]) : raw
      )
    : key

const pointerCaptureDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'setPointerCapture'
)

function stubPointerCapture() {
  Object.defineProperty(HTMLElement.prototype, 'setPointerCapture', {
    configurable: true,
    value: vi.fn(),
  })
}

function restorePointerCapture() {
  if (pointerCaptureDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'setPointerCapture',
      pointerCaptureDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'setPointerCapture')
  }
}

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

vi.mock('@/features/customer-contracts/template-api', () => ({
  getContractTemplates: () => getContractTemplates(),
  getContractTemplate: (templateId: number) => getContractTemplate(templateId),
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
    getContractTemplates.mockReset().mockResolvedValue({
      success: true,
      data: { items: [], total: 0, page: 1, page_size: 20 },
    })
    getContractTemplate.mockReset().mockResolvedValue({ success: false })
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
    // The gemini rule has two channel candidates, so none is assumed: the
    // per-rule decision stays a single select while the add row shows the
    // multi-select placeholder until channels are picked.
    expect(screen.getAllByText('Select channel').length).toBeGreaterThanOrEqual(
      1
    )
    expect(screen.getByPlaceholderText('Search and select models')).toBeTruthy()
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

  it('adds one same-discount rule per selected channel via select all', async () => {
    stubPointerCapture()
    try {
      renderDrawer()
      fireEvent.click(
        await screen.findByRole('button', { name: 'New contract' })
      )

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'gemini-3-pro' })
      )
      const channelInput = await screen.findByLabelText(
        'Channels for model gemini-3-pro'
      )
      await user.click(channelInput)
      await user.click(
        await screen.findByRole('button', { name: 'Select all' })
      )
      // Both candidate channels end up selected while the popup stays open
      // for further batch picking.
      expect(screen.getAllByText('primary').length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText('backup').length).toBeGreaterThanOrEqual(1)
      // The open popup keeps the drawer inert for role queries; text queries
      // still reach the add and save buttons.
      fireEvent.click(screen.getByText('Add'))

      fireEvent.change(screen.getByLabelText('Contract name'), {
        target: { value: 'Expansion contract' },
      })
      fireEvent.change(screen.getByLabelText('Change reason'), {
        target: { value: 'bind every gemini channel' },
      })
      fireEvent.click(screen.getByText('Create contract'))

      await vi.waitFor(() =>
        expect(createUserContract).toHaveBeenCalledTimes(1)
      )
      expect(createUserContract).toHaveBeenCalledWith(
        7,
        expect.objectContaining({
          rules: [
            expect.objectContaining({
              model: 'gemini-3-pro',
              channel_id: 11,
              route_group: 'contract-route',
              discount: '1',
            }),
            expect.objectContaining({
              model: 'gemini-3-pro',
              channel_id: 12,
              route_group: 'contract-route',
              discount: '1',
            }),
          ],
        })
      )
    } finally {
      restorePointerCapture()
    }
  })

  it('rejects adding a channel the model already binds instead of skipping it', async () => {
    stubPointerCapture()
    try {
      renderDrawer({ contractId: 5 })

      const user = userEvent.setup()
      // claude-sonnet-5 has exactly one candidate channel, so it is
      // preselected, and that channel is already bound by the saved rule.
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'claude-sonnet-5' })
      )
      // The open models popup keeps the drawer inert for role queries; text
      // queries still reach the add button.
      fireEvent.click(screen.getByText('Add'))

      await vi.waitFor(() =>
        // The test i18n mock returns keys verbatim, so the rejected channel
        // list surfaces inside the untranslated placeholder key.
        expect(toast.error).toHaveBeenCalledWith(
          'This model already binds channels: primary'
        )
      )
      expect(createUserContract).not.toHaveBeenCalled()
      expect(updateContractEntity).not.toHaveBeenCalled()
    } finally {
      restorePointerCapture()
    }
  })

  it('adds several models at once, each with its own channels', async () => {
    stubPointerCapture()
    try {
      renderDrawer()
      fireEvent.click(
        await screen.findByRole('button', { name: 'New contract' })
      )

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'claude-sonnet-5' })
      )
      await user.keyboard('{Escape}')
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'gemini-3-pro' })
      )
      await user.keyboard('{Escape}')
      // claude has a single candidate and is auto-picked; gemini needs an
      // explicit multi-pick even though claude already uses channel 11.
      const geminiChannels = await screen.findByLabelText(
        'Channels for model gemini-3-pro'
      )
      await user.click(geminiChannels)
      await user.click(
        await screen.findByRole('button', { name: 'Select all' })
      )
      // The i18n mock interpolates, so the counters are user-visible text.
      expect(screen.getByText('Will add 2 models and 3 rules')).toBeTruthy()
      fireEvent.click(screen.getByText('Add'))

      fireEvent.change(screen.getByLabelText('Contract name'), {
        target: { value: 'Expansion contract' },
      })
      fireEvent.change(screen.getByLabelText('Change reason'), {
        target: { value: 'batch models' },
      })
      fireEvent.click(screen.getByText('Create contract'))

      await vi.waitFor(() =>
        expect(createUserContract).toHaveBeenCalledTimes(1)
      )
      expect(createUserContract).toHaveBeenCalledWith(
        7,
        expect.objectContaining({
          rules: [
            expect.objectContaining({
              model: 'claude-sonnet-5',
              channel_id: 11,
              route_group: 'contract-route',
              discount: '1',
            }),
            expect.objectContaining({
              model: 'gemini-3-pro',
              channel_id: 11,
              route_group: 'contract-route',
              discount: '1',
            }),
            expect.objectContaining({
              model: 'gemini-3-pro',
              channel_id: 12,
              route_group: 'contract-route',
              discount: '1',
            }),
          ],
        })
      )
    } finally {
      restorePointerCapture()
    }
  })

  it('keeps the batch pending when a model has no channel, then lets it succeed', async () => {
    stubPointerCapture()
    try {
      renderDrawer()
      fireEvent.click(
        await screen.findByRole('button', { name: 'New contract' })
      )

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'gemini-3-pro' })
      )
      await user.keyboard('{Escape}')
      // gemini has two candidates, so none is assumed: adding without an
      // explicit pick names the model and rejects the whole batch.
      fireEvent.click(screen.getByText('Add'))
      await vi.waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(
          'Select a channel for model gemini-3-pro'
        )
      )
      expect(
        screen.getByLabelText('Channels for model gemini-3-pro')
      ).toBeTruthy()

      const channelInput = screen.getByLabelText(
        'Channels for model gemini-3-pro'
      )
      await user.click(channelInput)
      await user.click(
        await screen.findByRole('button', { name: 'Select all' })
      )
      fireEvent.click(screen.getByText('Add'))

      fireEvent.change(screen.getByLabelText('Contract name'), {
        target: { value: 'Expansion contract' },
      })
      fireEvent.change(screen.getByLabelText('Change reason'), {
        target: { value: 'retry batch' },
      })
      fireEvent.click(screen.getByText('Create contract'))

      await vi.waitFor(() =>
        expect(createUserContract).toHaveBeenCalledTimes(1)
      )
      expect(createUserContract).toHaveBeenCalledWith(
        7,
        expect.objectContaining({
          rules: [
            expect.objectContaining({ model: 'gemini-3-pro', channel_id: 11 }),
            expect.objectContaining({ model: 'gemini-3-pro', channel_id: 12 }),
          ],
        })
      )
    } finally {
      restorePointerCapture()
    }
  })

  it('rejects the whole batch when one model repeats a bound channel', async () => {
    stubPointerCapture()
    try {
      renderDrawer({ contractId: 5 })

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'claude-sonnet-5' })
      )
      await user.keyboard('{Escape}')
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'gemini-3-pro' })
      )
      await user.keyboard('{Escape}')
      const geminiChannels = await screen.findByLabelText(
        'Channels for model gemini-3-pro'
      )
      await user.click(geminiChannels)
      await user.click(
        await screen.findByRole('button', { name: 'Select all' })
      )
      fireEvent.click(screen.getByText('Add'))
      await vi.waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(
          'This model already binds channels: primary'
        )
      )

      // The batch is atomic: gemini's valid picks are not appended either.
      fireEvent.change(screen.getByLabelText('Contract name'), {
        target: { value: 'Main contract revised' },
      })
      fireEvent.change(screen.getByLabelText('Change reason'), {
        target: { value: 'check atomicity' },
      })
      fireEvent.click(screen.getByText('Save contract'))
      await vi.waitFor(() =>
        expect(updateContractEntity).toHaveBeenCalledTimes(1)
      )
      expect(updateContractEntity).toHaveBeenCalledWith(
        5,
        expect.objectContaining({
          rules: [
            expect.objectContaining({
              model: 'claude-sonnet-5',
              channel_id: 11,
            }),
          ],
        })
      )
    } finally {
      restorePointerCapture()
    }
  })

  it('rejects model names that differ only by letter case inside one batch', async () => {
    stubPointerCapture()
    try {
      getCustomerContractChannels.mockResolvedValue({
        success: true,
        data: [
          {
            group: 'contract-route',
            native_group_ratio: '1',
            special_group_ratio: false,
            models: [
              { model: 'GPT-5 Mini', channels: [{ id: 21, name: 'alpha' }] },
              { model: 'gpt-5 mini', channels: [{ id: 22, name: 'beta' }] },
            ],
          },
        ],
      })
      renderDrawer()

      const user = userEvent.setup()
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'GPT-5 Mini' })
      )
      await user.keyboard('{Escape}')
      await user.click(await screen.findByLabelText('Search and select models'))
      await user.click(
        await screen.findByRole('option', { name: 'gpt-5 mini' })
      )
      await user.keyboard('{Escape}')
      fireEvent.click(screen.getByText('Add'))

      await vi.waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(
          'Model names that differ only by letter case cannot coexist'
        )
      )
      expect(createUserContract).not.toHaveBeenCalled()
    } finally {
      restorePointerCapture()
    }
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

  it('ignores a template response after switching to an existing contract', async () => {
    stubPointerCapture()
    try {
      const source = {
        id: 3,
        name: 'Delayed template',
        enabled: true,
        version: 1,
        creator_id: 1,
        updater_id: 1,
        rules: [
          {
            public_model: 'claude-sonnet-5',
            channel_id: 11,
            route_group: 'contract-route',
            ratio_units: 50000000,
            available: true,
          },
        ],
      }
      getContractTemplates.mockResolvedValue({
        success: true,
        data: {
          items: [
            {
              ...source,
              model_count: 1,
              rule_count: 1,
              stale_rule_count: 0,
              updater_name: 'admin',
              created_at: 0,
              updated_at: 0,
            },
          ],
          total: 1,
          page: 1,
          page_size: 100,
        },
      })
      let resolve!: (response: ContractTemplateSnapshotResponse) => void
      getContractTemplate.mockReturnValue(
        new Promise((done) => {
          resolve = done
        })
      )
      renderDrawer()
      await screen.findByText('Main contract')
      fireEvent.click(screen.getByRole('button', { name: 'New contract' }))
      await userEvent.click(
        document.querySelector('#contract-template-source') as HTMLElement
      )
      await userEvent.click(
        await screen.findByRole('option', { name: 'Delayed template' })
      )
      fireEvent.click(screen.getByRole('button', { name: /Main contract/ }))
      await act(async () => {
        resolve({ success: true, data: source })
      })
      expect(screen.getByLabelText('Contract name')).toHaveValue(
        'Main contract'
      )
      expect(screen.getByDisplayValue('0.8')).toBeInTheDocument()
      expect(
        screen.queryByDisplayValue('Delayed template')
      ).not.toBeInTheDocument()
    } finally {
      restorePointerCapture()
    }
  })

  it('loads enabled templates beyond the first page', async () => {
    const template = {
      id: 1,
      name: 'Older template',
      enabled: true,
      version: 1,
      model_count: 1,
      rule_count: 1,
      stale_rule_count: 0,
      creator_id: 1,
      updater_id: 1,
      updater_name: 'admin',
      created_at: 0,
      updated_at: 0,
    }
    getContractTemplates
      .mockResolvedValueOnce({
        success: true,
        data: {
          items: Array.from({ length: 100 }, (_, i) => ({
            ...template,
            id: i + 2,
            name: `Template ${i + 2}`,
          })),
          total: 101,
          page: 1,
          page_size: 100,
        },
      })
      .mockResolvedValueOnce({
        success: true,
        data: { items: [template], total: 101, page: 2, page_size: 100 },
      })
    stubPointerCapture()
    try {
      renderDrawer()
      await screen.findByText('Main contract')
      fireEvent.click(screen.getByRole('button', { name: 'New contract' }))
      await userEvent.click(
        document.querySelector('#contract-template-source') as HTMLElement
      )
      expect(
        await screen.findByRole('option', { name: 'Older template' })
      ).toBeInTheDocument()
    } finally {
      restorePointerCapture()
    }
  })

  it('applies an enabled template on create and submits the confirmed source', async () => {
    stubPointerCapture()
    try {
      getContractTemplates.mockResolvedValue({
        success: true,
        data: {
          items: [
            {
              id: 3,
              name: 'Starter',
              enabled: true,
              version: 4,
              model_count: 1,
              rule_count: 1,
              stale_rule_count: 0,
              creator_id: 1,
              updater_id: 1,
              updater_name: 'admin',
              created_at: 0,
              updated_at: 0,
            },
          ],
          total: 1,
          page: 1,
          page_size: 20,
        },
      })
      getContractTemplate.mockResolvedValue({
        success: true,
        data: {
          id: 3,
          name: 'Starter',
          enabled: true,
          version: 4,
          creator_id: 1,
          updater_id: 1,
          rules: [
            {
              public_model: 'claude-sonnet-5',
              channel_id: 11,
              route_group: 'contract-route',
              ratio_units: 80000000,
              available: true,
            },
          ],
        },
      })

      renderDrawer()
      await screen.findByText('Main contract')
      fireEvent.click(screen.getByRole('button', { name: 'New contract' }))
      await screen.findByText('Create from template')
      const picker = await vi.waitFor(() => {
        const el = document.querySelector('#contract-template-source')
        expect(el).toBeTruthy()
        return el as HTMLElement
      })
      await userEvent.click(picker)
      await userEvent.click(
        await screen.findByRole('option', { name: 'Starter' })
      )

      expect(await screen.findByDisplayValue('0.8')).toBeTruthy()
      expect(screen.getByDisplayValue('Starter')).toBeTruthy()
      expect(
        screen.getByText(
          'Applied template version 4. Rules stay editable and failures must be fixed before saving.'
        )
      ).toBeTruthy()

      fireEvent.change(await screen.findByLabelText('Change reason'), {
        target: { value: 'from template' },
      })
      fireEvent.click(
        screen.getByRole('button', { name: 'Save and enable contract' })
      )
      await vi.waitFor(() =>
        expect(createUserContract).toHaveBeenCalledWith(
          7,
          expect.objectContaining({
            source_template_id: 3,
            source_template_version: 4,
            rules: [
              expect.objectContaining({
                model: 'claude-sonnet-5',
                channel_id: 11,
                route_group: 'contract-route',
                discount: '0.8',
              }),
            ],
          })
        )
      )
    } finally {
      restorePointerCapture()
    }
  })
  it.each([
    ['Keep my rules and confirm the latest version', '0.6', 5],
    ['Re-apply template rules', '0.7', 5],
    ['Decide later', '0.6', 4],
  ])(
    'preserves explicit conflict choice: %s',
    async (action, discount, sourceVersion) => {
      stubPointerCapture()
      try {
        const source = {
          id: 3,
          name: 'Starter',
          enabled: true,
          version: 4,
          creator_id: 1,
          updater_id: 1,
          rules: [
            {
              public_model: 'claude-sonnet-5',
              channel_id: 11,
              route_group: 'contract-route',
              ratio_units: 80000000,
              available: true,
            },
          ],
        }
        getContractTemplates.mockResolvedValue({
          success: true,
          data: {
            items: [
              {
                ...source,
                model_count: 1,
                rule_count: 1,
                stale_rule_count: 0,
                updater_name: 'admin',
                created_at: 0,
                updated_at: 0,
              },
            ],
            total: 1,
            page: 1,
            page_size: 20,
          },
        })
        getContractTemplate
          .mockResolvedValueOnce({ success: true, data: source })
          .mockResolvedValue({
            success: true,
            data: {
              ...source,
              version: 5,
              rules: source.rules.map((rule) => ({
                ...rule,
                ratio_units: 70000000,
              })),
            },
          })
        createUserContract.mockRejectedValueOnce({
          isAxiosError: true,
          response: { status: 409 },
        })
        renderDrawer()
        await screen.findByText('Main contract')
        fireEvent.click(screen.getByRole('button', { name: 'New contract' }))
        await userEvent.click(
          document.querySelector('#contract-template-source') as HTMLElement
        )
        await userEvent.click(
          await screen.findByRole('option', { name: 'Starter' })
        )
        fireEvent.change(await screen.findByDisplayValue('0.8'), {
          target: { value: '0.6' },
        })
        fireEvent.change(screen.getByLabelText('Change reason'), {
          target: { value: 'customized draft' },
        })
        fireEvent.click(
          screen.getByRole('button', { name: 'Save and enable contract' })
        )
        await screen.findByText('Contract template changed')
        expect(screen.getByDisplayValue('0.6')).toBeTruthy()
        fireEvent.click(screen.getByRole('button', { name: action }))
        await vi.waitFor(() =>
          expect(screen.queryByText('Contract template changed')).toBeNull()
        )
        expect(createUserContract).toHaveBeenCalledTimes(1)
        expect(screen.getByDisplayValue(discount)).toBeTruthy()
        fireEvent.click(
          screen.getByRole('button', { name: 'Save and enable contract' })
        )
        await vi.waitFor(() =>
          expect(createUserContract).toHaveBeenCalledTimes(2)
        )
        expect(createUserContract.mock.calls[1]?.[1]).toMatchObject({
          source_template_version: sourceVersion,
          rules: [expect.objectContaining({ discount })],
        })
      } finally {
        restorePointerCapture()
      }
    }
  )
})
