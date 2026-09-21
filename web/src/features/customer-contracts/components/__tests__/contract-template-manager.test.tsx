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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { ContractTemplateManager } from '../contract-template-manager'

const api = vi.hoisted(() => ({
  list: vi.fn(),
  options: vi.fn(),
  get: vi.fn(),
}))

vi.mock('../../template-api', () => ({
  getContractTemplates: api.list,
  getContractTemplateOptions: api.options,
  getContractTemplate: api.get,
  createContractTemplate: vi.fn(),
  updateContractTemplate: vi.fn(),
  getContractTemplateAudits: vi.fn(),
}))

let queryClient: QueryClient

beforeEach(() => {
  api.list.mockResolvedValue({
    success: true,
    data: { items: [], total: 0, page: 1, page_size: 20 },
  })
  api.options.mockResolvedValue({
    success: true,
    data: { channels: [], options: [], customer_context: false },
  })
  api.get.mockResolvedValue({
    success: true,
    data: {
      id: 31,
      name: 'Shared template',
      enabled: true,
      version: 1,
      rules: [],
    },
  })
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
})

afterEach(() => queryClient.clear())

function renderManager() {
  const router = createRouter({
    routeTree: createRootRoute({ component: ContractTemplateManager }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

it('opens the creation form from an empty list and clears the draft after cancel', async () => {
  renderManager()
  await screen.findByText('No contract templates')

  fireEvent.click(screen.getByRole('button', { name: 'New contract template' }))
  expect(
    await screen.findByRole('dialog', { name: 'New contract template' })
  ).toBeInTheDocument()
  const name = await screen.findByLabelText('Template name')
  expect(name).toHaveValue('')
  fireEvent.change(name, { target: { value: 'Unsaved draft' } })
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )

  fireEvent.click(screen.getByRole('button', { name: 'New contract template' }))
  expect(await screen.findByLabelText('Template name')).toHaveValue('')
})

it('opens the selected template for editing and starts a blank form when creating next', async () => {
  api.list.mockResolvedValue({
    success: true,
    data: {
      items: [
        {
          id: 31,
          name: 'Shared template',
          enabled: true,
          model_count: 0,
          rule_count: 0,
          stale_rule_count: 0,
          updater_id: 1,
          updater_name: 'Admin',
          updated_at: 1,
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    },
  })
  renderManager()
  fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
  expect(
    await screen.findByRole('dialog', { name: 'Edit contract template' })
  ).toBeInTheDocument()
  expect(await screen.findByLabelText('Template name')).toHaveValue(
    'Shared template'
  )
  expect(api.get).toHaveBeenCalledWith(31)
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )

  fireEvent.click(screen.getByRole('button', { name: 'New contract template' }))
  expect(
    await screen.findByRole('dialog', { name: 'New contract template' })
  ).toBeInTheDocument()
  expect(await screen.findByLabelText('Template name')).toHaveValue('')
})
