import type { TFunction } from 'i18next'

import { billingModeLabel, formatCustomerStatementQuota } from './lib'
import type {
  BillingDataQuality,
  BillingEnvelope,
  CustomerStatement,
} from './types'
import { escapeCsvCell } from './upstream-statement-utils'

export function buildCustomerStatementCsv(
  envelope: BillingEnvelope<CustomerStatement>,
  t: TFunction
) {
  const statement = envelope.result
  const month = new Date(envelope.period.start_timestamp * 1000 + 8 * 3600_000)
    .toISOString()
    .slice(0, 7)
  const headers = [
    t('Billing month'),
    t('Timezone'),
    t('Row type'),
    t('API Key ID'),
    t('API Key'),
    t('Model'),
    t('Billing mode'),
    t('Requests'),
    t('Input tokens'),
    t('Cache read tokens'),
    t('Cache write tokens'),
    t('Output tokens'),
    t('Billable calls'),
    t('Refunded calls'),
    t('Estimated list price'),
    t('Estimated savings'),
    t('Group Ratio'),
    t('Contract discount'),
    t('Gross charges'),
    t('Refund amount'),
    t('Net amount'),
    'net_quota',
    t('Data quality'),
    t('Generated at'),
    t('Notes'),
  ]
  const rows: Array<Array<string | number>> = [headers]
  const base = [month, envelope.period.timezone]
  const notes = t(
    'List price and savings are estimates reconstructed from rounded charges and historical discounts. Net amounts include task holds and adjustments.'
  )
  const tail = [new Date(envelope.generated_at * 1000).toISOString(), notes]
  rows.push([
    ...base,
    t('Total'),
    '',
    '',
    '',
    '',
    statement.summary.requests,
    ...customerStatementCsvValues(
      statement.summary,
      statement.original_quota,
      statement.discount_quota,
      statement.data_quality,
      false
    ),
    '',
    '',
    ...customerStatementCsvAmounts(statement.summary),
    t(
      statement.data_quality?.status === 'partial' ? 'Partial data' : 'Complete'
    ),
    ...tail,
  ])
  for (const group of statement.groups) {
    rows.push([
      ...base,
      t('API Key'),
      group.id,
      group.name,
      '',
      '',
      group.usage.requests,
      ...customerStatementCsvValues(
        group.usage,
        group.original_quota,
        group.discount_quota,
        undefined,
        false
      ),
      '',
      '',
      ...customerStatementCsvAmounts(group.usage),
      '',
      ...tail,
    ])
    for (const model of group.models) {
      rows.push([
        ...base,
        t('Model'),
        group.id,
        group.name,
        model.model_name,
        t(billingModeLabel(model.billing_mode)),
        model.usage.requests,
        ...customerStatementCsvValues(
          model.usage,
          model.original_quota,
          undefined,
          model.data_quality,
          model.billing_mode === 'token',
          model.billing_mode === 'per_call'
        ),
        model.multiple_discounts
          ? t('Multiple versions')
          : (model.discount_ratio ?? ''),
        model.multiple_contract_discounts
          ? t('Multiple versions')
          : (model.contract_discount_ratio ?? ''),
        ...customerStatementCsvAmounts(model.usage),
        t(
          model.data_quality?.status === 'partial' ? 'Partial data' : 'Complete'
        ),
        ...tail,
      ])
    }
  }
  return rows.map((row) => row.map(escapeCsvCell).join(',')).join('\r\n')
}

function customerStatementCsvValues(
  usage: CustomerStatement['summary'],
  original: number | undefined,
  savings: number | undefined,
  quality: BillingDataQuality | undefined,
  token: boolean,
  perCall = false
): Array<string | number> {
  return [
    token && !quality?.input_tokens_unavailable_requests
      ? usage.input_tokens
      : '',
    token ? usage.cache_read_tokens : '',
    token && !quality?.cache_write_unavailable_requests
      ? usage.cache_write_tokens
      : '',
    token ? usage.output_tokens : '',
    perCall ? usage.billable_calls : '',
    perCall ? usage.refunded_calls : '',
    customerStatementCsvMoney(original),
    customerStatementCsvMoney(savings),
  ]
}

function customerStatementCsvAmounts(usage: CustomerStatement['summary']) {
  return [
    customerStatementCsvMoney(usage.gross_quota),
    customerStatementCsvMoney(usage.refund_quota),
    customerStatementCsvMoney(usage.net_quota),
    usage.net_quota,
  ]
}

function customerStatementCsvMoney(quota: number | undefined) {
  return quota == null ? '' : formatCustomerStatementQuota(quota)
}

export function downloadCustomerStatementCsv(
  envelope: BillingEnvelope<CustomerStatement>,
  t: TFunction
) {
  const csv = buildCustomerStatementCsv(envelope, t)
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  try {
    link.href = url
    const month = new Date(
      envelope.period.start_timestamp * 1000 + 8 * 3600_000
    )
      .toISOString()
      .slice(0, 7)
    link.download = `my-billing-${month}.csv`
    document.body.append(link)
    link.click()
  } finally {
    link.remove()
    URL.revokeObjectURL(url)
  }
}
