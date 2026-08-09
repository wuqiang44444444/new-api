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
export type BillingMode = 'token' | 'per_call' | 'unknown'
export type CustomerStatementListQuality = 'all' | 'complete' | 'partial'
export type CustomerStatementListSortBy =
  | 'net_quota'
  | 'requests'
  | 'original_quota'
  | 'username'
export type CustomerStatementListSortOrder = 'asc' | 'desc'

export type AdminBillingSearch = {
  section?: BillingSection
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
  status: 'complete' | 'partial' | 'unavailable'
  unavailable_requests?: number
  unknown_billing_mode_requests?: number
  provider_model_fallback_rows?: number
  missing_historical_price_rows?: number
}

export type CustomerModelSummary = {
  model_name: string
  billing_mode: BillingMode
  usage: BillingUsage
  original_quota?: number
  discount_ratio?: number
  multiple_discounts?: boolean
  price_versions: number
  data_quality?: BillingDataQuality
  detail_filter: BillingDetailFilter
}

type CustomerGroupSummary = {
  id: number
  name: string
  usage: BillingUsage
  original_quota?: number
  discount_quota?: number
  models: CustomerModelSummary[]
  deleted?: boolean
}

export type CustomerStatement = {
  user_id: number
  username: string
  display_name: string
  deleted?: boolean
  dimension: BillingDimension
  current_balance: number
  summary: BillingUsage
  original_quota?: number
  discount_quota?: number
  groups: CustomerGroupSummary[]
  data_quality?: BillingDataQuality
}

export type CustomerStatementListItem = {
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

type ProviderDiscount = {
  value: string
  version: number
  source: 'database' | 'previous_period' | 'default'
  source_period?: number
}

type ProviderUsage = {
  requests: number
  billable_calls: number
  input_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  output_tokens: number
}

export type ProviderModelSummary = {
  channel_id: number
  channel_name: string
  provider_model: string
  provider_model_fallback?: boolean
  billing_mode: BillingMode
  usage: ProviderUsage
  discount: ProviderDiscount
  data_quality?: BillingDataQuality
  detail_filter: BillingDetailFilter
}

export type ProviderChannelSummary = {
  channel_id: number
  channel_name: string
  usage: ProviderUsage
  models: ProviderModelSummary[]
  data_quality?: BillingDataQuality
}

export type ProviderSummary = {
  channels: ProviderChannelSummary[]
  data_quality?: BillingDataQuality
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
