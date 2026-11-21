import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { expect, it } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { BillingReportStatus } from '../billing-report-status'

it('lists dates awaiting publication and removes them once a refreshed report covers them', async () => {
  const i18n = createInstance().use(initReactI18next)
  await i18n.init({ lng: 'zh', resources: { zh } })
  const report = {
    version: 'published-days',
    published_days: 1,
    expected_days: 2,
    as_of: 0,
  }
  const view = render(
    <I18nextProvider i18n={i18n}>
      <BillingReportStatus
        report={{ ...report, missing_days: ['2026-11-08'] }}
      />
    </I18nextProvider>
  )

  expect(screen.getByText('日报未发布 · 2026-11-08')).toBeVisible()

  view.rerender(
    <I18nextProvider i18n={i18n}>
      <BillingReportStatus
        report={{ ...report, version: 'complete', published_days: 2 }}
      />
    </I18nextProvider>
  )

  expect(screen.queryByText('日报未发布 · 2026-11-08')).toBeNull()
})
