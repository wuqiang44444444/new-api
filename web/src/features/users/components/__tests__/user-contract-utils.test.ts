import { describe, expect, it } from 'vitest'

import type { CustomerContractRule } from '../../types'
import {
  draftCurrentPrice,
  normalizeContractDiscount,
} from '../user-contract-utils'

describe('customer contract pricing form utilities', () => {
  it('normalizes all supported discount notations to the canonical decimal', () => {
    expect(normalizeContractDiscount('0.8')).toBe('0.8')
    expect(normalizeContractDiscount('80%')).toBe('0.8')
    expect(normalizeContractDiscount('8折')).toBe('0.8')
  })

  it('shows a discounted price for a newly added unsaved rule', () => {
    const rule = {
      model: 'new-model',
      route_group: 'contract-route',
      discount: '0.8',
      available: true,
      native_group_ratio: '0.87',
      effective_multiplier: '0.87',
      special_group_ratio: false,
      price: {
        price_type: 'model_ratio',
        current_discounted_price: '0.87',
      },
    } satisfies CustomerContractRule

    expect(draftCurrentPrice(rule)).toBe('0.696')
  })
})
