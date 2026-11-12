/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'

import type { BillingDataQuality, ProviderUsage } from '../types'
import {
  cacheMeterUnavailableLabel,
  formatStatementUsage,
} from '../upstream-statement-utils'

type UpstreamUsageProps = {
  usage: ProviderUsage
  quality?: BillingDataQuality
  billingMode?: string
}

type UsageMetric = {
  label: string
  value: string
  knownSubtotal?: boolean
}

function useUpstreamUsageMetrics(
  props: UpstreamUsageProps,
  showImageSubtotal: boolean
) {
  const { t, i18n } = useTranslation()
  const usage = props.usage
  const format = (value: number) =>
    formatStatementUsage(value, i18n?.language || 'en')
  const tokenBilling = !props.billingMode || props.billingMode === 'token'
  const metrics: UsageMetric[] = []

  if (tokenBilling) {
    metrics.push(
      {
        label: t('Input tokens'),
        value: format(usage.input_tokens),
        knownSubtotal: !!props.quality?.input_tokens_unavailable_requests,
      },
      { label: t('Output tokens'), value: format(usage.output_tokens) }
    )
  }
  if (usage.completed_images != null) {
    metrics.push({
      label: t('Images'),
      value: format(usage.completed_images),
      knownSubtotal: showImageSubtotal,
    })
  }
  if (usage.billable_calls > 0 || props.billingMode === 'per_call') {
    metrics.push({
      label: t('Billable calls'),
      value: format(usage.billable_calls),
    })
  }
  if (
    usage.seconds != null ||
    usage.seconds_unavailable_rows ||
    props.billingMode === 'per_second'
  ) {
    metrics.push({
      label: t('Billable seconds'),
      value:
        usage.seconds == null
          ? t('Not recorded')
          : format(Number(usage.seconds)),
      knownSubtotal: usage.seconds != null && !!usage.seconds_unavailable_rows,
    })
  }

  return { metrics, tokenBilling, format }
}

export function UpstreamTotalMetric(props: UsageMetric) {
  const { t } = useTranslation()
  return (
    <div className='min-w-0'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='text-sm font-medium wrap-anywhere tabular-nums'>
        {props.value}
        {props.knownSubtotal && (
          <span className='text-muted-foreground block text-xs font-normal'>
            {t('Known subtotal')}
          </span>
        )}
      </dd>
    </div>
  )
}

export function UpstreamUsageTotalMetrics(props: UpstreamUsageProps) {
  const { metrics } = useUpstreamUsageMetrics(props, false)
  return metrics.map((metric) => (
    <UpstreamTotalMetric key={metric.label} {...metric} />
  ))
}

export function UpstreamUsageValues(
  props: UpstreamUsageProps & {
    showCache?: boolean
  }
) {
  const { t } = useTranslation()
  const { metrics, tokenBilling, format } = useUpstreamUsageMetrics(props, true)
  const usage = props.usage
  return (
    <div className='space-y-0.5 tabular-nums'>
      {metrics.map((metric) => (
        <div key={metric.label}>
          {metric.label}{' '}
          {metric.knownSubtotal ? (
            <>· {t('Known subtotal: {{amount}}', { amount: metric.value })}</>
          ) : (
            metric.value
          )}
        </div>
      ))}
      {props.showCache && tokenBilling && (
        <details>
          <summary className='text-muted-foreground focus-visible:ring-ring w-fit cursor-pointer rounded-sm focus-visible:ring-2'>
            {t('Cached tokens')}
          </summary>
          <div>
            {t('Cache read tokens')}{' '}
            {cacheMeterUnavailableLabel(props.quality, 'read', t) ??
              format(usage.cache_read_tokens)}
          </div>
          <div>
            {t('Cache write tokens')}{' '}
            {cacheMeterUnavailableLabel(props.quality, 'write', t) ??
              format(usage.cache_write_tokens)}
          </div>
        </details>
      )}
    </div>
  )
}
