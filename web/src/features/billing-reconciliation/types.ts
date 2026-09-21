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
export type BillingSection = 'customer' | 'upstream'
export type BillingDimension = 'api_key' | 'channel'
export type BillingMode = 'token' | 'per_call' | 'per_second' | 'unknown'
export type CustomerStatementListQuality = 'all' | 'complete' | 'partial'
export type CustomerStatementListSortBy =
  | 'net_quota'
  | 'requests'
  | 'original_quota'
  | 'username'
export type CustomerStatementListSortOrder = 'asc' | 'desc'

export type AdminBillingSearch = {
  // section 兼容旧地址里的 upstream_url；页面组件负责把它并入 upstream。
  section?: BillingSection | 'upstream_url'
  month?: string
  userId?: number
  dimension?: BillingDimension
  customerSearch?: string
  customerQuality?: CustomerStatementListQuality
  customerSortBy?: CustomerStatementListSortBy
  customerSortOrder?: CustomerStatementListSortOrder
  customerPage?: number
  customerPageSize?: 10 | 20 | 50 | 100
}

type BillingPeriod = {
  start_timestamp: number
  end_timestamp: number
  period_start: number
  timezone: 'Asia/Shanghai'
}

type BillingUsage = {
  requests: number
  billable_calls: number
  refunded_calls: number
  input_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  output_tokens: number
  gross_quota: number
  refund_quota: number
  net_quota: number
}

type BillingDetailFilter = {
  start_timestamp: number
  end_timestamp: number
  user_id?: number
  token_id?: number
  channel_id?: number
  model_name?: string
  billing_mode?: BillingMode
}

export type BillingDataQuality = {
  evidence_coverage?: {
    rows: number
    gap_rows: number
    amount_gap_rows?: number
    usage_gap_rows?: number
    other_gap_rows?: number
  }
  legacy_test_cache_read_rows?: number
  legacy_test_cache_write_rows?: number
  cache_read_unreported_requests?: number
  cache_write_unreported_requests?: number
  cache_read_unavailable_requests?: number
  seconds_unavailable_rows?: number
  usage_without_amount_rows?: number
  test_amount_pending_reasons?: Record<string, number>
  test_recomputed_rows?: number
  test_recorded_original_rows?: number
  status: 'complete' | 'partial' | 'unavailable'
  input_tokens_unavailable_requests?: number
  cache_write_unavailable_requests?: number
  unavailable_requests?: number
  unknown_billing_mode_requests?: number
  provider_model_fallback_rows?: number
  missing_historical_price_rows?: number
  auxiliary_charge_rows?: number
  test_priced_rows?: number
  recovered_billing_seconds_rows?: number
  refunded_task_hold_rows?: number
  seconds_task_link_missing_rows?: number
  seconds_value_missing_rows?: number
}

// 优惠构成组合行（方案 §4）：后端按冻结折扣事实聚合的同一批结算日志投影。
export type BillingDiscountCombination = {
  estimate_reasons?: string[]
  group_name?: string
  group_ratio_source?: '' | 'group' | 'user_exclusive'
  group_id: number
  model_name: string
  billing_mode: string
  group_ratio?: number
  contract_applicable: 'yes' | 'no' | 'unrecorded' | 'unknown'
  contract_name?: string
  contract_id_known?: boolean
  contract_id?: number
  contract_version?: number
  contract_ratio?: number
  usage: {
    requests: number
    billable_calls: number
    refunded_calls: number
    input_tokens: number
    cache_read_tokens: number
    cache_write_tokens: number
    output_tokens: number
    gross_quota: number
    refund_quota: number
    net_quota: number
  }
  original_known: boolean
  original_quota?: number
  discount_quota?: number
  other?: boolean
}

export type CustomerModelSummary = {
  discount_quota?: number
  estimate_reasons?: string[]
  model_name: string
  billing_mode: BillingMode
  usage: BillingUsage
  original_quota?: number
  discount_ratio?: number
  multiple_discounts?: boolean
  contract_discount_ratio?: number
  multiple_contract_discounts?: boolean
  price_versions: number
  data_quality?: BillingDataQuality
  detail_filter: BillingDetailFilter
}

type CustomerGroupSummary = {
  estimate_reasons?: string[]
  id: number
  name: string
  usage: BillingUsage
  original_quota?: number
  discount_quota?: number
  models: CustomerModelSummary[]
  deleted?: boolean
}

export type CustomerStatement = {
  estimate_reasons?: string[]
  user_id: number
  username: string
  display_name: string
  deleted?: boolean
  dimension: BillingDimension
  current_balance: number | null
  summary: BillingUsage
  original_quota?: number
  discount_quota?: number
  groups: CustomerGroupSummary[]
  discount_combinations?: BillingDiscountCombination[]
  data_quality?: BillingDataQuality
}

