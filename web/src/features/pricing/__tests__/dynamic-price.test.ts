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
import assert from 'node:assert/strict'

import { describe, expect, test } from 'vitest'

import { getBillingModeLabelKey } from '../lib/billing-mode'
import {
  getCardExamplePrice,
  getDynamicPriceUnitLabelKey,
  getDynamicPricingSummary,
  getTaskUsagePriceUnitLabelKey,
  hasTaskUsageSchema,
  isUnconfiguredTaskUsageModel,
} from '../lib/dynamic-price'
import { isTokenBasedModel } from '../lib/model-helpers'
import type { PricingModel } from '../types'
import {
  billingDisplayFixture,
  taskBillingDisplayFixture,
} from './billing-display-fixtures'

function pricingModel(overrides: Partial<PricingModel>): PricingModel {
  return {
    id: 1,
    model_name: 'test-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    billing_display: billingDisplayFixture(overrides.billing_expr),
    ...overrides,
  }
}

const summaryOptions = {
  tokenUnit: 'K' as const,
  showRechargePrice: true,
  priceRate: 3,
  usdExchangeRate: 6,
  groupRatioMultiplier: 2,
}

describe('expression price summaries', () => {
  test('shows native token charges and per-request surcharges together in K units', () => {
    const summary = getDynamicPricingSummary(
      pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("base", p * 2 + c * 8 + 100000)',
        billing_display: {
          display_version: 2,
          expression_version: 1,
          expression_hash: 'fixture',
          unit: 'usd_per_million_tokens',
          status: 'exact',
          tiers: [
            { label: 'base', unit_prices: { p: 2, c: 8 }, constant: 0.1, has_constant: true },
            { label: 'premium', unit_prices: { p: 4, c: 16 }, constant: 0.2, has_constant: true },
          ],
        },
      }),
      { tokenUnit: 'K' }
    )
    expect(summary?.primaryEntries.map((entry) => entry.field)).toEqual([
      'inputPrice', 'outputPrice', 'constant',
    ])
    const fixed = summary?.primaryEntries.find((entry) => entry.unit === 'request')
    expect(fixed?.formatted).toBe('$0.1')
    expect(fixed?.formattedRange).toBe('$0.1 – $0.2')
  })

  test('preserves a versioned parenthesized base price and its request rule', () => {
    const summary = getDynamicPricingSummary(
      pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr:
          'v1:(tier("base", p * 2 + c * 8)) * (header("x-priority") == "high" ? 2 : 1)',
      }),
      { tokenUnit: 'M' }
    )
    expect(summary?.isSpecialExpression).toBe(false)
    expect(summary?.primaryEntries.map((entry) => entry.value)).toEqual([2, 8])
    expect(summary?.hasRequestRules).toBe(true)
  })
  test.each([
    'tier("custom", max(p * 2 + c * 8, 100))',
    'tier("base", p * 2 + c * 8) * 3',
    'param("premium") ? tier("pro", p * 4 + c * 16) : tier("base", p * 2 + c * 8)',
    'tier("overflow", p * 1e999 + c * 8)',
  ])('does not invent structured prices from %s', (expression) => {
    const summary = getDynamicPricingSummary(
      pricingModel({ billing_mode: 'tiered_expr', billing_expr: expression }),
      { tokenUnit: 'M' }
    )
    expect(summary?.isSpecialExpression).toBe(true)
    expect(summary?.entries).toEqual([])
  })

  test('retains explicit zero token rates while omitting absent categories', () => {
    const summary = getDynamicPricingSummary(
      pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("free", p * 0 + c * 0)',
      }),
      { tokenUnit: 'M' }
    )
    expect(
      summary?.primaryEntries.map((entry) => [entry.field, entry.value])
    ).toEqual([
      ['inputPrice', 0],
      ['outputPrice', 0],
    ])
    expect(summary?.secondaryEntries).toEqual([])
  })

  test('includes a free task tier in its range', () => {
    const summary = getDynamicPricingSummary(
      pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr:
          'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("free", u("seconds") * 0)',
        billing_display: taskBillingDisplayFixture(
          'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("free", u("seconds") * 0)'
        ),
        billing_usage_schema: {
          seconds: { type: 'number', unit: 'second' },
          mode: { enum: ['free', 'pro'] },
        },
      }),
      { tokenUnit: 'M' }
    )
    expect(summary?.primaryEntries[0]?.value).toBe(0)
    expect(summary?.primaryEntries[0]?.formattedRange).toBe('$0 – $0.8')
  })
})

