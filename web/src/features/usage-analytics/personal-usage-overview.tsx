import { useTranslation } from 'react-i18next'

import { Card, CardHeader, CardTitle } from '@/components/ui/card'
import { Combobox } from '@/components/ui/combobox'
import { formatCustomerStatementQuota } from '@/features/billing-reconciliation/lib'
import { formatNumber } from '@/lib/format'

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
  const status = [
    `${t('OK')} ${formatNumber(m.success_calls)}`,
    `${t('Failed')} ${formatNumber(m.failure_calls)}`,
    `${t('Cancelled')} ${formatNumber(m.cancelled_calls)}`,
  ]
  if (m.other_result_calls > 0) {
    status.push(`${t('Other')} ${formatNumber(m.other_result_calls)}`)
  }
  let amount = formatCustomerStatementQuota(m.net_quota)
  if (m.net_quota === 0 && moneyMissing) amount = t('unrecorded')
  if (m.net_quota === 0 && moneyPending) amount = t('Pending')
  let amountDetail: string | undefined
  if (m.net_quota !== 0 && moneyMissing) amountDetail = t('Known subtotal')
  if (m.net_quota !== 0 && moneyPending) amountDetail = t('Pending')
  if (props.upstream) {
    const reference = m.reference_amount ?? m.known_reference_amount
    amount =
      reference == null && m.reference_pending
        ? t('Pending reconciliation')
        : formatCustomerStatementQuota(reference)
    amountDetail =
      m.known_reference_amount != null && m.reference_amount == null
        ? t('Known subtotal')
        : undefined
  }
  const cards = [
    {
      label: props.upstream ? t('Upstream calls') : t('Calls'),
      value: formatNumber(m.total_calls),
      detail: status.join(' · '),
    },
    {
      label: t('Total tokens'),
      value: tokenValue(tokens),
      detail: missingTokens && tokens > 0 ? t('Known subtotal') : undefined,
    },
    { label: t('Input tokens'), value: tokenValue(m.input_tokens) },
    { label: t('Output tokens'), value: tokenValue(m.output_tokens) },
    { label: t('Cache read'), value: tokenValue(m.cache_read_tokens) },
    { label: t('Cache write'), value: tokenValue(m.cache_write_tokens) },
    ...usageNonTokenValues(t, m).map((unit) => ({
      label: unit.label,
      value: unit.value,
      detail: unit.partial ? t('Known subtotal') : undefined,
    })),
    {
      label: props.upstream ? t('Reference amount') : t('Net amount'),
      value: amount,
      detail: amountDetail,
    },
  ]
  return (
    <div
      role='region'
      aria-label={t('Usage')}
      tabIndex={0}
      className='min-w-0 shrink-0 overflow-x-auto p-px pb-1'
    >
      <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-[repeat(auto-fit,minmax(7.5rem,1fr))]'>
        {cards.map((card) => (
          <Card key={card.label} size='sm' className='py-2.5'>
            <CardHeader className='gap-0.5'>
              <CardTitle className='text-muted-foreground text-xs'>
                <h2>{card.label}</h2>
              </CardTitle>
              <p className='text-lg font-semibold break-all tabular-nums'>
                {card.value}
              </p>
              {card.detail && (
                <p className='text-muted-foreground text-xs'>{card.detail}</p>
              )}
            </CardHeader>
          </Card>
        ))}
      </div>
    </div>
  )
}
