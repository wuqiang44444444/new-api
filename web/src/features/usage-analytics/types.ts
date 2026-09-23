import type { BillingDiscountCombination } from '@/features/billing-reconciliation/types'

export interface UsageAnalyticsDay {
  date: string
  start: number
  end: number
  future?: boolean
}

export interface UsageAnalyticsPeriod {
  period: 'day' | 'week'
  date: string
  start_timestamp: number
  end_timestamp: number
  timezone: string
  days: UsageAnalyticsDay[]
}

export interface UsageAnalyticsMetrics {
  token_details?: Partial<
    Record<
      'image_input' | 'image_output' | 'audio_input' | 'audio_output',
      number
    >
  >
  total_calls: number
  success_calls: number
  failure_calls: number
  cancelled_calls: number
  other_result_calls: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  image_count: number
  seconds?: string
  seconds_missing_rows?: number
  gross_quota: number
  refund_quota: number
  net_quota: number
  unlinked_refund_quota?: number
  original_quota_estimate?: number | null
  estimate_reasons?: string[]
  multiple_discounts?: boolean
  reference_amount?: number | null
  reference_known?: boolean
  rows_missing_tokens?: number
  rows_missing_money?: number
  rows_money_pending?: number
  usage_only_rows?: number
  test_priced_rows?: number
  seconds_value_missing_rows?: number
}

export interface UsageCustomerModelRow {
  discount_combinations?: BillingDiscountCombination[]
  model_name: string
  days: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
}

export interface UsageCustomerKeyGroup {
  token_id: number
  token_name: string
  token_deleted?: boolean
  days: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
  models: UsageCustomerModelRow[]
}

export interface UsageCustomerView {
  day_totals: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
  keys: UsageCustomerKeyGroup[]
}

export interface UsageChannelMonthDiscount {
  period_start: number
  value: string
  version: number
  source: string
  source_period?: number
}

export interface UsageUpstreamModelRow {
  channels: UsageUpstreamChannelRow[]
  provider_model: string
  provider_model_fallback?: boolean
  billing_mode: string
  days: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
}

export interface UsageUpstreamChannelRow {
  channel_id: number
  channel_name: string
  discounts: UsageChannelMonthDiscount[]
  days: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
  usage_only?: boolean
}

export interface UsageUpstreamUrlGroup {
  url_key: string
  display_name: string
  base_url?: string
  unidentified?: boolean
  deleted?: boolean
  models: UsageUpstreamModelRow[]
  days: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
}

export interface UsageUpstreamView {
  day_totals: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
  url_groups: UsageUpstreamUrlGroup[]
}

export interface UsageCustomerOverviewRow {
  user_id: number
  username: string
  display_name?: string
  deleted?: boolean
  total: UsageAnalyticsMetrics
}

export interface UsageCustomersOverview {
  day_totals: UsageAnalyticsMetrics[]
  total: UsageAnalyticsMetrics
  customers: UsageCustomerOverviewRow[]
  truncated?: boolean
}

export interface UsageAnalyticsEnvelope<T> {
  period: UsageAnalyticsPeriod
  filters: Record<string, unknown>
  result: T
  generated_at: number
  data_source: string
  coverage_note: string
}

export interface UsageExportJobView {
  job_id: string
  status: string
  error_code?: string
  error?: string
}
