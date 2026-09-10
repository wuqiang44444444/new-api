import { describe, expect, test } from 'vitest'

import type { BillingDisplayProjection } from '../../types'
import {
  isUsableBillingDisplay,
  ruleGroupsFromBillingDisplay,
  tiersFromBillingDisplay,
} from '../billing-display'

const flashProjection: BillingDisplayProjection = {
  status: 'exact',
  unit: 'usd_per_million_tokens',
  display_version: 2,
  expression_version: 1,
  expression_hash: 'abc',
  tiers: [
    {
      label: 'base',
      unit_prices: { p: 1.5 / 6.71, cr: 0.05 / 6.71, c: 4.5 / 6.71 },
    },
  ],
  rules: [
    {
      text: 'weekday("Asia/Shanghai") >= 1 && hour("Asia/Shanghai") >= 9',
      multiplier: 2,
      fallback: 1,
      op: 'and',
      children: [
        {
          text: 'weekday("Asia/Shanghai") >= 1',
          multiplier: 2,
          source: 'time',
          time_func: 'weekday',
          timezone: 'Asia/Shanghai',
          compare_op: '>=',
          value: '1',
        },
        {
          text: 'hour("Asia/Shanghai") >= 9',
          multiplier: 2,
          source: 'time',
          time_func: 'hour',
          timezone: 'Asia/Shanghai',
          compare_op: '>=',
          value: '9',
        },
      ],
    },
  ],
}

describe('isUsableBillingDisplay', () => {
  test('accepts exact current-version projections only', () => {
    expect(isUsableBillingDisplay(flashProjection)).toBe(true)
    expect(
      isUsableBillingDisplay({
        ...flashProjection,
        status: 'opaque',
        reason: 'nonlinear_pricing',
      })
    ).toBe(false)
    expect(
      isUsableBillingDisplay({ ...flashProjection, display_version: 3 })
    ).toBe(false)
    expect(isUsableBillingDisplay(null)).toBe(false)
    expect(isUsableBillingDisplay(undefined)).toBe(false)
  })
})

describe('tiersFromBillingDisplay', () => {
  test('maps projected unit prices into display fields without guessing', () => {
    const tiers = tiersFromBillingDisplay(flashProjection)
    expect(tiers).toHaveLength(1)
    expect(tiers[0].label).toBe('base')
    // / 6.71 已被后端吸收进单价；前端不得再除一次。
    expect(tiers[0].inputPrice).toBeCloseTo(1.5 / 6.71, 15)
    expect(tiers[0].cacheReadPrice).toBeCloseTo(0.05 / 6.71, 15)
    expect(tiers[0].outputPrice).toBeCloseTo(4.5 / 6.71, 15)
  })

  test('opaque or missing projections yield no tiers', () => {
    expect(tiersFromBillingDisplay(null)).toEqual([])
    expect(
      tiersFromBillingDisplay({
        ...flashProjection,
        status: 'opaque',
        reason: 'task_usage_expression',
      })
    ).toEqual([])
  })
})

describe('ruleGroupsFromBillingDisplay', () => {
  test('flattens AND trees into structured conditions', () => {
    const groups = ruleGroupsFromBillingDisplay(flashProjection)
    expect(groups).toHaveLength(1)
    expect(groups[0].multiplier).toBe('2')
    expect(groups[0].conditions).toHaveLength(2)
    const first = groups[0].conditions[0]
    expect(first).toMatchObject({
      source: 'time',
      timeFunc: 'weekday',
      mode: 'gte',
      value: '1',
    })
  })

  test('non-1 fallback branches stay visible in the multiplier text', () => {
    const groups = ruleGroupsFromBillingDisplay({
      ...flashProjection,
      rules: [
        {
          text: 'param("service_tier") == "fast"',
          multiplier: 6,
          fallback: 1.5,
          source: 'param',
          path: 'service_tier',
          compare_op: '==',
          value: 'fast',
        },
      ],
    })
    expect(groups).toHaveLength(1)
    expect(groups[0].multiplier).toBe('6 : 1.5')
    expect(groups[0].conditionText).toBe('param("service_tier") == "fast"')
  })

  test('unstructured condition trees keep the canonical text only', () => {
    const groups = ruleGroupsFromBillingDisplay({
      ...flashProjection,
      rules: [
        {
          text: 'a || b',
          multiplier: 3,
          op: 'or',
          children: [],
          text_only: true,
          source: 'text',
        },
      ],
    })
    expect(groups).toHaveLength(1)
    expect(groups[0].conditions).toEqual([])
    expect(groups[0].conditionText).toBe('a || b')
  })
})
