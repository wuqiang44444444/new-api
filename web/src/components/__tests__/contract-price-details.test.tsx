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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { ContractPriceDetails } from '../contract-price-details'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string>) =>
      key.replaceAll(/\{\{(\w+)\}\}/g, (_, name: string) => values?.[name] ?? ''),
  }),
}))

vi.mock('@/lib/currency', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/currency')>()),
  formatBillingCurrencyFromUSD: (usd: number) => `$${usd}`,
}))

describe('contract price details', () => {
  it('summarizes final input and output prices per million tokens', () => {
    render(
      <ContractPriceDetails
        price={{
          price_type: 'model_ratio',
          base_model_ratio: '0.5',
          final_model_ratio: '0.25',
          completion_ratio: '4',
        }}
        channelMultiplier='1'
        contractDiscount='0.5'
        effectiveMultiplier='0.5'
      />
    )

    expect(screen.getByText('Input $0.5/M · Output $2/M')).toBeTruthy()
  })

  it('expands to the base and final unit prices with the multiplier chain', async () => {
    const user = userEvent.setup()
    render(
      <ContractPriceDetails
        price={{
          price_type: 'model_ratio',
          base_model_ratio: '0.5',
          final_model_ratio: '0.25',
          completion_ratio: '4',
        }}
        channelMultiplier='1'
        contractDiscount='0.5'
        effectiveMultiplier='0.5'
      />
    )

    await user.click(screen.getByRole('button', { name: 'Show pricing details' }))

    expect(screen.getByText('Base input price')).toBeTruthy()
    expect(screen.getByText('$1/M')).toBeTruthy()
    expect(screen.getByText('Base output price')).toBeTruthy()
    expect(screen.getByText('$4/M')).toBeTruthy()
    expect(screen.getByText('Channel multiplier')).toBeTruthy()
    expect(screen.getByText('1x')).toBeTruthy()
    expect(screen.getAllByText('0.5x')).toHaveLength(2)
    expect(screen.getByText('Final input price')).toBeTruthy()
    expect(screen.getByText('$0.5/M')).toBeTruthy()
    expect(screen.getByText('Final output price')).toBeTruthy()
    expect(screen.getByText('$2/M')).toBeTruthy()
  })

  it('includes the discounted image token price and per-100-token narrative', async () => {
    const user = userEvent.setup()
    render(
      <ContractPriceDetails
        price={{
          price_type: 'model_ratio',
          base_model_ratio: '0.5',
          final_model_ratio: '0.25',
          completion_ratio: '4',
          base_image_ratio: '1.25',
          final_image_ratio: '0.625',
        }}
        channelMultiplier='1'
        contractDiscount='0.5'
        effectiveMultiplier='0.5'
      />
    )

    expect(screen.getByText(/Image \$1\.25\/M/)).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Show pricing details' }))

    expect(screen.getByText('Base image token price')).toBeTruthy()
    expect(screen.getByText('$2.5/M')).toBeTruthy()
    expect(screen.getByText('Final image token price')).toBeTruthy()
    expect(
      screen.getByText(
        'Every 100 image billing tokens are priced as 125 standard tokens; with a final multiplier of 0.5, the final price is 62.5 standard tokens.'
      )
    ).toBeTruthy()
  })

  it('shows the final price per request for per-call models', () => {
    render(
      <ContractPriceDetails
        price={{
          price_type: 'model_price',
          base_model_price: '2',
          final_model_price: '1',
        }}
        channelMultiplier='1'
        contractDiscount='0.5'
        effectiveMultiplier='0.5'
      />
    )

    expect(screen.getByText('$1 / request')).toBeTruthy()
  })
})
