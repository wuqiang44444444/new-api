import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type { BillingDataQuality } from '../types'

export function BillingQualityNotice(props: {
  quality?: BillingDataQuality
  estimateAvailable: boolean
  compact?: boolean
}) {
  const details: Partial<BillingDataQuality> = props.quality ?? {}
  const usagePartial =
    !props.quality ||
    details.status === 'unavailable' ||
    [
      details.input_tokens_unavailable_requests,
      details.cache_write_unavailable_requests,
      details.unavailable_requests,
      details.unknown_billing_mode_requests,
    ].some((count) => (count ?? 0) > 0)
  const { t } = useTranslation()
  const reasons = [
    [
      details.input_tokens_unavailable_requests,
      t('Input token totals unavailable: {{count}} records', {
        count: details.input_tokens_unavailable_requests,
      }),
    ],
    [
      details.unknown_billing_mode_requests,
      t('Unknown billing mode: {{count}} records', {
        count: details.unknown_billing_mode_requests,
      }),
    ],
    [
      details.unavailable_requests,
      t('Usage metadata unreadable: {{count}} records', {
        count: details.unavailable_requests,
      }),
    ],
    [
      details.cache_write_unavailable_requests,
      t('Cache write usage unavailable: {{count}} records', {
        count: details.cache_write_unavailable_requests,
      }),
    ],
    [
      details.missing_historical_price_rows,
      t('List price and savings could not be estimated: {{count}} records', {
        count: details.missing_historical_price_rows,
      }),
    ],
    [
      details.provider_model_fallback_rows,
      t('Upstream model identity missing: {{count}} records', {
        count: details.provider_model_fallback_rows,
      }),
    ],
  ] as const
  return (
    <div
      role='note'
      className={
        props.compact
          ? 'space-y-1 text-xs'
          : 'bg-muted/30 rounded-lg border px-3 py-2 text-sm'
      }
    >
      <Badge variant={usagePartial ? 'warning' : 'secondary'}>
        {usagePartial
          ? t('Incomplete usage details')
          : t('Usage details available')}
      </Badge>
      <p>
        {props.estimateAvailable
          ? t('Original price and savings: estimated')
          : t('Original price and savings: unavailable')}
      </p>
      {!props.compact && (
        <p className='text-muted-foreground'>
          {t(
            'Amounts reflect recorded charges and refunds. Usage completeness does not certify wallet reconciliation.'
          )}
        </p>
      )}
      <ul className='mt-1 list-inside list-disc'>
        {reasons
          .filter(([count]) => (count ?? 0) > 0)
          .map(([, message]) => (
            <li key={message}>{message}</li>
          ))}
      </ul>
    </div>
  )
}
