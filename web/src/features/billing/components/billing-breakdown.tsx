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
import { DatabaseZap, Layers3, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber, formatQuota } from '@/lib/format'

import type { BillingBreakdownSummary } from '../types'

export function BillingBreakdown(props: { summary?: BillingBreakdownSummary }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage)
  const summary = props.summary
  if (!summary?.cache && !summary?.context && !summary?.billing_mode) {
    return null
  }

  return (
    <section className='flex flex-col gap-2'>
      <div>
        <h2 className='text-sm font-semibold'>{t('Usage breakdown')}</h2>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Categories overlap and use settled request cost; they are not recalculated line-item prices.'
          )}
        </p>
      </div>
      <div className='grid gap-2 md:grid-cols-3'>
        {summary.cache && (
          <Card size='sm'>
            <CardHeader>
              <div className='flex items-center gap-2'>
                <IconBadge tone='info' size='stat'>
                  <DatabaseZap />
                </IconBadge>
                <CardTitle>{t('Cache activity')}</CardTitle>
              </div>
            </CardHeader>
            <CardContent className='flex flex-col gap-2'>
              <div className='font-mono text-xl font-bold tabular-nums'>
                {new Intl.NumberFormat(locale, {
                  style: 'percent',
                  maximumFractionDigits: 1,
                }).format(summary.cache.hit_request_ratio)}
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('{{hits}} hit requests · {{writes}} write requests', {
                  hits: formatCompactNumber(summary.cache.hit_requests, locale),
                  writes: formatCompactNumber(
                    summary.cache.write_requests,
                    locale
                  ),
                })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('{{read}} read tokens · {{write}} write tokens', {
                  read: formatCompactNumber(summary.cache.read_tokens, locale),
                  write: formatCompactNumber(
                    summary.cache.write_tokens,
                    locale
                  ),
                })}
              </p>
              <p className='text-xs'>
                {t('Settled cost of hit requests: {{amount}}', {
                  amount: formatQuota(summary.cache.hit_request_gross_quota),
                })}
              </p>
            </CardContent>
          </Card>
        )}

        {summary.context && (
          <Card size='sm'>
            <CardHeader>
              <div className='flex items-center gap-2'>
                <IconBadge tone='chart-4' size='stat'>
                  <Layers3 />
                </IconBadge>
                <CardTitle>{t('Long / short context')}</CardTitle>
              </div>
            </CardHeader>
            <CardContent className='flex flex-col gap-2'>
              <div className='font-mono text-xl font-bold tabular-nums'>
                {t('{{long}} long · {{short}} short', {
                  long: formatCompactNumber(
                    summary.context.long_requests,
                    locale
                  ),
                  short: formatCompactNumber(
                    summary.context.short_requests,
                    locale
                  ),
                })}
              </div>
              <p className='text-muted-foreground text-xs'>
                {t(
                  '{{count}} requests classified by current model thresholds',
                  {
                    count: formatCompactNumber(
                      summary.context.classified_requests,
                      locale
                    ),
                  }
                )}
              </p>
              <p className='text-xs'>
                {t('Long: {{long}} · Short: {{short}}', {
                  long: formatQuota(summary.context.long_gross_quota),
                  short: formatQuota(summary.context.short_gross_quota),
                })}
              </p>
            </CardContent>
          </Card>
        )}

        {summary.billing_mode && (
          <Card size='sm'>
            <CardHeader>
              <div className='flex items-center gap-2'>
                <IconBadge tone='warning' size='stat'>
                  <Zap />
                </IconBadge>
                <CardTitle>{t('Dynamic billing')}</CardTitle>
              </div>
            </CardHeader>
            <CardContent className='flex flex-col gap-2'>
              <div className='font-mono text-xl font-bold tabular-nums'>
                {formatCompactNumber(
                  summary.billing_mode.tiered_requests,
                  locale
                )}
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('Requests settled with tiered expressions')}
              </p>
              <p className='text-xs'>
                {t('Settled cost: {{amount}}', {
                  amount: formatQuota(summary.billing_mode.tiered_gross_quota),
                })}
              </p>
            </CardContent>
          </Card>
        )}
      </div>
    </section>
  )
}
