import { describe, expect, it } from 'vitest'

import type {
  ContractRuleDraft,
  CustomerContractChannelGroupOption,
  CustomerContractGroupOption,
  CustomerContractRule,
} from '../../types'
import {
  buildContractBatchRules,
  draftEffectiveMultiplier,
  draftPricePreview,
  normalizeContractDiscount,
  type ContractBatchAddInput,
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

describe('buildContractBatchRules', () => {
  const channelGroups: CustomerContractChannelGroupOption[] = [
    {
      group: 'route-a',
      native_group_ratio: '0.87',
      special_group_ratio: true,
      models: [
        { model: 'model-a', channels: [{ id: 11, name: 'primary' }] },
        {
          model: 'model-b',
          channels: [
            { id: 11, name: 'primary' },
            { id: 12, name: 'backup' },
          ],
        },
      ],
    },
    {
      group: 'route-b',
      native_group_ratio: '1',
      special_group_ratio: false,
      models: [{ model: 'model-a', channels: [{ id: 11, name: 'primary' }] }],
    },
  ]
  const groupOptions: CustomerContractGroupOption[] = [
    {
      group: 'route-a',
      models: ['model-a', 'model-b'],
      prices: {
        'model-a': { price_type: 'model_ratio', base_model_ratio: '2' },
        'model-b': { price_type: 'model_price', base_model_price: '3' },
      },
      native_group_ratio: '0.87',
      special_group_ratio: true,
    },
    {
      group: 'route-b',
      models: ['model-a'],
      prices: {},
      native_group_ratio: '1',
      special_group_ratio: false,
    },
  ]

  const batch = (
    overrides: Partial<Parameters<typeof buildContractBatchRules>[0]>
  ) =>
    buildContractBatchRules({
      channelGroups,
      groupOptions,
      draftRules: [],
      routeGroup: 'route-a',
      models: [],
      channelIdsByModel: {},
      discount: '0.8',
      ...overrides,
    })

  it('expands one rule per selected channel with each model keeping its own price', () => {
    const result = batch({
      models: ['model-a', 'model-b'],
      channelIdsByModel: { 'model-a': ['11'], 'model-b': ['11', '12'] },
    })
    if (!result.ok) throw new Error(result.error.key)
    expect(result.rules.map((rule) => [rule.model, rule.channel_id])).toEqual([
      ['model-a', 11],
      ['model-b', 11],
      ['model-b', 12],
    ])
    expect(
      result.rules.every(
        (rule) =>
          rule.route_group === 'route-a' &&
          rule.discount === '0.8' &&
          rule.native_group_ratio === '0.87' &&
          rule.effective_multiplier === '0.87' &&
          rule.special_group_ratio === true
      )
    ).toBe(true)
    expect(result.rules[0]?.price).toEqual({
      price_type: 'model_ratio',
      base_model_ratio: '2',
    })
    expect(result.rules[1]?.price).toEqual({
      price_type: 'model_price',
      base_model_price: '3',
    })
  })

  it('drops duplicated model entries instead of creating duplicate rules', () => {
    const result = batch({
      models: ['model-b', 'model-b'],
      channelIdsByModel: { 'model-b': ['11'] },
    })
    if (!result.ok) throw new Error(result.error.key)
    expect(result.rules).toHaveLength(1)
  })

  type BatchCase = [
    name: string,
    overrides: Partial<ContractBatchAddInput>,
    error: { key: string; params?: Record<string, string> },
  ]

  const rejectionCases: BatchCase[] = [
    [
      'rejects a model without any selected channel',
      { models: ['model-b'], channelIdsByModel: { 'model-b': [] } },
      {
        key: 'Select a channel for model {{model}}',
        params: { model: 'model-b' },
      },
    ],
    [
      'rejects selected channels that are no longer candidates',
      { models: ['model-b'], channelIdsByModel: { 'model-b': ['11', '99'] } },
      {
        key: 'Selected channels of {{model}} are no longer available. Reselect its channels.',
        params: { model: 'model-b' },
      },
    ],
    [
      'rejects a model that is no longer a candidate in the group',
      {
        models: ['model-ghost'],
        channelIdsByModel: { 'model-ghost': ['11'] },
      },
      {
        key: 'Model {{model}} is no longer a candidate in this group. Reselect models.',
        params: { model: 'model-ghost' },
      },
    ],
    [
      'rejects batch model names that differ only by letter case',
      {
        models: ['model-a', 'MODEL-A'],
        channelIdsByModel: { 'model-a': ['11'], 'MODEL-A': ['11'] },
      },
      {
        key: 'Model names that differ only by letter case cannot coexist',
      },
    ],
    [
      'rejects an invalid discount',
      {
        models: ['model-a'],
        channelIdsByModel: { 'model-a': ['11'] },
        discount: 'abc',
      },
      { key: 'Invalid contract discount' },
    ],
    [
      'rejects a duplicate model and channel already bound in another group',
      {
        models: ['model-a'],
        channelIdsByModel: { 'model-a': ['11'] },
        draftRules: [
          {
            model: 'model-a',
            channel_id: 11,
            route_group: 'route-b',
            discount: '0.8',
            available: true,
            native_group_ratio: '1',
            effective_multiplier: '0.8',
            special_group_ratio: false,
            price: { price_type: 'model_ratio' },
          },
        ],
      },
      {
        key: 'This model already binds channels: {{channels}}',
        params: { channels: 'primary' },
      },
    ],
    [
      'rejects a discount that differs from the model existing discount',
      {
        models: ['model-b'],
        channelIdsByModel: { 'model-b': ['12'] },
        draftRules: [
          {
            model: 'model-b',
            channel_id: 11,
            route_group: 'route-a',
            discount: '0.9',
            available: true,
            native_group_ratio: '0.87',
            effective_multiplier: '0.783',
            special_group_ratio: true,
            price: { price_type: 'model_ratio' },
          },
        ],
      },
      {
        key: 'All channels of one model must share the same contract discount in this save',
      },
    ],
  ]

  it.each(rejectionCases)('%s', (_name, overrides, error) => {
    const result = batch(overrides)
    expect(result).toEqual({ ok: false, error })
  })

  it('accepts another channel for a model already contracted at the same discount', () => {
    const existing: ContractRuleDraft = {
      model: 'model-b',
      channel_id: 11,
      route_group: 'route-a',
      discount: '0.8',
      available: true,
      native_group_ratio: '0.87',
      effective_multiplier: '0.696',
      special_group_ratio: true,
      price: { price_type: 'model_ratio' },
    }
    const result = batch({
      models: ['model-b'],
      channelIdsByModel: { 'model-b': ['12'] },
      draftRules: [existing],
    })
    if (!result.ok) throw new Error(result.error.key)
    expect(result.rules).toEqual([
      expect.objectContaining({
        model: 'model-b',
        channel_id: 12,
        discount: '0.8',
      }),
    ])
  })
})
