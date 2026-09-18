import { expect, it } from 'vitest'

import { assertCustomerBillingPrecision } from '../billing-precision'

it('accepts exact quota integers, refunds, null estimates and decimal discounts', () => {
  expect(() =>
    assertCustomerBillingPrecision({
      original_quota: Number.MAX_SAFE_INTEGER,
      summary: { net_quota: -17 },
      groups: [{ original_quota: null, discount_ratio: 0.87 }],
    })
  ).not.toThrow()
})

it('rejects unsafe aggregate, model and discount combination amounts before rendering', () => {
  const unsafe = Number('9007199254740993')
  for (const bill of [
    { summary: { net_quota: unsafe } },
    { groups: [{ models: [{ original_quota: unsafe }] }] },
    { discount_combinations: [{ discount_quota: -unsafe }] },
    { items: [{ usage: { refund_quota: unsafe } }] },
  ]) {
    expect(() => assertCustomerBillingPrecision(bill)).toThrow()
  }
})
