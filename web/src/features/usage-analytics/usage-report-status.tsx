import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'

import type { UsageAnalyticsEnvelope } from './types'
import { formatPeriodRange, shanghaiToday } from './utils'

export function UsageReportStatus(props: {
  report?: Pick<
    UsageAnalyticsEnvelope<unknown>,
    'period' | 'generated_at' | 'published_days' | 'incomplete_days'
  >
  showReportPeriod?: boolean
  showCoverage?: boolean
}) {
  const { t, i18n } = useTranslation()
  const period = props.report?.period
  const publishedDays = props.report?.published_days
  const generatedAt = props.report?.generated_at
  const showCoverage = props.showCoverage ?? true
  if (!showCoverage && !publishedDays) return null

  // The legacy Traditional Chinese catalog does not maintain these report labels.
  const reportLanguage = i18n?.language === 'zhTW' ? 'zhCN' : undefined
  const missing = period?.days.some(
    (day) => !day.future && publishedDays && !publishedDays.includes(day.date)
  )
  const time =
    period && generatedAt
      ? new Intl.DateTimeFormat(toIntlLocale(i18n?.language), {
          dateStyle: 'short',
          timeStyle: 'short',
          timeZone: period.timezone,
        }).format(new Date(generatedAt * 1000))
      : undefined

  return (
    <p
      className='text-muted-foreground text-[13px] leading-5'
      lang={toIntlLocale(reportLanguage)}
      role='status'
    >
      {showCoverage && (
        <span className='me-3'>
          {t('Usage coverage notice', { lng: reportLanguage })}
        </span>
      )}
      {period && props.showReportPeriod && (
        <span className='me-3 inline-block'>
          {t('Reporting period (Beijing time): {{period}}', {
            lng: reportLanguage,
            period: formatPeriodRange(period),
          })}
        </span>
      )}
      {time && (
        <span className='me-3 inline-block'>
          {props.showReportPeriod
            ? t('Report snapshot: {{time}}', { lng: reportLanguage, time })
            : t('Data as of {{time}}', { lng: reportLanguage, time })}
        </span>
      )}
      {missing && (
        <span className='me-3 inline-block'>
          {t('Some usage data is still being prepared', {
            lng: reportLanguage,
          })}
        </span>
      )}
      {!!props.report?.incomplete_days?.length && (
        <span className='me-3 inline-block'>
          {t('Partial data', { lng: reportLanguage })}
        </span>
      )}
      {publishedDays?.includes(shanghaiToday()) && (
        <span className='inline-block'>
          {t('Today is an estimate', { lng: reportLanguage })}
        </span>
      )}
    </p>
  )
}