export type CustomerStatementListItem = {
  billing_version?: { version_number: number; confirmed_at: number }
  money_usd?: StatementListMoney
  user_id: number
  username: string
  display_name: string
  deleted?: boolean
  usage: BillingUsage
  original_quota?: number
  discount_quota?: number
  data_quality?: BillingDataQuality
  last_activity_at: number
}

type CustomerStatementListSummary = {
  money_usd?: StatementListMoney
  customer_count: number
  usage: BillingUsage
  original_quota?: number
  discount_quota?: number
  data_quality?: BillingDataQuality
}

export type CustomerStatementList = {
  summary: CustomerStatementListSummary
  items: CustomerStatementListItem[]
  page: number
  page_size: number
  total: number
  sort_by: CustomerStatementListSortBy
  sort_order: CustomerStatementListSortOrder
}

export type ProviderDiscount = {
  value: string
  version: number
  source: 'database' | 'previous_period' | 'migrated' | 'default'
  source_period?: number
}

type ProviderUsage = {
  seconds?: string | null
  seconds_unavailable_rows?: number
  requests: number
  billable_calls: number
  input_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  output_tokens: number
}

// 渠道行直接展开模型：渠道父行携带当月综合系数；discount 为 null 表示
// 当月待确认（迁移冲突），绝不表示系数 1。
export type ProviderUrlChannelGroupSummary = {
  known_original_amount?: number
  known_reference_amount?: number
  channel_id: number
  channel_name: string
  discount: ProviderDiscount | null
  updated_at?: number
  updated_by?: number
  usage: ProviderUsage
  models: ProviderUrlChannelModelSummary[]
  data_quality?: BillingDataQuality
  original_amount?: number
  reference_amount?: number
  estimate_reasons?: string[]
  usage_only?: boolean
}

// 渠道 × 上游模型 × 计费方式的叶子行；fallback 身份与计费方式不混行。
export type ProviderUrlChannelModelSummary = {
  known_original_amount?: number
  known_reference_amount?: number
  usage_only?: boolean
  provider_model: string
  provider_model_fallback?: boolean
  billing_mode: BillingMode
  customer_models: string[]
  usage: ProviderUsage
  data_quality?: BillingDataQuality
  detail_filter: BillingDetailFilter
  original_amount?: number
  reference_amount?: number
  estimate_reasons?: string[]
}

export type ProviderUrlGroupSummary = {
  usage_only?: boolean
  known_original_amount?: number
  known_reference_amount?: number
  url_key: string
  display_name: string
  custom_name?: string
  base_url?: string
  unidentified?: boolean
  deleted?: boolean
  channel_ids: number[]
  channel_count: number
  model_count: number
  usage: ProviderUsage
  channels: ProviderUrlChannelGroupSummary[]
  data_quality?: BillingDataQuality
  original_amount?: number
  reference_amount?: number
  reference_known: boolean
  discount_pending_channels: number
  estimate_reasons?: string[]
}

export type ProviderUrlSummary = {
  page: number
  page_size: number
  total: number
  model_count: number
  channels: ProviderUrlChannelGroupSummary[]
  models: ProviderUrlChannelModelSummary[]
  url_groups: ProviderUrlGroupSummary[]
  data_quality?: BillingDataQuality
}

export type UpstreamDetailEvent =
  | 'refund'
  | 'call'
  | 'task_create'
  | 'task_adjustment'
  | 'task_call'
  | 'task_refunded_hold'
  | 'channel_test'

export type UpstreamDetailItem = {
  seconds_source?: 'recorded_usage' | 'billing_parameters' | 'refunded_hold'
  seconds?: string | null
  row_id: number
  time: number
  channel_id: number
  channel_name: string
  customer_model: string
  provider_model: string
  provider_model_fallback?: boolean
  billing_mode: BillingMode
  recorded_input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  original_amount?: number
  estimate_reasons?: string[]
  request_id: string
  upstream_request_id: string
  platform_task_id?: string
  upstream_task_id?: string
  test_pricing?: {
    mode: string
    status: string
    reason?: string
    basis?: string
    replayed?: boolean
    recorded_quota?: number
    recomputed_quota?: number
  }
  event: UpstreamDetailEvent
  data_quality?: BillingDataQuality
}

export type UpstreamDetails = {
  items: UpstreamDetailItem[]
  total: number
  page: number
  page_size: number
}

export type BillingEnvelope<T> = {
  period: BillingPeriod
  filters: Record<string, string | number>
  result: T
  generated_at: number
  data_version: number
  data_source: string
}

export type ApiResponse<T> = {
  success: boolean
  message: string
  data?: T
}

export type StatementListMoney = {
  gross: string
  refund: string
  net: string
  original: string | null
  discount: string | null
}
