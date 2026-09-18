import { expect, it } from 'vitest'

import { getContractDiscountFact } from '../discount-display'

it.each([
  { contract_discount: '0.3' },
  { contract_discount: 0.3, contract_applicable: true },
])('retains the recorded contract factor: %j', (other) => {
  expect(getContractDiscountFact(other)).toEqual({
    state: 'applied',
    ratio: 0.3,
  })
})

it.each([
  { contract_applicable: true },
  { contract_id: 5 },
  { contract_name: 'Annual contract' },
  { contract_version: 2 },
  { contract_discount: 'invalid' },
  { contract_discount: -0.3 },
  { contract_discount: 0.3, contract_applicable: false },
])(
  'does not invent a final contract factor for incomplete or conflicting facts: %j',
  (other) => {
    expect(getContractDiscountFact(other)).toEqual({
      state: 'unrecorded',
      ratio: null,
    })
  }
)
