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
import type { TFunction } from 'i18next'

import { formatLogQuota } from '@/lib/format'

import { escapeCsvCell, formatBillingDuration } from '../lib'
import type { BillingPeriod } from '../types'
import type { BillingStatementRow } from './statement-rows'

type BillingStatementCsvOptions = {
  rows: BillingStatementRow[]
  totalQuota: number
  locale?: string
  period: BillingPeriod
  t: TFunction
  keyLabel: (row: BillingStatementRow) => string
}

export function buildBillingStatementCsv(options: BillingStatementCsvOptions) {
  const percentFormatter = new Intl.NumberFormat(options.locale, {
    style: 'percent',
    maximumFractionDigits: 1,
  })
  const headers = [
    options.t('API Key'),
    options.t('Model'),
    options.t('Requests'),
    options.t('Input Tokens'),
    options.t('Output Tokens'),
    options.t('Total Tokens'),
    options.t('Cache Hit Requests'),
    options.t('Cache Write Requests'),
    options.t('Cache Hit Denominator'),
    options.t('Cache Hit Denominator Scope'),
    options.t('Request Cache Hit Rate'),
    'cache_hit_ratio_raw',
    options.t('Cache Read Tokens'),
    options.t('Cache Write Tokens'),
    options.t('Cache Write Tokens (5m)'),
    options.t('Cache Write Tokens (1h)'),
    options.t('Settled Cost of Cache-hit Requests'),
    'cache_hit_request_gross_quota_raw',
    options.t('Current Context Threshold'),
    options.t('Context Threshold Source'),
    options.t('Classified Requests'),
    options.t('Unclassified Requests'),
    options.t('Classification Coverage'),
    'classification_coverage_raw',
    options.t('Short Context Requests'),
    options.t('Short Context Input Tokens'),
    options.t('Short Context Request Cost'),
    'short_context_gross_quota_raw',
    options.t('Long Context Requests'),
    options.t('Long Context Input Tokens'),
    options.t('Long Context Request Cost'),
    'long_context_gross_quota_raw',
    options.t('Dynamic Billing Requests'),
    options.t('Dynamic Billing Request Cost'),
    'dynamic_billing_gross_quota_raw',
    options.t('Unallocated Async Adjustments'),
    'unallocated_adjustment_quota_raw',
    options.t('Gross Usage Cost'),
    'gross_quota_raw',
    options.t('Refunds / Adjustments'),
    'refund_quota_raw',
    options.t('Net Settled Cost'),
    'net_quota_raw',
    options.t('Average Response Time'),
    options.t('Streaming Requests'),
    options.t('Latest Request Time'),
    options.t('Usage Composition Status'),
    'statement_saturated',
    options.t('Cost Share'),
    'cost_share_raw',
  ]
  const lines = [
    headers.map(escapeCsvCell).join(','),
    ...options.rows.map((row) => {
      const costShare =
        options.totalQuota > 0 ? row.netQuota / options.totalQuota : 0
      let compositionStatus = options.t('Available')
      if (!row.breakdownComplete) {
        compositionStatus = options.t('Unavailable')
      } else if (row.unallocatedAdjustmentQuota > 0) {
        compositionStatus = options.t('Partial')
      }
      return [
        options.keyLabel(row),
        row.modelName || '-',
        row.requests,
        row.promptTokens,
        row.completionTokens,
        row.totalTokens,
        row.cache?.hit_requests ?? '',
        row.cache?.write_requests ?? '',
        row.cache?.denominator_requests ?? '',
        row.cache?.denominator_scope ?? '',
        row.cache ? percentFormatter.format(row.cache.hit_request_ratio) : '',
        row.cache?.hit_request_ratio ?? '',
        row.cache?.read_tokens ?? '',
        row.cache?.write_tokens ?? '',
        row.cache?.write_tokens_5m ?? '',
        row.cache?.write_tokens_1h ?? '',
        row.cache ? formatLogQuota(row.cache.hit_request_gross_quota) : '',
        row.cache?.hit_request_gross_quota ?? '',
        row.context?.threshold_tokens ?? '',
        row.context?.threshold_source ?? '',
        row.context?.classified_requests ?? '',
        row.context?.unclassified_requests ?? '',
        row.context
          ? percentFormatter.format(row.context.classification_coverage)
          : '',
        row.context?.classification_coverage ?? '',
        row.context?.short_requests ?? '',
        row.context?.short_input_tokens ?? '',
        row.context ? formatLogQuota(row.context.short_gross_quota) : '',
        row.context?.short_gross_quota ?? '',
        row.context?.long_requests ?? '',
        row.context?.long_input_tokens ?? '',
        row.context ? formatLogQuota(row.context.long_gross_quota) : '',
        row.context?.long_gross_quota ?? '',
        row.billingMode?.tiered_requests ?? '',
        row.billingMode
          ? formatLogQuota(row.billingMode.tiered_gross_quota)
          : '',
        row.billingMode?.tiered_gross_quota ?? '',
        row.unallocatedAdjustmentQuota > 0
          ? formatLogQuota(row.unallocatedAdjustmentQuota)
          : '',
        row.unallocatedAdjustmentQuota || '',
        formatLogQuota(row.grossQuota),
        row.grossQuota,
        formatLogQuota(row.refundQuota),
        row.refundQuota,
        formatLogQuota(row.netQuota),
        row.netQuota,
        formatBillingDuration(row.averageUseTimeSeconds, options.locale),
        row.streamRequests,
        row.latestRequestTimestamp > 0
          ? new Date(row.latestRequestTimestamp * 1000).toISOString()
          : '',
        compositionStatus,
        row.statementSaturated ? 'true' : 'false',
        percentFormatter.format(costShare),
        costShare,
      ]
        .map(escapeCsvCell)
        .join(',')
    }),
  ]
  return lines.join('\n')
}

export function downloadBillingStatementCsv(
  options: BillingStatementCsvOptions
) {
  const csv = buildBillingStatementCsv(options)
  const blob = new Blob([`\uFEFF${csv}`], {
    type: 'text/csv;charset=utf-8',
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `usage-statement-${options.period.start_timestamp}-${options.period.end_timestamp}.csv`
  document.body.append(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
