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
import type {
  BillingBreakdownItem,
  BillingCacheBreakdown,
  BillingContextBreakdown,
  BillingModeBreakdown,
  BillingStatementItem,
  BillingView,
} from '../types'

export type BillingStatementRow = {
  id: string
  tokenId?: number
  tokenName?: string
  modelName?: string
  requests: number
  promptTokens: number
  completionTokens: number
  totalTokens: number
  grossQuota: number
  refundQuota: number
  netQuota: number
  averageUseTimeSeconds: number
  streamRequests: number
  latestRequestTimestamp: number
  breakdownComplete: boolean
  unallocatedAdjustmentQuota: number
  statementSaturated: boolean
  cache?: BillingCacheBreakdown
  context?: BillingContextBreakdown
  billingMode?: BillingModeBreakdown
}

function detailKey(tokenId: number, modelName: string) {
  return `${tokenId}:${modelName}`
}

function rowKey(item: BillingStatementItem, view: BillingView) {
  if (view === 'model') return `model:${item.model_name}`
  if (view === 'key') return `key:${item.token_id}`
  return `detail:${detailKey(item.token_id, item.model_name)}`
}

function mergeCache(
  current: BillingCacheBreakdown | undefined,
  incoming: BillingCacheBreakdown
): BillingCacheBreakdown {
  const merged = current ?? {
    hit_requests: 0,
    write_requests: 0,
    denominator_requests: 0,
    denominator_scope: 'all_settled_requests',
    read_tokens: 0,
    write_tokens: 0,
    write_tokens_5m: 0,
    write_tokens_1h: 0,
    hit_request_gross_quota: 0,
    hit_request_ratio: 0,
  }
  merged.hit_requests += incoming.hit_requests
  merged.write_requests += incoming.write_requests
  merged.read_tokens += incoming.read_tokens
  merged.write_tokens += incoming.write_tokens
  merged.write_tokens_5m += incoming.write_tokens_5m
  merged.write_tokens_1h += incoming.write_tokens_1h
  merged.hit_request_gross_quota += incoming.hit_request_gross_quota
  return merged
}

function mergeContext(
  current: BillingContextBreakdown | undefined,
  incoming: BillingContextBreakdown
): BillingContextBreakdown {
  const merged = current ?? {
    threshold_tokens: incoming.threshold_tokens,
    threshold_source: 'current_model_config',
    classified_requests: 0,
    unclassified_requests: 0,
    short_requests: 0,
    long_requests: 0,
    short_input_tokens: 0,
    long_input_tokens: 0,
    short_gross_quota: 0,
    long_gross_quota: 0,
    classification_coverage: 0,
  }
  if (
    merged.threshold_tokens != null &&
    incoming.threshold_tokens !== merged.threshold_tokens
  ) {
    merged.threshold_tokens = undefined
  }
  merged.classified_requests += incoming.classified_requests
  merged.unclassified_requests += incoming.unclassified_requests
  merged.short_requests += incoming.short_requests
  merged.long_requests += incoming.long_requests
  merged.short_input_tokens += incoming.short_input_tokens
  merged.long_input_tokens += incoming.long_input_tokens
  merged.short_gross_quota += incoming.short_gross_quota
  merged.long_gross_quota += incoming.long_gross_quota
  return merged
}

function mergeBillingMode(
  current: BillingModeBreakdown | undefined,
  incoming: BillingModeBreakdown
): BillingModeBreakdown {
  const merged = current ?? {
    tiered_requests: 0,
    tiered_gross_quota: 0,
  }
  merged.tiered_requests += incoming.tiered_requests
  merged.tiered_gross_quota += incoming.tiered_gross_quota
  return merged
}

function finalizeRow(row: BillingStatementRow) {
  if (!row.breakdownComplete) {
    row.unallocatedAdjustmentQuota = 0
    row.cache = undefined
    row.context = undefined
    row.billingMode = undefined
    return
  }
  if (row.cache) {
    row.cache.denominator_requests = row.requests
    row.cache.denominator_scope = 'all_settled_requests'
    row.cache.hit_request_ratio =
      row.requests > 0 ? row.cache.hit_requests / row.requests : 0
  }
  if (row.context) {
    const contextRequests =
      row.context.classified_requests + row.context.unclassified_requests
    row.context.classification_coverage =
      contextRequests > 0
        ? row.context.classified_requests / contextRequests
        : 0
  }
}

