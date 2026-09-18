import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { afterEach, beforeAll, expect, it } from 'vitest'

import { LogDiscountCell } from '../log-discount-cell'

const i18n = createInstance()
beforeAll(async () => {
  await i18n.init({
    lng: 'en',
    resources: { en: { translation: {} } },
  })
})
afterEach(cleanup)

it('keeps the full contract status visible with enough room for labels and values', () => {
  const { container } = render(
    <LogDiscountCell other={{ group_ratio: 1 }} t={i18n.t} />
  )
  const status = screen.getByText('No contract')
  expect(status).not.toHaveClass('truncate')
  expect(status).toHaveClass('whitespace-normal', 'wrap-anywhere')
  expect(container.querySelector('dl')).toHaveClass('min-w-60')
  expect(screen.getAllByText('×1', { exact: true })).toHaveLength(2)
  expect(
    screen.getByText('Final discount').nextElementSibling
  ).toHaveTextContent('×1')
})

it('wraps long contract names separately from the applied and final discount values', () => {
  const contractName =
    'EnterpriseAnnualContractWithAnUnbrokenReference0123456789'
  render(
    <LogDiscountCell
      other={{
        group_ratio: 0.5,
        contract_discount: 0.3,
        contract_name: contractName,
      }}
      t={i18n.t}
    />
  )
  const name = screen.getByText(contractName, { exact: true })
  expect(name).toHaveClass('block', 'wrap-anywhere', 'whitespace-normal')
  expect(name).not.toHaveClass('truncate')
  expect(screen.getByText('×0.3')).toBeTruthy()
  expect(screen.getByText('×0.15')).toBeTruthy()
  expect(screen.getByText('85% off')).toBeTruthy()
})

it('shows an explicit no-contract state and an unchanged final factor without duplicating factors', () => {
  render(
    <LogDiscountCell
      other={{ group_ratio: 1, contract_applicable: false }}
      t={i18n.t}
    />
  )
  expect(screen.getByText('No contract')).toBeTruthy()
  expect(screen.getAllByText('×1', { exact: true })).toHaveLength(2)
  expect(screen.queryByText('×1 ×1')).toBeNull()
})

it('keeps missing discount facts unavailable', () => {
  render(<LogDiscountCell other={null} t={i18n.t} />)
  expect(screen.getByText('—')).toBeTruthy()
  expect(screen.queryByText('×1')).toBeNull()
})

it.each([
  { group_ratio: 0.8, expected: '×0.8', tier: '20% off' },
  {
    group_ratio: 0.8,
    user_group_ratio: 0.5,
    expected: '×0.5',
    tier: '50% off',
  },
  { group_ratio: 1.5, expected: '×1.5', tier: '×1.5' },
])(
  'shows the actual final factor without a contract: $expected',
  ({ expected, tier, ...other }) => {
    render(<LogDiscountCell other={other} t={i18n.t} />)
    expect(screen.getByText('No contract')).toBeTruthy()
    const finalValue = screen.getByText('Final discount').nextElementSibling
    expect(finalValue).toHaveTextContent(expected)
    expect(finalValue).toHaveTextContent(tier)
  }
)
