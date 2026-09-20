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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'

import { ContractTemplateDrawer } from '../contract-template-drawer'

const api = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  get: vi.fn(),
  audits: vi.fn(),
  options: vi.fn(),
}))
vi.mock('../../template-api', () => ({
  createContractTemplate: api.create,
  updateContractTemplate: api.update,
  getContractTemplate: api.get,
  getContractTemplateAudits: api.audits,
  getContractTemplateOptions: api.options,
}))
vi.mock('@/features/users/components/user-contract-add-rule', () => ({
  CustomerContractAddRule: () => null,
}))
const translate = (key: string) => key
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: translate }) }))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const snapshot = {
  id: 31,
  name: 'Shared',
  enabled: true,
  version: 1,
  creator_id: 1,
  updater_id: 1,
  rules: [
    {
      public_model: 'chat',
      channel_id: 11,
      route_group: 'default',
      ratio_units: 80000000,
      available: true,
    },
  ],
}
beforeEach(() => {
  vi.clearAllMocks()
  api.options.mockResolvedValue({
    success: true,
    data: { channels: [], options: [] },
  })
  api.get.mockResolvedValue({ success: true, data: snapshot })
  api.create.mockResolvedValue({ success: true, data: snapshot })
  api.update.mockResolvedValue({
    success: true,
    data: { ...snapshot, version: 2 },
  })
  api.audits.mockResolvedValue({ success: true, data: { items: [], total: 0 } })
})
it('updates the same template on the second save after creation', async () => {
  render(
    <ContractTemplateDrawer
      open
      onOpenChange={vi.fn()}
      templateId={null}
      initial={{
        name: 'Shared',
        rules: [
          {
            model: 'chat',
            channel_id: 11,
            route_group: 'default',
            discount: '0.8',
            available: true,
            native_group_ratio: '1',
            effective_multiplier: '0.8',
            special_group_ratio: false,
            price: { price_type: 'model_ratio' },
          },
        ],
      }}
    />
  )
  fireEvent.change(await screen.findByLabelText('Change reason'), {
    target: { value: 'initial' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save template' }))
  await screen.findByText('Edit contract template')
  fireEvent.change(screen.getByLabelText('Template name'), {
    target: { value: 'Revised' },
  })
  fireEvent.change(screen.getByLabelText('Change reason'), {
    target: { value: 'revision' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save template' }))
  await waitFor(() =>
    expect(api.update).toHaveBeenCalledWith(
      31,
      expect.objectContaining({ expected_version: 1, name: 'Revised' })
    )
  )
  expect(api.create).toHaveBeenCalledTimes(1)
})
it('loads audit history on first opening and distinguishes failed loads from empty history', async () => {
  api.audits.mockResolvedValueOnce({ success: false })
  render(<ContractTemplateDrawer open onOpenChange={vi.fn()} templateId={31} />)
  await screen.findByLabelText('Template name')
  fireEvent.click(screen.getByRole('tab', { name: 'Audit history' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Loading failed')
  expect(api.audits).toHaveBeenCalledWith(31, 1)
  expect(screen.queryByText('No contract changes yet')).not.toBeInTheDocument()
})

it('rebuilds save-as pricing with ordinary template group ratios', async () => {
  api.options.mockResolvedValue({ success: true, data: {
    channels: [{ group: 'default', native_group_ratio: '1', special_group_ratio: false, models: [] }],
    options: [{ group: 'default', native_group_ratio: '1', models: ['chat'],
      prices: { chat: { price_type: 'model_ratio', base_model_ratio: '1' } } }] } })
  render(<ContractTemplateDrawer open onOpenChange={vi.fn()} initial={{ name: 'Copy', rules: [{
    model: 'chat', channel_id: 11, route_group: 'default', discount: '0.8', available: true,
    native_group_ratio: '0.25', effective_multiplier: '0.2', special_group_ratio: true,
    price: { price_type: 'model_ratio', base_model_ratio: '1', final_model_ratio: '0.2' },
  }] }} />)
  await screen.findByLabelText('Template name')
  expect(screen.queryByText('A special native group ratio also applies')).not.toBeInTheDocument()
})
