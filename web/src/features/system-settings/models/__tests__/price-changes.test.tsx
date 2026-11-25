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
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { afterEach, expect, it, vi } from 'vitest'

import type { ModelPricingEntry } from '@/features/model-pricing/api'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ModelRatioForm } from '../model-ratio-form'
import type { ModelPricingIssue } from '../pricing-issues'

let client: QueryClient | undefined

afterEach(() => {
  client?.clear()
  localStorage.clear()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

// deepseek-v4 input $0.27, output $1.08, cache read $0.027; gpt-image-2 $0.04
// per request.
const saved = {
  ModelPrice: '{"gpt-image-2":0.04}',
  ModelRatio: '{"deepseek-v4":0.135}',
  CacheRatio: '{"deepseek-v4":0.1}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{"deepseek-v4":4}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  BillingMode: '{}',
  BillingExpr: '{}',
  PluginBillingExpr: '{}',
  StorageHourlyExpr: '{}',
  TaskPreConsumeTokens: '{}',
  ExposeRatioEnabled: false,
}

// deepseek-v4 input $0.3, output $1.2, cache read $0.03.
const raisedDeepseek = { ModelRatio: '{"deepseek-v4":0.15}' }

function PriceChangesFixture(props: {
  draft?: Partial<typeof saved>
  savedValues: typeof saved
  onSave: (values: typeof saved) => Promise<void>
}) {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  const [titleStatus, setTitleStatus] = useState<HTMLSpanElement | null>(null)
  const form = useForm({ defaultValues: { ...props.savedValues, ...props.draft } })
  return (
    <SettingsPageProvider
      actionsContainer={actions}
      titleStatusContainer={titleStatus}
    >
      <header>
        <span ref={setTitleStatus} />
        <div ref={setActions} />
      </header>
      <ModelRatioForm
        form={form}
        savedValues={props.savedValues}
        onSave={props.onSave}
        onReset={() => undefined}
        isSaving={false}
        isResetting={false}
      />
    </SettingsPageProvider>
  )
}

async function renderForm(
  fixture: {
    draft?: Partial<typeof saved>
    saved?: Partial<typeof saved>
    issues?: ModelPricingIssue[]
    entries?: ModelPricingEntry[]
  } = {}
) {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/option/model_pricing/issues') {
      return { data: { success: true, data: fixture.issues ?? [] } }
    }
    if (url === '/api/option/model_pricing') {
      return {
        data: {
          success: true,
          data: { entries: fixture.entries ?? [], options: {}, empty_version: '' },
        },
      }
    }
    return { data: { success: true, data: [], vendors: [] } }
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { effective: {} } },
  })
  if (fixture.issues || fixture.entries) {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
  }
  const onSave = vi.fn(async (_values: typeof saved) => undefined)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const queryClient = client
  render(
    <QueryClientProvider client={queryClient}>
      <PriceChangesFixture
        draft={fixture.draft}
        savedValues={{ ...saved, ...fixture.saved }}
        onSave={onSave}
      />
    </QueryClientProvider>
  )
  await waitFor(() => expect(queryClient.isFetching()).toBe(0))
  return { onSave }
}

// The editor panel has its own Save; the page action bar's sits outside it.
const topBar = () => {
  const save = screen
    .getAllByRole('button', { name: /Save model prices/ })
    .find((button) => !button.closest('form'))
  if (!save?.parentElement) throw new Error('Page action bar not found')
  return within(save.parentElement)
}
const saveButton = () =>
  topBar().getByRole('button', { name: /Save model prices/ })

it.each(['desktop', 'mobile'] as const)(
  'loads and saves a storage-only price edit on %s',
  async (layout) => {
    if (layout === 'mobile') {
      const matchMedia = window.matchMedia
      vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
        ...matchMedia(query),
        matches: query.includes('max-width') || matchMedia(query).matches,
      }))
    }
    const user = userEvent.setup()
    const name = 'doubao-seed-2-1-lite-260915'
    const inference = 'tier("base", p * 0.1190298507 + c * 0.40298507463 + ai * 1.761047761 + cr * 0.023880059701)'
    const original =
      'tier("storage_hourly", u("storage_token_hours") * 0.0025 / 1000000)'
    const updated =
      'tier("storage_hourly", u("storage_token_hours") * 0.003 / 1000000)'
    const { onSave } = await renderForm({
      saved: {
        StorageHourlyExpr: JSON.stringify({ [name]: original }),
        BillingMode: JSON.stringify({ [name]: 'tiered_expr' }),
        BillingExpr: JSON.stringify({ [name]: inference }),
      },
    })
    await user.click(screen.getByText(name))
    const input = await screen.findByRole('textbox', {
      name: 'Hourly cache storage expression',
    })
    expect(input).toHaveValue(original)
    await user.clear(input)
    await user.paste(updated)
    const save =
      layout === 'mobile'
        ? within(
            screen.getByRole('dialog', { name: 'Edit model pricing' })
          ).getByRole('button', { name: 'Save model prices' })
        : saveButton()
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Confirm price changes',
    })
    expect(within(dialog).getAllByText(original).length).toBeGreaterThan(0)
    expect(within(dialog).getAllByText(updated).length).toBeGreaterThan(0)
    await user.click(
      within(dialog).getByRole('button', { name: 'Confirm and save' })
    )
    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    const submitted = onSave.mock.calls[0][0]
    expect(JSON.parse(submitted.StorageHourlyExpr)).toEqual({ [name]: updated })
    expect(JSON.parse(submitted.BillingExpr)).toEqual({ [name]: inference })
  }
)

