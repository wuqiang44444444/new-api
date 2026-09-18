import { describe, expect, it } from 'vitest'

import { formatVersionQuota } from '../version-money'

const version = { quota_per_unit: 1000000, currency: 'USD', currency_rate: 1 }
describe('frozen statement amounts', () => {
  it('uses the frozen non-default quota conversion and keeps refund signs', () => {
    expect(formatVersionQuota(17570, version)).toBe('$0.01757000')
    expect(formatVersionQuota('-17570', version)).toBe('-$0.01757000')
    expect(
      formatVersionQuota(17570, {
        ...version,
        currency: 'CNY',
        currency_rate: 7,
      })
    ).toBe('¥0.12299000')
  })
  it('preserves integers larger than the JavaScript number range', () => {
    expect(formatVersionQuota('9007199254740993', version)).toBe(
      '$9007199254.74099300'
    )
  })
  it('rounds the final amount half away from zero', () => {
    expect(
      formatVersionQuota(1, { ...version, quota_per_unit: 200000000 })
    ).toBe('$0.00000001')
    expect(
      formatVersionQuota(-1, { ...version, quota_per_unit: 200000000 })
    ).toBe('-$0.00000001')
  })
})
