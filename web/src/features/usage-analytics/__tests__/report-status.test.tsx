import { render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zhCN from '@/i18n/locales/zh.json'

import { UsageReportStatus } from '../usage-report-status'

const report = {
  period: {
    period: 'day' as const,
    date: '2026-11-09',
    timezone: 'Asia/Shanghai',
    start_timestamp: 1794153600,
    end_timestamp: 1794240000,
    days: [{ date: '2026-11-09', start: 1794153600, end: 1794240000 }],
  },
  generated_at: 1794374400,
  published_days: ['2026-11-09'],
}

it.each([
  ['zhCN', true, '报表快照时间'],
  ['zhTW', true, '报表快照时间'],
  ['zhTW', false, '数据截至'],
] as const)(
  'renders the coverage and timestamp together in Chinese for %s (period shown: %s)',
  async (language, showReportPeriod, timeLabel) => {
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      fallbackLng: 'en',
      resources: { en, zhCN, zhTW },
      nsSeparator: false,
    })
    render(
      <I18nextProvider i18n={i18n}>
        <UsageReportStatus
          report={report}
          showReportPeriod={showReportPeriod}
        />
      </I18nextProvider>
    )

    const status = within(screen.getByRole('status'))
    expect(status.getByText(/^统计仅覆盖/)).toBeVisible()
    expect(status.getByText(new RegExp(timeLabel))).toBeVisible()
    if (showReportPeriod) {
      expect(status.getByText('统计周期（北京时间）：2026-11-09')).toBeVisible()
    }
    expect(
      status.queryByText(/Reporting period|Report snapshot|Data as of/)
    ).not.toBeInTheDocument()
  }
)
