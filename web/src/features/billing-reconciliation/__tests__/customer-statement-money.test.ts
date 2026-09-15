import { afterEach, expect, it } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { formatCustomerStatementQuota } from '../lib'

const originalConfig = useSystemConfigStore.getState().config
afterEach(() => useSystemConfigStore.setState({ config: originalConfig }))

it.each([
  ['USD', 1, '$0.000002'],
  ['USD', -1, '-$0.000002'],
  ['CNY', 1, '¥0.000014'],
  ['TOKENS', 1, '1'],
] as const)(
  'preserves the monetary unit for %s with %s quota',
  (type, quota, expected) => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: type,
        usdExchangeRate: 7,
      },
    })
    expect(formatCustomerStatementQuota(quota)).toBe(expected)
  }
)

it('rounds subprecision conversions without inflating them to a minimum charge', () => {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY',
      usdExchangeRate: 0.001,
    },
  })
  expect(formatCustomerStatementQuota(1)).toBe('¥0')
  expect(formatCustomerStatementQuota(undefined)).toBe('-')
})