const seedanceSource = 'seedance-2-0-fast-b'
const seedanceTarget = 'seedance-2-0-b'
const seedanceBudgets = { [seedanceSource]: 800000, [seedanceTarget]: 7000000 }

function renderSeedanceBudgets() {
  const names = [seedanceSource, seedanceTarget] as const
  return renderForm({
    saved: {
      BillingMode: JSON.stringify(
        Object.fromEntries(names.map((name) => [name, 'tiered_expr']))
      ),
      BillingExpr: JSON.stringify(
        Object.fromEntries(
          names.map((name) => [name, 'tier("base", u("tokens") * 5 / 1000000)'])
        )
      ),
      TaskPreConsumeTokens: JSON.stringify(seedanceBudgets),
    },
    entries: names.map((name) => ({
      model_name: name,
      version: 'saved',
      configured: {
        'task_billing_setting.preconsume_tokens': seedanceBudgets[name],
      },
      effective: {},
      usage_schema: { tokens: { type: 'number', unit: 'token' } },
      preconsume_token_budget: true,
    })),
  })
}

it('shows the copied budget in the save confirmation and restores the target budget on undo', async () => {
  const user = userEvent.setup()
  const { onSave } = await renderSeedanceBudgets()
  const target = screen.getByRole('row', { name: new RegExp(seedanceTarget) })
  await user.click(screen.getByText(seedanceSource, { exact: true }))
  await user.click(within(target).getByRole('checkbox'))
  await user.click(
    screen.getByRole('button', { name: `Copy ${seedanceSource} pricing` })
  )
  await user.click(saveButton())

  const dialog = await screen.findByRole('alertdialog', {
    name: 'Confirm price changes',
  })
  const update = within(dialog).getByRole('row', {
    name: new RegExp(seedanceTarget),
  })
  expect(update).toHaveTextContent('7,000,000')
  expect(update).toHaveTextContent('800,000')
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await user.click(screen.getByText(seedanceTarget, { exact: true }))
  expect(await screen.findByPlaceholderText('250000')).toHaveValue('800000')
  await user.click(screen.getByRole('button', { name: 'Undo changes' }))
  expect(await screen.findByPlaceholderText('250000')).toHaveValue('7000000')
  expect(saveButton()).toBeDisabled()
  expect(onSave).not.toHaveBeenCalled()
})

it('keeps a pending budget across the JSON view and restores it when changes are discarded', async () => {
  const user = userEvent.setup()
  const { onSave } = await renderSeedanceBudgets()
  await user.click(screen.getByText(seedanceSource, { exact: true }))
  const budget = await screen.findByPlaceholderText('250000')
  await user.clear(budget)
  await user.type(budget, '850000')
  await user.click(screen.getByRole('button', { name: 'Switch to JSON' }))
  await user.click(
    await screen.findByRole('button', { name: 'Switch to Visual' })
  )
  await user.click(screen.getByText(seedanceSource, { exact: true }))
  expect(await screen.findByPlaceholderText('250000')).toHaveValue('850000')
  await user.click(topBar().getByRole('button', { name: 'Discard changes' }))
  await user.click(screen.getByText(seedanceSource, { exact: true }))
  expect(await screen.findByPlaceholderText('250000')).toHaveValue('800000')
  expect(onSave).not.toHaveBeenCalled()
})

it.each(['0', '1.5'])(
  'restores a saved budget after typing %s and switching to the expression editor',
  async (input) => {
    const user = userEvent.setup()
    await renderSeedanceBudgets()
    await user.click(screen.getByText(seedanceSource, { exact: true }))
    const budget = await screen.findByPlaceholderText('250000')
    await user.clear(budget)
    await user.type(budget, input)
    await user.click(screen.getByRole('combobox', { name: 'Editor mode' }))
    await user.click(
      await screen.findByRole('option', { name: 'Expression editor' })
    )
    expect(await screen.findByPlaceholderText('250000')).toHaveValue('800000')
    expect(saveButton()).toBeDisabled()
  }
)

