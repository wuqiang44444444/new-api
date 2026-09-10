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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ApiResponse, UserContractEntities } from '@/features/users/types'

import type { CustomerContractMigrationPreview } from '../../types'
import { CustomerContractMigrationDialog } from '../customer-contract-migration-dialog'

const { getPreview, migrate } = vi.hoisted(() => ({
  getPreview: vi.fn<() => Promise<ApiResponse<CustomerContractMigrationPreview[]>>>(),
  migrate: vi.fn<
    (payload: Record<string, unknown>) => Promise<ApiResponse<UserContractEntities>>
  >(),
}))

vi.mock('../../api', () => ({
  getCustomerContractMigrationPreview: () => getPreview(),
  migrateCustomerContract: (payload: Record<string, unknown>) =>
    migrate(payload),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const previews: CustomerContractMigrationPreview[] = [
  {
    user_id: 7,
    username: 'customer-a',
    contract_enabled: true,
    bound_token_count: 3,
    already_migrated: false,
    rules: [
      {
        public_model: 'claude-sonnet-5',
        route_group: 'default',
        ratio_units: 80_000_000,
        channel_ids: [11],
        resolved_channel_id: 11,
        needs_decision: false,
      },
      {
        public_model: 'gemini-3-pro',
        route_group: 'vip',
        ratio_units: 100_000_000,
        channel_ids: [12, 13],
        resolved_channel_id: 0,
        needs_decision: true,
      },
    ],
  },
  {
    user_id: 8,
    username: 'migrated-user',
    contract_enabled: false,
    bound_token_count: 0,
    already_migrated: true,
    rules: [],
  },
]

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <CustomerContractMigrationDialog
        open
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>
  )
}

function selectTriggers(): HTMLButtonElement[] {
  return [
    ...document.querySelectorAll<HTMLButtonElement>('[data-slot="select-trigger"]'),
  ]
}

async function pickChannel(
  user: ReturnType<typeof userEvent.setup>,
  trigger: HTMLButtonElement,
  label: string
) {
  await user.click(trigger)
  await user.click(await screen.findByRole('option', { name: label }))
}

describe('legacy contract migration dialog', () => {
  beforeEach(() => {
    getPreview.mockReset().mockResolvedValue({ success: true, data: previews })
    migrate.mockReset().mockResolvedValue({
      success: true,
      data: { user_id: 7, username: 'customer-a', contracts: [] },
    })
  })

  it('autofills resolved channels and disables migration until every decision is made', async () => {
    renderDialog()

    expect(await screen.findByText('customer-a')).toBeTruthy()
    expect(screen.getByText('migrated-user')).toBeTruthy()
    expect(screen.getByText('Migrated')).toBeTruthy()
    expect(screen.getByText('This user has already been migrated.')).toBeTruthy()

    const migrateButton = screen.getByRole('button', { name: 'Migrate' })
    expect(migrateButton.hasAttribute('disabled')).toBe(true)
    expect(migrate).not.toHaveBeenCalled()
  })

  it('allows migration after refreshing a previously unavailable channel without losing the reason', async () => {
    const initial = { ...previews[0], rules: [{ ...previews[0].rules[0], channel_ids: [], resolved_channel_id: 0, needs_decision: true }] }
    getPreview.mockResolvedValueOnce({ success: true, data: [initial] })
    renderDialog()
    await screen.findByText('customer-a')
    fireEvent.change(screen.getByLabelText('Change reason'), { target: { value: 'channel repaired' } })
    expect(screen.getByRole('button', { name: 'Migrate' })).toBeDisabled()
    getPreview.mockResolvedValue({ success: true, data: [{ ...initial, rules: [previews[0].rules[0]] }] })
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Migrate' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: 'Migrate' }))
    await vi.waitFor(() => expect(migrate).toHaveBeenCalledWith(expect.objectContaining({ reason: 'channel repaired', channel_overrides: { 'claude-sonnet-5': 11 } })))
  })

  it('submits the explicit channel decision with the contract name and reason', async () => {
    const user = userEvent.setup()
    renderDialog()

    await screen.findByText('customer-a')
    // gemini-3-pro has two candidates; the admin must pick one explicitly.
    const triggers = selectTriggers()
    expect(triggers).toHaveLength(2)
    await pickChannel(user, triggers[1], '#13')

    fireEvent.change(screen.getByLabelText('Contract name'), {
      target: { value: 'Migrated contract' },
    })
    fireEvent.change(screen.getByLabelText('Change reason'), {
      target: { value: 'switching to contract entities' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Migrate' }))

    await vi.waitFor(() => expect(migrate).toHaveBeenCalledTimes(1))
    expect(migrate).toHaveBeenCalledWith({
      user_id: 7,
      contract_name: 'Migrated contract',
      reason: 'switching to contract entities',
      channel_overrides: {
        'claude-sonnet-5': 11,
        'gemini-3-pro': 13,
      },
    })
    expect(toast.success).toHaveBeenCalledWith('Contract created')
  })

  it('reports an already-migrated conflict without a crash', async () => {
    const user = userEvent.setup()
    migrate.mockRejectedValueOnce({
      isAxiosError: true,
      response: { status: 409 },
    })
    renderDialog()

    await screen.findByText('customer-a')
    await pickChannel(user, selectTriggers()[1], '#12')
    await user.type(screen.getByLabelText('Change reason'), 'migrate now')
    await user.click(screen.getByRole('button', { name: 'Migrate' }))

    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        'This user has already been migrated or has no rules to migrate.'
      )
    )
  })
})