describe('task dynamic pricing', () => {
  test('treats task coefficients as dollars per unit without a token divisor', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr:
        'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)',
      billing_display: taskBillingDisplayFixture(
        'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)'
      ),
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
        mode: { enum: ['std', 'pro'] },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    assert.equal(summary.isTaskUsage, true)
    assert.equal(summary.isSpecialExpression, false)
    assert.equal(summary.tier?.label, 'std')
    assert.equal(summary.primaryEntries[0]?.value, 0.4)
    assert.equal(summary.primaryEntries[0]?.unit, 'second')
    assert.match(summary.primaryEntries[0]?.formatted ?? '', /0[.,]4/)
  })

  test('falls back for a non-canonical task expression', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr:
        'u("seconds") > 30 ? tier("long", u("seconds") * 0.3) : tier("short", u("seconds") * 0.4)',
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    assert.equal(summary.isSpecialExpression, true)
    assert.equal(summary.tiers.length, 0)
  })

  test('summarizes different task tier prices as a range while preserving the fallback price', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr:
        'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)',
      billing_display: taskBillingDisplayFixture(
        'u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)'
      ),
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
        mode: { enum: ['std', 'pro'] },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    assert.match(summary.primaryEntries[0]?.formattedRange ?? '', /0[.,]4/)
    assert.match(summary.primaryEntries[0]?.formattedRange ?? '', /0[.,]8/)
    assert.match(summary.primaryEntries[0]?.formattedRange ?? '', /\S – \S/)
    assert.match(summary.primaryEntries[0]?.formatted ?? '', /0[.,]4/)
  })

  test('omits a task price range when every tier has the same unit price', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", u("seconds") * 0.4)',
      billing_display: taskBillingDisplayFixture('tier("base", u("seconds") * 0.4)'),
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    assert.equal(summary.primaryEntries[0]?.formattedRange, undefined)
  })

  test('identifies unconfigured task usage models without inventing token pricing', () => {
    const secondsModel = pricingModel({
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
      },
    })
    const countModel = pricingModel({
      billing_usage_schema: {
        clips: { type: 'number', unit: 'count' },
      },
    })

    assert.equal(hasTaskUsageSchema(secondsModel), true)
    assert.equal(isUnconfiguredTaskUsageModel(secondsModel), true)
    assert.equal(getDynamicPricingSummary(secondsModel, summaryOptions), null)
    assert.equal(getBillingModeLabelKey(secondsModel), 'Task billing')
    assert.equal(getBillingModeLabelKey(countModel), 'Task billing')
  })

  test('does not mark configured task usage pricing as unconfigured', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", u("seconds") * 0.4)',
      billing_display: taskBillingDisplayFixture('tier("base", u("seconds") * 0.4)'),
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
      },
    })

    assert.equal(isUnconfiguredTaskUsageModel(model), false)
    assert.ok(getDynamicPricingSummary(model, summaryOptions))
  })

  test('leaves fixed per-request pricing configured when a usage schema is present', () => {
    const model = pricingModel({
      quota_type: 1,
      model_price: 0.5,
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
      },
    })

    assert.equal(isUnconfiguredTaskUsageModel(model), false)
    assert.equal(getDynamicPricingSummary(model, summaryOptions), null)
    assert.equal(isTokenBasedModel(model), false)
  })

  test('labels task token usage prices without changing chat token units', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", u("tokens") * 9.8 / 1000000)',
      billing_display: taskBillingDisplayFixture('tier("base", u("tokens") * 9.8 / 1000000)'),
      billing_usage_schema: {
        tokens: { type: 'number', unit: 'token' },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    const tokenEntry = summary.primaryEntries[0]
    assert.ok(tokenEntry)
    assert.equal(tokenEntry.unit, 'token')
    assert.equal(tokenEntry.value, 9.8)
    assert.equal(getDynamicPriceUnitLabelKey(tokenEntry), '1M token')
    assert.equal(getTaskUsagePriceUnitLabelKey('token'), '1M token')
    assert.equal(
      getDynamicPriceUnitLabelKey({
        key: 'p',
        field: 'inputPrice',
        label: 'Input',
        shortLabel: 'Input',
        labelKind: 'i18n',
        value: 2,
        formatted: '$2',
        unit: 'token',
        variable: {
          key: 'p',
          field: 'inputPrice',
          tierField: 'input_unit_cost',
          label: 'Input price',
          shortLabel: 'Input',
          side: 'input',
        },
      }),
      null
    )
  })

  test('labels task credit usage prices as a direct per-credit rate', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", u("units") * 0.14)',
      billing_display: taskBillingDisplayFixture('tier("base", u("units") * 0.14)'),
      billing_usage_schema: {
        units: { type: 'number', unit: 'credit' },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    const creditEntry = summary.primaryEntries[0]
    assert.ok(creditEntry)
    assert.equal(creditEntry.unit, 'credit')
    assert.equal(creditEntry.value, 0.14)
    assert.equal(getDynamicPriceUnitLabelKey(creditEntry), 'credit')
    assert.equal(getTaskUsagePriceUnitLabelKey('credit'), 'credit')
  })

  test('leaves token models without a usage schema unchanged', () => {
    const model = pricingModel({})

    assert.equal(hasTaskUsageSchema(model), false)
    assert.equal(isUnconfiguredTaskUsageModel(model), false)
    assert.equal(getBillingModeLabelKey(model), 'Token-based')
  })

  test('preserves all billing-mode badge states', () => {
    assert.equal(
      getBillingModeLabelKey(
        pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", u("seconds") * 0.4)',
          billing_usage_schema: {
            seconds: { type: 'number', unit: 'second' },
          },
        })
      ),
      'Task billing'
    )
    assert.equal(
      getBillingModeLabelKey(
        pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", u("clips") * 0.05)',
          billing_usage_schema: {
            clips: { type: 'number', unit: 'count' },
          },
        })
      ),
      'Task billing'
    )
    assert.equal(
      getBillingModeLabelKey(
        pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", u("tokens") * 9.8 / 1000000)',
          billing_usage_schema: {
            tokens: { type: 'number', unit: 'token' },
          },
        })
      ),
      'Task billing'
    )
    assert.equal(
      getBillingModeLabelKey(
        pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", u("units") * 0.14)',
          billing_usage_schema: {
            units: { type: 'number', unit: 'credit' },
          },
        })
      ),
      'Task billing'
    )
    assert.equal(
      getBillingModeLabelKey(
        pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", p * 2 + c * 8)',
        })
      ),
      'Dynamic Pricing'
    )
    assert.equal(getBillingModeLabelKey(pricingModel({})), 'Token-based')
    assert.equal(
      getBillingModeLabelKey(pricingModel({ quota_type: 1 })),
      'Per Request'
    )
  })

  test('marks task usage field labels as schema-owned so they are not translated', () => {
    const tokenModel = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", 0.1 + u("tokens") * 9.8 / 1000000)',
      billing_display: taskBillingDisplayFixture('tier("base", 0.1 + u("tokens") * 9.8 / 1000000)'),
      billing_usage_schema: {
        tokens: { type: 'number', unit: 'token' },
      },
    })
    const tokenSummary = getDynamicPricingSummary(tokenModel, summaryOptions)
    assert.ok(tokenSummary)
    assert.equal(tokenSummary.primaryEntries[0]?.shortLabel, 'tokens')
    assert.equal(tokenSummary.primaryEntries[0]?.labelKind, 'schema')
    // 用量费与固定附加费同时作为主要价格展示。
    assert.equal(
      tokenSummary.primaryEntries[1]?.shortLabel,
      'Additional charge'
    )
    assert.equal(tokenSummary.primaryEntries[1]?.labelKind, 'i18n')
    assert.equal(tokenSummary.secondaryEntries.length, 0)

    const multiFieldModel = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr:
        'tier("base", u("seconds") * 0.4 + u("tokens") * 9.8 / 1000000)',
      billing_display: taskBillingDisplayFixture(
        'tier("base", u("seconds") * 0.4 + u("tokens") * 9.8 / 1000000)'
      ),
      billing_usage_schema: {
        seconds: { type: 'number', unit: 'second' },
        tokens: { type: 'number', unit: 'token' },
      },
    })
    const multiSummary = getDynamicPricingSummary(
      multiFieldModel,
      summaryOptions
    )
    assert.ok(multiSummary)
    assert.equal(multiSummary.primaryEntries.length, 2)
    assert.ok(
      multiSummary.primaryEntries.every((entry) => entry.labelKind === 'schema')
    )

    const chatSummary = getDynamicPricingSummary(
      pricingModel({
        billing_mode: 'tiered_expr',
        billing_expr: 'tier("base", p * 2 + c * 8)',
      }),
      summaryOptions
    )
    assert.ok(chatSummary)
    assert.ok(
      chatSummary.primaryEntries.every((entry) => entry.labelKind === 'i18n')
    )
  })

  test('returns the first evaluated usage example for a canonical task expression', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", u("tokens") * 9.8 / 1000000)',
      billing_usage_schema: {
        tokens: { type: 'number', unit: 'token' },
      },
      billing_usage_examples: [
        // total 由后端按冻结表达式求值：9.8/1e6 × tokens。
        { label: '720p · 5s', facts: { tokens: 108000 }, total: 1.0584 },
        { label: '1080p · 5s', facts: { tokens: 243000 }, total: 2.3814 },
      ],
    })

    const example = getCardExamplePrice(model, summaryOptions)

    assert.ok(example)
    assert.equal(example.label, '720p · 5s')
    assert.match(example.formatted, /1[.,]0584/)
  })

  test('returns null when the expression is not canonical or examples are missing', () => {
    const schema = {
      tokens: { type: 'number' as const, unit: 'token' as const },
    }
    const examples = [{ label: '720p · 5s', facts: { tokens: 108000 } }]

    assert.equal(
      getCardExamplePrice(
        pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr:
            'u("tokens") * 0.00007 * (u("tokens") > 100000 ? 0.8 : 1)',
          billing_usage_schema: schema,
          billing_usage_examples: examples,
        }),
        summaryOptions
      ),
      null
    )
    assert.equal(
      getCardExamplePrice(
        pricingModel({
          billing_mode: 'tiered_expr',
          billing_expr: 'tier("base", u("tokens") * 9.8 / 1000000)',
          billing_usage_schema: schema,
        }),
        summaryOptions
      ),
      null
    )
    assert.equal(getCardExamplePrice(pricingModel({}), summaryOptions), null)
  })
})

