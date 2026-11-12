import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'

import type { BillingDataQuality } from '../types'

export function BillingQualityNotice(props: {
  quality?: BillingDataQuality
  estimateAvailable?: boolean
  recordedAmounts?: boolean
  compact?: boolean
  inline?: boolean
  discloseReasons?: boolean
  rowDetails?: ReactNode
}) {
  const details: Partial<BillingDataQuality> = props.quality ?? {}
  const usagePartial =
    !props.quality ||
    details.status === 'unavailable' ||
    [
      details.input_tokens_unavailable_requests,
      details.cache_write_unavailable_requests,
      details.unavailable_requests,
    ].some((count) => (count ?? 0) > 0)
  const unknownMode = (details.unknown_billing_mode_requests ?? 0) > 0
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
      t('Billing mode needs identification · {{count}} billing events', {
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
      props.recordedAmounts
        ? t('List price or savings unavailable for {{count}} records', {
            count: details.missing_historical_price_rows,
          })
        : t(
            'List price and savings could not be estimated: {{count}} records',
            {
              count: details.missing_historical_price_rows,
            }
          ),
    ],
    [
      details.provider_model_fallback_rows,
      t('Upstream model identity missing: {{count}} records', {
        count: details.provider_model_fallback_rows,
      }),
    ],
  ] as const
  let className = 'bg-muted/30 rounded-lg border px-3 py-2 text-sm'
  if (props.compact || props.inline) className = 'space-y-1 text-xs'

  const visibleReasons = reasons.filter(([count]) => (count ?? 0) > 0)

  let status = t('Usage details available')
  if (usagePartial) status = t('Incomplete usage details')
  else if (unknownMode) status = t('Billing mode needs identification')
  const variant = usagePartial || unknownMode ? 'warning' : 'secondary'
  const reasonList = visibleReasons.length > 0 && (
    <ul className='mt-1 list-inside list-disc'>
      {visibleReasons.map(([, message]) => (
        <li key={message}>{message}</li>
      ))}
    </ul>
  )

  if (
    props.discloseReasons &&
    (visibleReasons.length > 0 || props.rowDetails != null)
  ) {
    const countLabel =
      visibleReasons.length === 1
        ? t('{{count}} billing events', { count: visibleReasons[0][0] })
        : undefined
    return (
      <Popover>
        <PopoverTrigger
          render={
            <Button
              variant='ghost'
              size='sm'
              className='gap-1.5 px-0'
              aria-label={countLabel ? `${status} ${countLabel}` : undefined}
            />
          }
        >
          <Badge variant={variant}>{status}</Badge>
          {countLabel && (
            <span className='text-muted-foreground tabular-nums'>
              {countLabel}
            </span>
          )}
        </PopoverTrigger>
        <PopoverContent align='start' className='text-xs'>
          <PopoverTitle>{t('Data quality')}</PopoverTitle>
          {reasonList}
          {props.rowDetails}
        </PopoverContent>
      </Popover>
    )
  }

  return (
    <div role='note' className={className}>
      <div
        className={
          props.inline
            ? 'flex flex-wrap items-center gap-x-3 gap-y-1'
            : undefined
        }
      >
        <Badge variant={variant}>{status}</Badge>
        {!props.recordedAmounts && (
          <p>
            {props.estimateAvailable
              ? t('Original price and savings: estimated')
              : t('Original price and savings: unavailable')}
          </p>
        )}
        {!props.compact && (
          <p className='text-muted-foreground'>
            {t(
              'Amounts reflect recorded charges and refunds. Usage completeness does not certify wallet reconciliation.'
            )}
          </p>
        )}
      </div>
      {reasonList}
    </div>
  )
}
