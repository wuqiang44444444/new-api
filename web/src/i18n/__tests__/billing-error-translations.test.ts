import { createInstance } from 'i18next'
import { expect, it } from 'vitest'

import en from '../locales/en.json'
import zh from '../locales/zh.json'

it.each([
  ['en', en],
  ['zh', zh],
] as const)(
  'resolves billing and error-report copy in %s',
  async (language, resource) => {
    const i18n = createInstance()
    await i18n.init({
      lng: language,
      resources: { [language]: resource },
      keySeparator: false,
    })
    for (const key of [
      'Error Logs',
      'Clear filter: {{model}}',
      'Page {{page}} of {{total}}',
      'Confirmed at {{time}}',
      'Version history ({{count}})',
      'Confirmed v{{version}}',
      'You are viewing confirmed version v{{version}}. Amounts are frozen at confirmation time.',
      '{{percent}}% off',
    ]) {
      expect(i18n.exists(key)).toBe(true)
    }
    expect(i18n.t('Confirmed at {{time}}', { time: '12:00' })).toBe(
      language === 'zh' ? '确认时间：12:00' : 'Confirmed at 12:00'
    )
    expect(i18n.t('{{percent}}% off', { tier: '5', percent: '50' })).toBe(
      language === 'zh' ? '5折' : '50% off'
    )
  }
)
