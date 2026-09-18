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
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ApiResponse, SelfCustomerContract } from '../../types'
import { CustomerContractPricing } from '../customer-contract-pricing'

const getSelfCustomerContract =
  vi.fn<() => Promise<ApiResponse<SelfCustomerContract>>>()

vi.mock('../../api', () => ({
  getSelfCustomerContract: () => getSelfCustomerContract(),
}))

vi.mock('@/lib/currency', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/currency')>()),
  formatBillingCurrencyFromUSD: (usd: number) => `$${usd}`,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) =>
      key.replace('{{discount}}', String(values?.discount ?? '')),
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
      data: { contracts: [] },
    })

    const { container } = renderPricing()

    await vi.waitFor(() => expect(container.textContent).toBe(''))
  })

  it('shows one model and contract discount row per public model without channel facts', async () => {
    getSelfCustomerContract.mockResolvedValue({
      success: true,
      data: {
        contracts: [
          {
            id: 3,
            name: 'Team contract',
            enabled: true,
            version: 4,
            models: [
              // The owner API has already merged same-model channel rules.
              { model: 'claude-sonnet-5', discount: '0.8' },
              { model: 'gemini-3-pro', discount: '0.9' },
            ],
          },
        ],
      },
    })

    const { container } = renderPricing()

    expect(await screen.findByText('claude-sonnet-5')).toBeTruthy()
    expect(screen.getByText('gemini-3-pro')).toBeTruthy()
    expect(screen.getAllByText('0.8')).toHaveLength(1)
    expect(screen.getByText('0.9')).toBeTruthy()
    expect(container.textContent).not.toContain('route_group')
    expect(container.textContent).not.toContain('channel')
    expect(container.textContent).not.toContain('provider')
  })

  it('renders several contracts separately and marks disabled ones as not in effect', async () => {
    getSelfCustomerContract.mockResolvedValue({
      success: true,
      data: {
        contracts: [
          {
            id: 3,
            name: 'Team contract',
            enabled: true,
            version: 2,
            models: [{ model: 'claude-sonnet-5', discount: '0.8' }],
          },
          {
            id: 9,
            name: 'Old contract',
            enabled: false,
            version: 2,
            models: [{ model: 'legacy-model', discount: '0.7' }],
          },
        ],
      },
    })

    renderPricing()

    expect(await screen.findByText('Team contract')).toBeTruthy()
    expect(screen.getByText('Old contract')).toBeTruthy()
    expect(
      screen.getByText(
        'This contract is disabled. Bound keys use their own group routing and pricing.'
      )
    ).toBeTruthy()
  })

  it('shows that an enabled contract has no model rules', async () => {
    getSelfCustomerContract.mockResolvedValue({
      success: true,
      data: {
        contracts: [
          {
            id: 3,
            name: 'Team contract',
            enabled: true,
            version: 2,
            models: [],
          },
        ],
      },
    })

    renderPricing()

    expect(
      await screen.findByText('This contract has no model rules')
    ).toBeTruthy()
  })

  it('keeps unavailable contract terms visible with their reason', async () => {
    getSelfCustomerContract.mockResolvedValue({
      success: true,
      data: {
        contracts: [
          {
            id: 3,
            name: 'Team contract',
            enabled: true,
            version: 2,
            models: [
              {
                model: 'unavailable-model',
                discount: '0.8',
                availability: 'unavailable',
              },
              {
                model: 'restricted-model',
                discount: '0.9',
                availability: 'group_denied',
              },
            ],
          },
        ],
      },
    })
    renderPricing()
    expect(await screen.findByText('unavailable-model')).toBeTruthy()
    expect(screen.getByText('restricted-model')).toBeTruthy()
    expect(screen.getByText('No available contract channel')).toBeTruthy()
    expect(
      screen.getByText('Contract group access is unavailable')
    ).toBeTruthy()
    expect(screen.getByText('0.8')).toBeTruthy()
    expect(screen.getByText('0.9')).toBeTruthy()
  })

  it('shows a loading failure instead of pretending the contract is unbound', async () => {
    getSelfCustomerContract.mockRejectedValue(new Error('database unavailable'))

    renderPricing()

    expect(
      await screen.findByText('Contract pricing is temporarily unavailable')
    ).toBeTruthy()
    expect(
      screen.getByText(
        'Discount details stay unavailable until the contract can be loaded.'
      )
    ).toBeTruthy()
  })
})
