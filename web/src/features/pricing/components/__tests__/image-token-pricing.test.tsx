import { cleanup, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { formatDynamicUnitPrice } from '../../lib/dynamic-price'
import type { BillingDisplayProjection } from '../../types'
import { DynamicPricingBreakdown } from '../dynamic-pricing-breakdown'
import { TokenPricingNote } from '../token-pricing-note'

beforeEach(() => {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
})
afterEach(cleanup)

it.each([
  { price: 1.25, unit: 'K' as const, ratio: 1, expected: '<$0.01' },
  { price: 0.001, unit: 'M' as const, ratio: 1, expected: '<$0.01' },
  { price: 1.25, unit: 'M' as const, ratio: 0.001, expected: '<$0.01' },
  { price: 0, unit: 'K' as const, ratio: 1, expected: '$0.00' },
  { price: 10, unit: 'K' as const, ratio: 1, expected: '$0.01' },
])(
  'shows $price/M at /$unit with multiplier $ratio without inflating the quote',
  ({ price, unit, ratio, expected }) => {
    expect(
      formatDynamicUnitPrice(price, {
        tokenUnit: unit,
        imageGeneration: true,
        groupRatioMultiplier: ratio,
      })
    ).toBe(expected)
  }
)

it.each([false, true])(
  'keeps a small image price honest with structured projection %s',
  (projected) => {
    render(
      <DynamicPricingBreakdown
        imageGeneration
        billingExpr='tier("base", p * 0.001 + img * 0.002 + c * 0.003)'
        billingDisplay={
          projected
            ? {
                expression_hash: 'small-image-price',
                display_version: 2,
                expression_version: 1,
                status: 'exact',
                unit: 'usd_per_million_tokens',
                tiers: [
                  {
                    label: 'base',
                    unit_prices: { p: 0.001, img: 0.002, c: 0.003 },
                  },
                ],
              }
            : undefined
        }
      />
    )
    expect(
      within(screen.getByRole('table')).getAllByText('<$0.01')
    ).toHaveLength(3)
    if (projected) {
      expect(
        screen.getByTitle(
          'Unit price before discounts: 0.001 USD per 1M tokens.'
        )
      ).toHaveTextContent('<$0.01')
    }
  }
)

it('preserves the existing precision of unrelated token pricing', () => {
  expect(formatDynamicUnitPrice(0.001, { tokenUnit: 'M' })).toBe('$0.001')
})

it.each([
  { currency: 'CNY' as const, expected: '<¥0.01' },
  { currency: 'CUSTOM' as const, expected: '<¤ 0.01' },
  { currency: 'TOKENS' as const, expected: '<$0.01' },
])(
  'discloses a small quote after conversion to $currency',
  ({ currency, expected }) => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: currency,
        usdExchangeRate: 7,
        customCurrencySymbol: '¤',
        customCurrencyExchangeRate: 0.5,
      },
    })
    expect(
      formatDynamicUnitPrice(0.001, { tokenUnit: 'M', imageGeneration: true })
    ).toBe(expected)
  }
)

it.each([false, true])(
  'applies the group multiplier once with structured projection %s',
  (projected) => {
    render(
      <DynamicPricingBreakdown
        imageGeneration
        priceMultiplier={10}
        billingExpr='tier("base", p * 0.001 + img * 0.002 + c * 0.003)'
        billingDisplay={
          projected
            ? {
                expression_hash: 'small-group-image-price',
                display_version: 2,
                expression_version: 1,
                status: 'exact',
                unit: 'usd_per_million_tokens',
                tiers: [
                  {
                    label: 'base',
                    unit_prices: { p: 0.001, img: 0.002, c: 0.003 },
                  },
                ],
              }
            : undefined
        }
      />
    )
    const table = within(screen.getByRole('table'))
    for (const price of ['$0.01', '$0.02', '$0.03']) {
      expect(table.getByText(price)).toBeVisible()
    }
  }
)

it.each(['en', 'zh'])(
  'explains the five image prices and keeps two decimals in %s',
  async (lng) => {
    const i18n = createInstance()
    await i18n.init({
      lng,
      resources: { en, zh },
      interpolation: { escapeValue: false },
    })
    const projection: BillingDisplayProjection = {
      expression_hash: 'saved-image-prices',
      display_version: 2,
      expression_version: 1,
      status: 'exact',
      unit: 'usd_per_million_tokens',
      tiers: [
        {
          label: 'base',
          unit_prices: { p: 5, c: 30, cr: 1.25, img_cr: 2, img: 8 },
        },
      ],
    }
    render(
      <I18nextProvider i18n={i18n}>
        <TokenPricingNote imageGeneration />
        <DynamicPricingBreakdown
          imageGeneration
          billingExpr='saved expression'
          billingDisplay={projection}
        />
      </I18nextProvider>
    )
    const table = within(screen.getByRole('table'))
    for (const name of lng === 'zh'
      ? ['文字输入', '文本缓存输入', '图片输入', '图片缓存输入', '图片输出']
      : [
          'Text Input',
          'Cached Text Input',
          'Image Input',
          'Cached Image Input',
          'Image Output',
        ]) {
      expect(table.getByRole('columnheader', { name })).toBeVisible()
    }
    for (const price of ['$5.00', '$30.00', '$1.25', '$2.00', '$8.00']) {
      expect(table.getByText(price, { exact: true })).toBeVisible()
    }
    expect(
      screen.getByText(
        lng === 'zh'
          ? '文本缓存输入、图片缓存输入：上游复用已处理的文字或图片时，按对应缓存价格计费；是否命中及用量以上游报告为准。'
          : 'Cached text/image input: the provider reuses previously processed text or images. Cache rates apply to the cache usage reported by the provider.'
      )
    ).toBeVisible()
  }
)

it('shows the administrator prices from the projection instead of official constants', () => {
  render(
    <DynamicPricingBreakdown
      imageGeneration
      billingExpr='saved custom expression'
      billingDisplay={{
        expression_hash: 'custom-image-prices',
        display_version: 2,
        expression_version: 1,
        status: 'exact',
        unit: 'usd_per_million_tokens',
        tiers: [
          {
            label: 'custom',
            unit_prices: { p: 7.125, c: 33, cr: 0, img_cr: 1.5, img: 11 },
          },
        ],
      }}
    />
  )
  const table = within(screen.getByRole('table'))
  for (const price of ['$7.13', '$33.00', '$0.00', '$1.50', '$11.00']) {
    expect(table.getByText(price, { exact: true })).toBeVisible()
  }
  expect(table.queryByText('$5.00')).not.toBeInTheDocument()
})
