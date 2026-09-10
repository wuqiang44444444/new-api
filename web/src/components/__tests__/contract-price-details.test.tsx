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

import { billingDisplayFixture } from '@/features/pricing/__tests__/billing-display-fixtures'
import fixtures from '@/features/pricing/__tests__/billing-display-fixtures.json'
import type { BillingDisplayProjection } from '@/features/pricing/types'

import { ContractPriceDetails } from '../contract-price-details'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    i18n: { language: 'en' },
    t: (key: string, values?: Record<string, string>) =>
      key.replaceAll(
        /\{\{(\w+)\}\}/g,
        (_, name: string) => values?.[name] ?? ''
      ),
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

    await user.click(
      screen.getByRole('button', { name: 'Show pricing details' })
    )

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

    await user.click(
      screen.getByRole('button', { name: 'Show pricing details' })
    )

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

  it('expands tiered expressions into tier prices with the contract multiplier applied', async () => {
    const user = userEvent.setup()
    render(
      <ContractPriceDetails
        price={{
          price_type: 'tiered_multiplier',
          billing_mode: 'tiered_expr',
          billing_expr:
            'len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long_context", p * 6 + c * 30)',
          billing_display: billingDisplayFixture(
            'len <= 200000 ? tier("standard", p * 3 + c * 15) : tier("long_context", p * 6 + c * 30)'
          ),
        }}
        channelMultiplier='1'
        contractDiscount='0.5'
        effectiveMultiplier='0.5'
      />
    )

    await user.click(
      screen.getByRole('button', { name: 'Show pricing details' })
    )

    expect(
      screen.getByText(
        'The tiered expression defines the base price; the effective contract multiplier 0.5 applies to the billed amount, and tier thresholds are not discounted.'
      )
    ).toBeTruthy()
    expect(screen.getAllByText('standard').length).toBeGreaterThan(0)
    expect(screen.getAllByText('long_context').length).toBeGreaterThan(0)
    expect(
      screen.getAllByText('Full input length ≤ 200K').length
    ).toBeGreaterThan(0)
    expect(screen.getAllByText('$1.5').length).toBeGreaterThan(0)
    expect(screen.getAllByText('$3').length).toBeGreaterThan(0)
  })

  it('falls back to the raw expression when the tiered expression cannot be parsed', async () => {
    const user = userEvent.setup()
    render(
      <ContractPriceDetails
        price={{
          price_type: 'tiered_multiplier',
          billing_mode: 'tiered_expr',
          billing_expr: 'max(p * 3, 10) + c * 15',
        }}
        channelMultiplier='1'
        contractDiscount='0.5'
        effectiveMultiplier='0.5'
      />
    )

    await user.click(
      screen.getByRole('button', { name: 'Show pricing details' })
    )

    expect(screen.getByText('Raw expression')).toBeTruthy()
    expect(screen.getByText('max(p * 3, 10) + c * 15')).toBeTruthy()
  })
})

it('renders the contract response projection including both time-price branches', async () => {
  const expression = Object.keys(fixtures).find(
    (key) => key.includes('weekday(') && key.endsWith('/ 6.71')
  )
  if (!expression) throw new Error('Missing time-price fixture')
  const projection = fixtures[
    expression as keyof typeof fixtures
  ] as BillingDisplayProjection
  render(
    <ContractPriceDetails
      price={{
        price_type: 'tiered_multiplier',
        billing_mode: 'tiered_expr',
        billing_expr: expression,
        billing_display: projection,
      }}
      channelMultiplier='1'
      contractDiscount='0.87'
      effectiveMultiplier='0.87'
    />
  )
  await userEvent.click(screen.getByRole('button'))
  expect(screen.getByText(/Condition met/)).toBeVisible()
  expect(screen.getByText(/0\.38897/)).toBeVisible()
  expect(
    screen.queryByText('Unable to parse structured pricing')
  ).not.toBeInTheDocument()
})