it.each(['desktop', 'mobile'] as const)(
  'shows the saved budget and preserves it when saving an edited expression on %s',
  async (layout) => {
    const user = userEvent.setup()
    if (layout === 'mobile') {
      const originalMatchMedia = window.matchMedia
      vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
        ...originalMatchMedia(query),
        matches:
          query.includes('max-width') || originalMatchMedia(query).matches,
      }))
    }
    const name = 'seedance-2-0-fast-b'
    const expression = 'tier("base", u("tokens") * 5 / 1000000)'
    const edited = 'tier("base", u("tokens") * 6 / 1000000)'
    const { onSave } = await renderForm({
      saved: {
        BillingMode: JSON.stringify({ [name]: 'tiered_expr' }),
        BillingExpr: JSON.stringify({ [name]: expression }),
        TaskPreConsumeTokens: JSON.stringify({ [name]: 800000 }),
      },
      entries: [
        {
          model_name: name,
          version: 'saved',
          configured: { 'task_billing_setting.preconsume_tokens': 800000 },
          effective: {},
          usage_schema: { tokens: { type: 'number', unit: 'token' } },
          preconsume_token_budget: true,
        },
      ],
    })
    await user.click(screen.getByText(name))

    expect(await screen.findByPlaceholderText('250000')).toHaveValue('800000')
    await user.click(screen.getByRole('combobox', { name: 'Editor mode' }))
    await user.click(
      await screen.findByRole('option', { name: 'Expression editor' })
    )
    fireEvent.change(
      await screen.findByRole('textbox', { name: 'Billing expression' }),
      {
        target: { value: edited },
      }
    )
    await user.click(
      layout === 'mobile'
        ? within(
            screen.getByRole('dialog', { name: 'Edit model pricing' })
          ).getByRole('button', { name: 'Save model prices' })
        : saveButton()
    )
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Confirm price changes',
    })
    await user.click(
      within(dialog).getByRole('button', { name: 'Confirm and save' })
    )

    await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
    const submitted = onSave.mock.calls[0][0]
    expect(JSON.parse(submitted.BillingExpr)).toEqual({ [name]: edited })
    expect(JSON.parse(submitted.TaskPreConsumeTokens)).toEqual({
      [name]: 800000,
    })
  }
)

