import { useTranslation } from 'react-i18next'

import {
  ReportCard,
  ReportMetric,
  ReportMetricCard,
} from '@/components/report-card'
import { Badge } from '@/components/ui/badge'
import { Combobox } from '@/components/ui/combobox'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { UsageAnalyticsMetrics, UsageCustomerKeyGroup } from './types'
import { usageKeyLabel, usageNonTokenValues } from './utils'

export function PersonalUsageKeyFilter(props: {
  keys: UsageCustomerKeyGroup[]
  selectedKey?: UsageCustomerKeyGroup
  onChange: (tokenId: number | undefined) => void
}) {
  const { t } = useTranslation()
  return (
    <Combobox
      aria-label={t('API key')}
      className='w-48 max-w-full'
      value={props.selectedKey ? String(props.selectedKey.token_id) : ''}
      options={[
        { value: '', label: t('All API Keys') },
        ...props.keys.map((key) => ({
          value: String(key.token_id),
          label: usageKeyLabel(t, key),
          description: key.token_id
            ? `${t('API Key ID')} ${key.token_id}`
            : undefined,
        })),
      ]}
      onValueChange={(value) =>
        props.onChange(value ? Number(value) : undefined)
      }
    />
  )
}

export function PersonalUsageOverview(props: {
  metrics: UsageAnalyticsMetrics
  upstream?: boolean
}) {
  const { t } = useTranslation()
  const m = props.metrics
  const missingTokens = (m.rows_missing_tokens ?? 0) > 0
  const moneyPending = (m.rows_money_pending ?? 0) > 0
  const moneyMissing = (m.rows_missing_money ?? 0) > 0
  const tokenValue = (value: number) =>
    missingTokens && value === 0 ? t('unrecorded') : formatNumber(value)
  const tokens = m.input_tokens + m.output_tokens
  const tokenDetails = [
    { label: t('Input tokens'), value: m.input_tokens },
    { label: t('Output tokens'), value: m.output_tokens },
    { label: t('Cache read'), value: m.cache_read_tokens },
    { label: t('Cache write'), value: m.cache_write_tokens },
  ]
  const tokenApplicable =
    !props.upstream ||
    m.billing_tokens_applicable ||
    tokens > 0 ||
    missingTokens
  const nonToken = usageNonTokenValues(t, m, props.upstream)
  let amount = formatCustomerStatementQuota(m.net_quota)
  if (m.net_quota === 0 && moneyMissing) amount = t('unrecorded')
  if (m.net_quota === 0 && moneyPending) amount = t('Pending')
  const amountDetail =
    !props.upstream && m.net_quota !== 0 && moneyPending
      ? t('Pending')
      : undefined
  if (props.upstream) {
    const reference = m.reference_amount ?? m.known_reference_amount
    amount =
      reference == null && m.reference_pending
        ? t('Pending reconciliation')
        : formatCustomerStatementQuota(reference)
  }
  return (
    <div
      role='region'
      aria-label={t('Usage')}
      className='@container flex min-w-0 shrink-0 flex-col gap-2 p-px pb-1'
    >
      <div
        className={cn(
          'grid gap-2 @min-[56rem]:grid-cols-[minmax(0,1.7fr)_minmax(0,3fr)_minmax(0,1fr)]',
          nonToken.length === 1 &&
            '@min-[76rem]:grid-cols-[minmax(0,1.8fr)_minmax(0,2.8fr)_minmax(0,0.9fr)_minmax(6.75rem,0.55fr)]',
          nonToken.length === 2 &&
            '@min-[76rem]:grid-cols-[minmax(0,1.82fr)_minmax(0,2.8fr)_minmax(0,0.85fr)_minmax(12rem,1fr)]'
        )}
      >
        <ReportMetricCard
          label={<h2>{props.upstream ? t('Upstream calls') : t('Calls')}</h2>}
          value={formatNumber(m.total_calls)}
        >
          <div className='flex flex-wrap gap-1 tabular-nums'>
            <Badge variant='secondary'>
              {t('OK')} {formatNumber(m.success_calls)}
            </Badge>
            <Badge
              variant='outline'
              className='border-destructive/30 text-destructive'
            >
              {t('Failed')} {formatNumber(m.failure_calls)}
            </Badge>
            <Badge variant='secondary'>
              {t('Cancelled')} {formatNumber(m.cancelled_calls)}
            </Badge>
            {m.other_result_calls > 0 && (
              <Badge variant='secondary'>
                {t('Other')} {formatNumber(m.other_result_calls)}
              </Badge>
            )}
          </div>
        </ReportMetricCard>
        <ReportMetricCard
          stacked
          label={
            <h2>{props.upstream ? t('Billable tokens') : t('Total tokens')}</h2>
          }
          value={tokenApplicable ? tokenValue(tokens) : t('Not applicable')}
          mutedValue={!tokenApplicable || (missingTokens && tokens === 0)}
        >
          {tokenApplicable && (
            <div className='flex flex-wrap gap-x-2 gap-y-1'>
              {tokenDetails.map((metric) => (
                <div
                  key={metric.label}
                  className='max-w-full min-w-0 flex-auto'
                >
                  <h3 className='text-muted-foreground text-xs'>
                    {metric.label}
                  </h3>
                  <p
                    className={cn(
                      'overflow-x-auto text-xs font-medium whitespace-nowrap tabular-nums',
                      missingTokens &&
                        metric.value === 0 &&
                        'text-muted-foreground'
                    )}
                  >
                    {tokenValue(metric.value)}
                  </p>
                </div>
              ))}
            </div>
          )}
        </ReportMetricCard>
        <ReportMetricCard
          emphasized
          label={
            <h2>{props.upstream ? t('Reference amount') : t('Net amount')}</h2>
          }
          value={amount}
          caption={amountDetail}
        />
        {nonToken.length > 0 && (
          <ReportCard
            className={cn(
              'grid items-center py-1.5 @min-[56rem]:col-span-full',
              nonToken.length <= 2
                ? 'auto-cols-fr grid-flow-col @min-[76rem]:col-span-1'
                : 'grid-cols-[repeat(auto-fit,minmax(9rem,1fr))] gap-y-2'
            )}
          >
            {nonToken.map((unit, index) => (
              <div
                key={unit.label}
                className={cn('min-w-0', index > 0 && 'border-s ps-2')}
              >
                <ReportMetric
                  stacked
                  label={<h2>{unit.label}</h2>}
                  value={unit.value}
                  caption={unit.showSubtotal ? t('Known subtotal') : undefined}
                />
              </div>
            ))}
          </ReportCard>
        )}
      </div>
    </div>
  )
}
