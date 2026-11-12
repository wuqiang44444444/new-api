import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import type { UsageAnalyticsMetrics, UsageNonTokenMetrics } from './types'
import { formatNumber, usageNonTokenValues } from './utils'

// Renders call counts with result breakdown. Failures and cancellations are
// never filtered out, and missing usage is shown as "unrecorded" instead of 0.
export function UsageCallsCell(props: { metrics: UsageAnalyticsMetrics }) {
  const { t } = useTranslation()
  const m = props.metrics
  return (
    <div className='flex flex-col gap-0.5 text-sm'>
      <span className='font-medium'>{formatNumber(m.total_calls)}</span>
      <span className='text-muted-foreground text-xs'>
        {t('OK')} {formatNumber(m.success_calls)} · {t('Failed')}{' '}
        {formatNumber(m.failure_calls)} · {t('Cancelled')}{' '}
        {formatNumber(m.cancelled_calls)}
        {m.other_result_calls > 0 && (
          <>
            {' '}
            · {t('Other')} {formatNumber(m.other_result_calls)}
          </>
        )}
      </span>
    </div>
  )
}

export function UsageTokensCell(props: { metrics: UsageAnalyticsMetrics }) {
  const { t } = useTranslation()
  const m = props.metrics
  const missing = m.rows_missing_tokens ?? 0
  const total = m.input_tokens + m.output_tokens
  return (
    <div className='flex flex-col gap-0.5 text-sm tabular-nums'>
      <span className='font-medium'>
        {missing > 0 && total === 0 ? t('unrecorded') : formatNumber(total)}
        {missing > 0 && total > 0 && (
          <span className='text-muted-foreground ml-1 text-xs'>
            {t('Known subtotal')}
          </span>
        )}
      </span>
      <span className='text-muted-foreground text-xs'>
        {t('Input')}{' '}
        {missing > 0 && m.input_tokens === 0
          ? t('unrecorded')
          : formatNumber(m.input_tokens)}{' '}
        · {t('Output')}{' '}
        {missing > 0 && m.output_tokens === 0
          ? t('unrecorded')
          : formatNumber(m.output_tokens)}
      </span>
      <span className='text-muted-foreground text-xs'>
        {t('Cache read')} {formatNumber(m.cache_read_tokens)} ·{' '}
        {t('Cache write')} {formatNumber(m.cache_write_tokens)}
        {missing > 0 && (
          <> · {t('{{count}} rows unrecorded', { count: missing })}</>
        )}
      </span>
      {m.token_details && (
        <span className='text-muted-foreground text-xs'>
          {m.token_details.image_input != null && (
            <>
              {t('Image input tokens')}{' '}
              {formatNumber(m.token_details.image_input)} ·{' '}
            </>
          )}
          {m.token_details.image_output != null && (
            <>
              {t('Image output tokens')}{' '}
              {formatNumber(m.token_details.image_output)} ·{' '}
            </>
          )}
          {m.token_details.audio_input != null && (
            <>
              {t('Audio input tokens')}{' '}
              {formatNumber(m.token_details.audio_input)} ·{' '}
            </>
          )}
          {m.token_details.audio_output != null && (
            <>
              {t('Audio output tokens')}{' '}
              {formatNumber(m.token_details.audio_output)}
            </>
          )}
        </span>
      )}
    </div>
  )
}

export function UsageNonTokenCell(props: { metrics?: UsageNonTokenMetrics }) {
  const { t } = useTranslation()
  const values = usageNonTokenValues(t, props.metrics)
  if (!values.length) {
    return (
      <span className='text-muted-foreground'>
        <span aria-hidden='true'>—</span>
        <span className='sr-only'>{t('Not applicable')}</span>
      </span>
    )
  }
  return (
    <div className='flex flex-col gap-1 whitespace-normal tabular-nums'>
      {values.map((unit) => (
        <div key={unit.label}>
          {unit.text && <span>{unit.text}</span>}
          {unit.partial && (
            <span className='text-muted-foreground block text-xs'>
              {t('Known subtotal')}
            </span>
          )}
          {unit.notes?.map((note) => (
            <span key={note} className='text-muted-foreground block text-xs'>
              {note}
            </span>
          ))}
        </div>
      ))}
    </div>
  )
}

export function UsageReferenceAmount({
  metrics,
}: {
  metrics: UsageAnalyticsMetrics
}) {
  const { t } = useTranslation()
  const amount = metrics.reference_amount ?? metrics.known_reference_amount
  return (
    <>
      <span className='tabular-nums'>
        {amount == null && metrics.reference_pending
          ? t('Pending reconciliation')
          : formatCustomerStatementQuota(amount)}
      </span>
      {metrics.known_reference_amount != null &&
        metrics.reference_amount == null && (
          <span className='text-muted-foreground block text-xs'>
            {t('Known subtotal')}
          </span>
        )}
    </>
  )
}

export function UsageMoneyCell(props: {
  metrics: UsageAnalyticsMetrics
  describeFees?: boolean
}) {
  const { t } = useTranslation()
  const m = props.metrics
  return (
    <div className='flex flex-col gap-0.5 text-sm'>
      <span>
        {t('Net')}{' '}
        {(m.rows_money_pending ?? 0) > 0 && m.net_quota === 0
          ? t('Pending')
          : formatCustomerStatementQuota(m.net_quota)}
      </span>
      <details className='text-muted-foreground text-xs'>
        <summary className='cursor-pointer'>
          {props.describeFees ? t('View fee details') : t('View details')}
        </summary>
        <span>
          {t('Gross')}{' '}
          {(m.rows_missing_money ?? 0) > 0
            ? t('unrecorded')
            : formatCustomerStatementQuota(m.gross_quota)}{' '}
          · {t('Refund')}{' '}
          {(m.rows_missing_money ?? 0) > 0
            ? t('unrecorded')
            : formatCustomerStatementQuota(m.refund_quota)}
        </span>
      </details>
      {(m.rows_money_pending ?? 0) > 0 && (
        <Badge variant='secondary' className='w-fit'>
          {t('Money pending ({{count}} rows)', { count: m.rows_money_pending })}
        </Badge>
      )}
      {(m.rows_missing_money ?? 0) > 0 && (
        <Badge variant='secondary' className='w-fit'>
          {t('Money unrecorded ({{count}} rows)', {
            count: m.rows_missing_money,
          })}
        </Badge>
      )}
    </div>
  )
}

export function UsageQualityCell(props: { metrics: UsageAnalyticsMetrics }) {
  const { t } = useTranslation()
  const m = props.metrics
  const reasons: string[] = []
  if ((m.unlinked_refund_quota ?? 0) !== 0) {
    reasons.push(
      t('Unlinked refund {{amount}}', { amount: m.unlinked_refund_quota })
    )
  }
  for (const reason of m.estimate_reasons ?? []) {
    reasons.push(reason)
  }
  if (reasons.length === 0) {
    return <span className='text-muted-foreground text-xs'>—</span>
  }
  return (
    <div className='text-muted-foreground flex flex-col gap-0.5 text-xs'>
      {reasons.map((reason) => (
        <span key={reason}>{reason}</span>
      ))}
    </div>
  )
}
