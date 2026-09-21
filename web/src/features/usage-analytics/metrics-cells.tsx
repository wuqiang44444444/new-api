import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'

import type { UsageAnalyticsMetrics } from './types'
import { formatNumber } from './utils'

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
  return (
    <div className='flex flex-col gap-0.5 text-sm'>
      <span>
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
      {m.image_count > 0 && (
        <span className='text-muted-foreground text-xs'>
          {t('Images')} {formatNumber(m.image_count)}
        </span>
      )}
      {m.seconds != null && (
        <span className='text-muted-foreground text-xs'>
          {t('Seconds')} {m.seconds}
        </span>
      )}
      {(m.seconds_missing_rows ?? 0) > 0 && (
        <span className='text-muted-foreground text-xs'>
          {t('Seconds unrecorded ({{count}} rows)', {
            count: (m.seconds_missing_rows ?? 0) - (m.seconds_value_missing_rows ?? 0),
          })}
        </span>
      )}
      {(m.seconds_value_missing_rows ?? 0) > 0 && (
        <span className='text-muted-foreground text-xs'>
          {t(
            'Second unit known; measured duration unrecorded ({{count}} rows)',
            { count: m.seconds_value_missing_rows }
          )}
        </span>
      )}
    </div>
  )
}

export function UsageMoneyCell(props: { metrics: UsageAnalyticsMetrics }) {
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
      <span className='text-muted-foreground text-xs'>
        {t('Gross')}{' '}
        {(m.rows_missing_money ?? 0) > 0
          ? t('unrecorded')
          : formatCustomerStatementQuota(m.gross_quota)}{' '}
        · {t('Refund')}{' '}
        {(m.rows_missing_money ?? 0) > 0
          ? t('unrecorded')
          : formatCustomerStatementQuota(m.refund_quota)}
      </span>
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
