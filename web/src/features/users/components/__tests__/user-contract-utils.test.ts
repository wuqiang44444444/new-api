import { describe, expect, it } from 'vitest'

import type { CustomerContractRule } from '../../types'
import {
  draftEffectiveMultiplier,
  draftPricePreview,
  normalizeContractDiscount,
} from '../user-contract-utils'

describe('customer contract pricing form utilities', () => {
  it('normalizes all supported discount notations to the canonical decimal', () => {
    expect(normalizeContractDiscount('0.8')).toBe('0.8')
    expect(normalizeContractDiscount('80%')).toBe('0.8')
    expect(normalizeContractDiscount('8折')).toBe('0.8')
  })

  it('computes the effective multiplier from the channel ratio and the discount', () => {
    const rule = {
      native_group_ratio: '0.87',
      discount: '0.8',
    } as CustomerContractRule
    expect(draftEffectiveMultiplier(rule)).toBe('0.696')
  })

  it('recalculates final ratios for a draft discount before saving', () => {
    const rule = {
      model: 'image-model',
      route_group: 'contract-route',
      discount: '0.5',
      available: true,
      native_group_ratio: '1',
      effective_multiplier: '0.8',
      special_group_ratio: false,
      price: {
        price_type: 'model_ratio',
        base_model_ratio: '1',
        base_image_ratio: '1.25',
        final_model_ratio: '0.8',
        final_image_ratio: '1',
      },
    } satisfies CustomerContractRule

    const preview = draftPricePreview(rule)
    expect(preview.final_model_ratio).toBe('0.5')
    expect(preview.final_image_ratio).toBe('0.625')
    expect(preview.current_discounted_price).toBe('0.5')
  })

  it('recalculates a per-call price from its base price', () => {
    const rule = {
      model: 'call-model',
      route_group: 'contract-route',
      discount: '0.5',
      available: true,
      native_group_ratio: '1',
      effective_multiplier: '0.8',
      special_group_ratio: false,
      price: {
        price_type: 'model_price',
        base_model_price: '2',
        final_model_price: '1.6',
      },
    } satisfies CustomerContractRule

    const preview = draftPricePreview(rule)
    expect(preview.final_model_price).toBe('1')
    expect(preview.current_discounted_price).toBe('1')
  })

  it('rescales a legacy discounted price when no base ratio is available', () => {
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

    expect(draftPricePreview(rule).current_discounted_price).toBe('0.696')
  })

  it('keeps the saved preview when the draft discount is invalid', () => {
    const rule = {
      model: 'image-model',
      route_group: 'contract-route',
      discount: 'abc',
      available: true,
      native_group_ratio: '1',
      effective_multiplier: '0.5',
      special_group_ratio: false,
      price: {
        price_type: 'model_ratio',
        base_image_ratio: '1.25',
        final_image_ratio: '0.625',
      },
    } satisfies CustomerContractRule

    expect(draftPricePreview(rule)).toBe(rule.price)
  })
})
