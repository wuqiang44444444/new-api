import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { TokenPricingNote } from '../token-pricing-note'

afterEach(cleanup)

it.each(['en', 'zh'])(
  'explains separate prices and cache uncertainty in %s',
  async (lng) => {
    const i18n = createInstance()
    await i18n.init({
      lng,
      resources: { en, zh },
      interpolation: { escapeValue: false },
    })
    render(
      <I18nextProvider i18n={i18n}>
        <TokenPricingNote />
      </I18nextProvider>
    )
    expect(
      screen.getByText(
        lng === 'zh'
          ? '“输入”和“输出”指未单独定价的 Token；图片输入、图片输出和缓存若有独立价格，则分别按对应单价计费。'
          : 'Input and output cover tokens not priced separately. Image input, image output and cache tokens use their own listed rates when separately priced.'
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        lng === 'zh'
          ? '列出缓存价格不代表接口支持缓存或本次已命中；缓存费用按上游提供的用量计收。'
          : 'Cache prices do not guarantee cache availability or a cache hit. Cache charges depend on the usage reported by the upstream provider.'
      )
    ).toBeVisible()
  }
)
