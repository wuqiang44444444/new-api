import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import zh from '@/i18n/locales/zh.json'

import { BillingQualityNotice } from '../billing-quality-notice'

it.each(['en', 'zh'])(
  'separates unavailable original amounts from usage and funds in %s',
  async (language) => {
    const i18n = createInstance().use(initReactI18next)
    await i18n.init({ lng: language, resources: { en, zh } })
    render(
      <I18nextProvider i18n={i18n}>
        <BillingQualityNotice
          quality={{ status: 'partial', missing_historical_price_rows: 1 }}
        />
      </I18nextProvider>
    )
    expect(screen.getByText(i18n.t('Usage details available'))).toBeTruthy()
    expect(
      screen.getByText(
        i18n.t('List price or savings unavailable for {{count}} records', {
          count: 1,
        })
      )
    ).toBeTruthy()
    expect(
      screen.getByText(
        i18n.t(
          'Amounts reflect recorded charges and refunds. Usage completeness does not certify wallet reconciliation.'
        )
      )
    ).toBeTruthy()
  }
)

it('reports missing token usage independently of original prices', async () => {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'en', resources: { en } })
  render(
    <I18nextProvider i18n={i18n}>
      <BillingQualityNotice
        compact
        quality={{ status: 'partial', input_tokens_unavailable_requests: 14 }}
      />
    </I18nextProvider>
  )
  expect(screen.getByText('Incomplete usage details')).toBeTruthy()
  expect(screen.queryByText(/list price or savings unavailable/i)).toBeNull()
  expect(
    screen.getByText('Input token totals unavailable: 14 records')
  ).toBeTruthy()
})