async function deleteRow(
  user: ReturnType<typeof userEvent.setup>,
  name: string
) {
  const row = screen.getByRole('row', { name: new RegExp(name) })
  await user.click(within(row).getByRole('button', { name: 'Open menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
}

it('keeps a deleted saved price listed as pending deletion with an undo action', async () => {
  const user = userEvent.setup()
  const { onSave } = await renderForm()

  await deleteRow(user, 'gpt-image-2')

  const row = screen.getByRole('row', { name: /gpt-image-2/ })
  expect(within(row).getByText('Pending deletion')).toBeVisible()
  expect(within(row).getByRole('button', { name: 'Undo' })).toBeVisible()
  expect(
    within(row).queryByRole('button', { name: 'Open menu' })
  ).not.toBeInTheDocument()
  expect(onSave).not.toHaveBeenCalled()
})

it('restores the saved price into the draft when a pending deletion is undone', async () => {
  const user = userEvent.setup()
  await renderForm()
  await deleteRow(user, 'gpt-image-2')

  const row = screen.getByRole('row', { name: /gpt-image-2/ })
  await user.click(within(row).getByRole('button', { name: 'Undo' }))

  expect(within(row).queryByText('Pending deletion')).not.toBeInTheDocument()
  expect(within(row).getByRole('button', { name: 'Open menu' })).toBeVisible()
  expect(saveButton()).toBeDisabled()
})

it('offers only the edit action for a model without a saved price', async () => {
  await renderForm({
    issues: [
      {
        model_name: 'kling-v3',
        plugin_key: 'kling',
        plugin_name: 'Kling',
        reason: 'unconfigured',
      },
    ],
  })

  const row = await screen.findByRole('row', { name: /kling-v3/ })

  expect(within(row).getByRole('button', { name: 'Edit' })).toBeVisible()
  expect(
    within(row).queryByRole('button', { name: 'Open menu' })
  ).not.toBeInTheDocument()
})

it('shows each changed price of a modified model as a complete old → new line', async () => {
  await renderForm({ draft: raisedDeepseek })

  const row = screen.getByRole('row', { name: /deepseek-v4/ })
  expect(within(row).getByText('Modified')).toBeVisible()
  const changes = within(
    within(row).getByRole('list', { name: 'Unsaved price changes' })
  ).getAllByRole('listitem')

  expect(changes).toHaveLength(2)
  expect(changes[0]).toHaveTextContent('Input $0.27 → $0.3')
  expect(changes[1]).toHaveTextContent('Output $1.08 → $1.2 +1 more')
})

it('counts unsaved models on the save button and in the page status', async () => {
  await renderForm({
    draft: { ...raisedDeepseek, ModelPrice: '{}' },
  })

  expect(saveButton()).toBeEnabled()
  expect(saveButton()).toHaveTextContent('2')
  expect(screen.getByText('2 unsaved price changes')).toBeVisible()
})

it('lists every change in the confirmation and saves only after confirming', async () => {
  const user = userEvent.setup()
  const { onSave } = await renderForm({ draft: raisedDeepseek })
  await deleteRow(user, 'gpt-image-2')

  await user.click(saveButton())

  const dialog = await screen.findByRole('alertdialog', {
    name: 'Confirm price changes',
  })
  expect(
    within(dialog).getAllByRole('row', { name: /deepseek-v4|gpt-image-2/ })
  ).toHaveLength(2)
  const deleted = within(dialog).getByRole('row', { name: /gpt-image-2/ })
  expect(within(deleted).getByText('Remove saved price')).toBeVisible()
  expect(onSave).not.toHaveBeenCalled()

  await user.click(
    within(dialog).getByRole('button', { name: 'Confirm and save' })
  )

  await waitFor(() => expect(onSave).toHaveBeenCalledOnce())
  expect(JSON.parse(onSave.mock.calls[0][0].ModelPrice)).toEqual({})
  expect(JSON.parse(onSave.mock.calls[0][0].ModelRatio)).toEqual({
    'deepseek-v4': 0.15,
  })
})

it('enables save for an edit still open in the editor and confirms it', async () => {
  const user = userEvent.setup()
  const { onSave } = await renderForm()
  expect(saveButton()).toBeDisabled()

  await user.click(screen.getByText('gpt-image-2'))
  const price = await screen.findByRole('textbox', { name: 'Fixed price' })
  await user.clear(price)
  await user.type(price, '0.05')

  await waitFor(() => expect(saveButton()).toBeEnabled())
  await user.click(saveButton())
  const dialog = await screen.findByRole('alertdialog', {
    name: 'Confirm price changes',
  })
  expect(within(dialog).getByRole('row', { name: /gpt-image-2/ })).toBeVisible()
  expect(onSave).not.toHaveBeenCalled()
})

it('resets the draft to the saved prices when changes are discarded', async () => {
  const user = userEvent.setup()
  await renderForm({ draft: raisedDeepseek })

  await user.click(topBar().getByRole('button', { name: 'Discard changes' }))

  const row = screen.getByRole('row', { name: /deepseek-v4/ })
  expect(within(row).queryByText('Modified')).not.toBeInTheDocument()
  expect(saveButton()).toBeDisabled()
  expect(screen.queryByText(/unsaved price changes/)).not.toBeInTheDocument()
  expect(
    topBar().queryByRole('button', { name: 'Discard changes' })
  ).not.toBeInTheDocument()
})

it('shows the saved and draft prices above the editor and undoes them from there', async () => {
  const user = userEvent.setup()
  await renderForm({ draft: raisedDeepseek })

  await user.click(screen.getByText('deepseek-v4'))
  const card = await screen.findByRole('region', {
    name: 'Unsaved price changes',
  })
  expect(within(card).getByText('Saved price')).toBeVisible()
  expect(within(card).getByText('Price after saving')).toBeVisible()
  expect(within(card).getByText('$0.3').tagName).toBe('MARK')

  await user.click(within(card).getByRole('button', { name: 'Undo changes' }))

  expect(
    screen.queryByRole('region', { name: 'Unsaved price changes' })
  ).not.toBeInTheDocument()
  const row = screen.getByRole('row', { name: /deepseek-v4/ })
  expect(within(row).queryByText('Modified')).not.toBeInTheDocument()
})

it('keeps save in the phone top bar and moves the other actions into a menu', async () => {
  const user = userEvent.setup()
  const originalMatchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...originalMatchMedia(query),
    matches: query.includes('max-width') || originalMatchMedia(query).matches,
  }))
  await renderForm({ draft: raisedDeepseek })

  expect(saveButton()).toBeVisible()
  expect(
    topBar().queryByRole('button', { name: 'Reset prices' })
  ).not.toBeInTheDocument()
  await user.click(topBar().getByRole('button', { name: 'More actions' }))

  const menu = await screen.findByRole('menu')
  expect(
    within(menu)
      .getAllByRole('menuitem')
      .map((item) => item.textContent)
  ).toEqual(['Reset prices', 'Switch to JSON', 'Discard changes'])
})