describe('task fixed surcharges and multiplier scenarios', () => {
  test('keeps the fixed surcharge primary and ranges over provable scenarios', () => {
    const expression =
      '(u("generate_audio") ? 2 : 1) * tier("base", u("duration_seconds") * 0.4)'
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: expression,
      billing_display: taskBillingDisplayFixture(expression),
      billing_usage_schema: {
        duration_seconds: { type: 'number', unit: 'second' },
        generate_audio: { type: 'boolean' },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    // 倍率场景进入用量费范围:0.4 与 0.4×2。
    const usageEntry = summary.primaryEntries.find(
      (entry) => entry.unit === 'second'
    )
    assert.ok(usageEntry)
    assert.match(usageEntry.formattedRange ?? '', /0[.,]4/)
    assert.match(usageEntry.formattedRange ?? '', /0[.,]8/)
    // 该公式没有固定附加费,不虚构常量条目。
    assert.equal(
      summary.primaryEntries.find((entry) => entry.unit === 'request'),
      undefined
    )
  })

  test('shows declared fixed charges as real prices instead of missing values', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", 0.4 + u("duration_seconds") * 0.2)',
      billing_display: taskBillingDisplayFixture(
        'tier("base", 0.4 + u("duration_seconds") * 0.2)'
      ),
      billing_usage_schema: {
        duration_seconds: { type: 'number', unit: 'second' },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    const constantEntry = summary.primaryEntries.find(
      (entry) => entry.unit === 'request'
    )
    assert.ok(constantEntry)
    assert.equal(constantEntry.value, 0.4)
  })

  test('shows an explicit zero constant as a real price instead of a missing value', () => {
    const model = pricingModel({
      billing_mode: 'tiered_expr',
      billing_expr: 'tier("base", 0)',
      billing_display: taskBillingDisplayFixture('tier("base", 0)'),
      billing_usage_schema: {
        duration_seconds: { type: 'number', unit: 'second' },
      },
    })

    const summary = getDynamicPricingSummary(model, summaryOptions)

    assert.ok(summary)
    // 显式零价是有效报价:主项存在且值为 0,不得当作缺失。
    const constantEntry = summary.primaryEntries.find(
      (entry) => entry.unit === 'request'
    )
    assert.ok(constantEntry)
    assert.equal(constantEntry.value, 0)
  })
})