export function buildBillingStatementRows(
  items: BillingStatementItem[],
  breakdownItems: BillingBreakdownItem[],
  view: BillingView
): BillingStatementRow[] {
  const breakdownByDetail = new Map(
    breakdownItems.map((item) => [
      detailKey(item.token_id, item.model_name),
      item,
    ])
  )
  const rows = new Map<string, BillingStatementRow>()

  for (const item of items) {
    const id = rowKey(item, view)
    let row = rows.get(id)
    if (!row) {
      row = {
        id,
        tokenId: view === 'model' ? undefined : item.token_id,
        tokenName: view === 'model' ? undefined : item.token_name,
        modelName: view === 'key' ? undefined : item.model_name,
        requests: 0,
        promptTokens: 0,
        completionTokens: 0,
        totalTokens: 0,
        grossQuota: 0,
        refundQuota: 0,
        netQuota: 0,
        averageUseTimeSeconds: 0,
        streamRequests: 0,
        latestRequestTimestamp: 0,
        breakdownComplete: true,
        unallocatedAdjustmentQuota: 0,
        statementSaturated: false,
      }
      rows.set(id, row)
    }

    const totalUseTime =
      row.averageUseTimeSeconds * row.requests +
      item.average_use_time_seconds * item.requests
    row.requests += item.requests
    row.promptTokens += item.prompt_tokens
    row.completionTokens += item.completion_tokens
    row.totalTokens += item.total_tokens
    row.grossQuota += item.gross_quota
    row.refundQuota += item.refund_quota
    row.netQuota += item.net_quota
    row.streamRequests += item.stream_requests
    row.latestRequestTimestamp = Math.max(
      row.latestRequestTimestamp,
      item.latest_request_timestamp
    )
    row.statementSaturated ||= item.data_quality?.saturated === true
    if (row.requests > 0) {
      row.averageUseTimeSeconds = totalUseTime / row.requests
    }

    const breakdown = breakdownByDetail.get(
      detailKey(item.token_id, item.model_name)
    )
    const expectsBreakdown = item.requests > 0 || item.gross_quota > 0
    if (
      expectsBreakdown &&
      (breakdown?.requests !== item.requests ||
        breakdown?.gross_quota !== item.gross_quota ||
        (breakdown.data_quality?.unavailable_requests ?? 0) > 0)
    ) {
      row.breakdownComplete = false
    }
    row.unallocatedAdjustmentQuota +=
      breakdown?.unallocated_adjustment_quota ?? 0
    if (breakdown?.cache) {
      row.cache = mergeCache(row.cache, breakdown.cache)
    }
    if (breakdown?.context) {
      row.context = mergeContext(row.context, breakdown.context)
    }
    if (breakdown?.billing_mode) {
      row.billingMode = mergeBillingMode(
        row.billingMode,
        breakdown.billing_mode
      )
    }
  }

  const result = [...rows.values()]
  for (const row of result) finalizeRow(row)
  return result.sort((left, right) => {
    if (left.netQuota !== right.netQuota) return right.netQuota - left.netQuota
    return right.totalTokens - left.totalTokens
  })
}

export function countOrphanBillingBreakdownItems(
  items: BillingStatementItem[],
  breakdownItems: BillingBreakdownItem[]
) {
  const statementKeys = new Set(
    items.map((item) => detailKey(item.token_id, item.model_name))
  )
  return breakdownItems.filter(
    (item) => !statementKeys.has(detailKey(item.token_id, item.model_name))
  ).length
}

export function selectReliableBillingBreakdownItems(
  items: BillingStatementItem[],
  breakdownItems: BillingBreakdownItem[]
) {
  const statementTotals = new Map(
    items.map((item) => [
      detailKey(item.token_id, item.model_name),
      { requests: item.requests, grossQuota: item.gross_quota },
    ])
  )
  return breakdownItems.filter((item) => {
    const statement = statementTotals.get(
      detailKey(item.token_id, item.model_name)
    )
    return (
      statement?.requests === item.requests &&
      statement.grossQuota === item.gross_quota &&
      (item.data_quality?.unavailable_requests ?? 0) === 0
    )
  })
}

export function countBillingBreakdownMismatches(
  items: BillingStatementItem[],
  breakdownItems: BillingBreakdownItem[]
) {
  const breakdownByDetail = new Map(
    breakdownItems.map((item) => [
      detailKey(item.token_id, item.model_name),
      item,
    ])
  )
  const missingOrChanged = items.filter((item) => {
    if (item.requests <= 0 && item.gross_quota <= 0) return false
    const breakdown = breakdownByDetail.get(
      detailKey(item.token_id, item.model_name)
    )
    return (
      breakdown?.requests !== item.requests ||
      breakdown?.gross_quota !== item.gross_quota ||
      (breakdown.data_quality?.unavailable_requests ?? 0) > 0
    )
  }).length
  return (
    missingOrChanged + countOrphanBillingBreakdownItems(items, breakdownItems)
  )
}
