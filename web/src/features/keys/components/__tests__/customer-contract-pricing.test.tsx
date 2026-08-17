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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ApiResponse, SelfCustomerContract } from '../../types'
import { CustomerContractPricing } from '../customer-contract-pricing'

const getSelfCustomerContract =
  vi.fn<() => Promise<ApiResponse<SelfCustomerContract>>>()

vi.mock('../../api', () => ({
  getSelfCustomerContract: () => getSelfCustomerContract(),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string>) =>
      key.replace('{{discount}}', values?.discount || ''),
  }),
}))

function renderPricing() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <CustomerContractPricing />
    </QueryClientProvider>
  )
}

describe('customer contract pricing on the API key page', () => {
  beforeEach(() => {
    getSelfCustomerContract.mockReset()
  })

  it('does not change the native API key page for users without a contract', async () => {
    getSelfCustomerContract.mockResolvedValue({
      success: true,
      data: { contract_mode: false, contract_version: 0, models: [] },
    })

    const { container } = renderPricing()

    await vi.waitFor(() => expect(container.textContent).toBe(''))
  })

  it('shows only customer-visible model, discount, and price facts', async () => {
    getSelfCustomerContract.mockResolvedValue({
      success: true,
      data: {
        contract_mode: true,
        contract_version: 4,
        models: [
          {
            model: 'claude-sonnet-5',
            discount: '0.8',
            available: true,
            price: {
              price_type: 'model_ratio',
              current_discounted_price: '0.696',
            },
          },
          {
            model: 'claude-opus-4-8',
            discount: '0.6',
            available: false,
            price: { price_type: 'tiered_multiplier' },
          },
        ],
      },
    })

    const { container } = renderPricing()

    expect(await screen.findByText('claude-sonnet-5')).toBeTruthy()
    expect(screen.getByText(/0\.696/)).toBeTruthy()
    expect(screen.getByText('Tiered price × 0.6')).toBeTruthy()
    expect(screen.getByText('Unavailable')).toBeTruthy()
    expect(container.textContent).not.toContain('route_group')
    expect(container.textContent).not.toContain('channel')
    expect(container.textContent).not.toContain('provider')
  })

  it('makes an enabled zero-rule contract visibly fail closed', async () => {
    getSelfCustomerContract.mockResolvedValue({
      success: true,
      data: { contract_mode: true, contract_version: 2, models: [] },
    })

    renderPricing()

    expect(
      await screen.findByText('No models are currently authorized')
    ).toBeTruthy()
    expect(
      screen.getByText(
        'All model calls from every API key are currently denied.'
      )
    ).toBeTruthy()
  })

  it('shows a fail-closed warning when contract facts cannot be loaded', async () => {
    getSelfCustomerContract.mockRejectedValue(new Error('database unavailable'))

    renderPricing()

    expect(
      await screen.findByText('Contract pricing is temporarily unavailable')
    ).toBeTruthy()
    expect(
      screen.getByText(
        'Model access remains fail-closed until the contract can be loaded.'
      )
    ).toBeTruthy()
  })
})
