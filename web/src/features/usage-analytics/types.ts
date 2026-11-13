import type { BillingMode } from '@/features/billing-reconciliation/types'

export interface UsageAnalyticsDay {
  date: string
  start: number
  end: number
  future?: boolean
  unpublished?: boolean
}

export type UsagePeriodKind = 'day' | 'week' | 'month'

export interface UsageAnalyticsPeriod {
  period: UsagePeriodKind
  date: string
  start_timestamp: number
  end_timestamp: number
  timezone: string
  days: UsageAnalyticsDay[]
}

export interface BillingImageTotals {
  billable_images?: number
  billable_image_missing_rows?: number
  test_billable_images?: number
  test_billable_image_missing_rows?: number
}

export interface UsageNonTokenMetrics extends BillingImageTotals {
  billing_tokens_applicable?: boolean
  billing_meter_unknown_rows?: number
  image_count?: number
  image_count_known?: boolean
  image_count_missing_rows?: number
  seconds?: string
  seconds_missing_rows?: number
  seconds_value_missing_rows?: number
}

export interface UsageTokenMetrics {
  token_details?: Partial<
    Record<
      'image_input' | 'image_output' | 'audio_input' | 'audio_output',
      number
    >
  >
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  rows_missing_tokens?: number
}

export interface UsageAnalyticsMetrics
  extends UsageNonTokenMetrics, UsageTokenMetrics {
  total_calls: number
  success_calls: number
  failure_calls: number
  cancelled_calls: number
  other_result_calls: number
  image_count: number
  gross_quota: number
  refund_quota: number
  net_quota: number
  unlinked_refund_quota?: number
  original_quota_estimate?: number | null
  estimate_reasons?: string[]
  multiple_discounts?: boolean
  reference_amount?: number | null
  reference_known?: boolean
  known_reference_amount?: number | null
  reference_pending?: boolean
  reference_coefficients?: string[]
  rows_missing_money?: number
  rows_money_pending?: number
  usage_only_rows?: number
  test_priced_rows?: number
}

export interface UsageCustomerModelRow {
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
  model_ranks?: UsageReportRankItem[]
}

export type UsageBillingMode = BillingMode | 'per_image'

export interface UsageReportRankItem {
  name: string
  billing_mode?: UsageBillingMode
  channel_id?: number
  calls: number
  failure_calls?: number
  tokens: number
  missing_tokens: number
  token_usage?: UsageTokenMetrics
  amount: number | null
  pending_amount: number
  non_token?: UsageNonTokenMetrics
}

export interface UsageChannelMonthDiscount {
  period_start: number
  value: string
  changes?: { effective_from: number; value: string }[]
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
  channels: UsageUpstreamChannelTotal[]
}

export type UsageUpstreamChannelTotal = Pick<
  UsageUpstreamChannelRow,
  'channel_id' | 'channel_name' | 'total'
>

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

export type UsageReportDayReference = { start: number; generation: string }

export interface UsageAnalyticsEnvelope<T> {
  period: UsageAnalyticsPeriod
  filters: Record<string, unknown>
  result: T
  generated_at: number
  data_source: string
  coverage_note: string
  published_days?: string[]
  incomplete_days?: string[] | null
  rebuilding_days?: string[] | null
  source_unavailable_days?: string[] | null
  report_days?: UsageReportDayReference[]
  pending_refunds?: { task_id: string; requested_at: number }[] | null
}
