import type { CustomerExportJobView } from '@/features/billing-reconciliation/export-api'
import { api } from '@/lib/api'

import type {
  UsageAnalyticsEnvelope,
  UsagePeriodKind,
  UsageCustomerView,
  UsageCustomersOverview,
  UsageUpstreamView,
  UsageReportDayReference,
  UsageReportRankItem,
} from './types'

interface ApiResponse<T> {
  success: boolean
  message: string
  message_key?: string
  data: T
}

export type UsagePeriodParam = {
  period: UsagePeriodKind
  date: string
}

export type UsageReportQuery = UsagePeriodParam & { report_days?: string }

export function usageReportRebuildInterval(query: {
  state: {
    data?: { data?: Pick<UsageAnalyticsEnvelope<unknown>, 'rebuilding_days'> }
  }
}) {
  return query.state.data?.data?.rebuilding_days?.length ? 15_000 : false
}
type UpstreamUsageScope = { url_key?: string; channel_id?: number }
export type UsageRankMetric = 'calls' | 'tokens' | 'amount'

export async function getUsageSelfSummary(
  params: UsageReportQuery,
  signal?: AbortSignal
) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageCustomerView>>
  >(
    '/api/usage/self/summary',
    // React Query owns deduplication for cancellable queries.
    signal ? { params, signal, disableDuplicate: true } : { params }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message_key ||
        response.data.message ||
        'Usage request failed'
    )
  }
  response.data.data.result.keys ??= []
  annotateUnpublishedDays(response.data.data)
  return response.data
}

export async function getAdminUsageCustomers(
  params: UsageReportQuery & {
    search?: string
    compact?: boolean
    include_days?: boolean
  },
  signal?: AbortSignal
) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageCustomersOverview>>
  >(
    '/api/usage/admin/customers',
    signal ? { params, signal, disableDuplicate: true } : { params }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message_key ||
        response.data.message ||
        'Usage request failed'
    )
  }
  response.data.data.result.customers ??= []
  annotateUnpublishedDays(response.data.data)
  return response.data
}

export async function getAdminUsageRanking(
  kind: 'model' | 'channel',
  params: UsageReportQuery & {
    url_key?: string
    rank_metric: UsageRankMetric
    rank_page: number
    rank_page_size: number
  },
  signal: AbortSignal
) {
  const model = kind === 'model'
  const response = await api.get<
    ApiResponse<{
      items: UsageReportRankItem[]
      total: number
    }>
  >(
    model ? '/api/usage/admin/customers' : '/api/usage/admin/upstream-summary',
    {
      params: model ? { ...params, group_by: 'model' } : params,
      signal,
      disableDuplicate: true,
    }
  )
  if (!response.data.success || !Array.isArray(response.data.data?.items)) {
    throw new Error(
      response.data.message_key ||
        response.data.message ||
        'Usage request failed'
    )
  }
  return response.data.data
}

export async function getAdminUsageCustomerSummary(
  params: UsageReportQuery & { user_id: number },
  signal?: AbortSignal
) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageCustomerView>>
  >(
    '/api/usage/admin/customer-summary',
    signal ? { params, signal, disableDuplicate: true } : { params }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message_key ||
        response.data.message ||
        'Usage request failed'
    )
  }
  response.data.data.result.keys ??= []
  annotateUnpublishedDays(response.data.data)
  return response.data
}

export async function getAdminUsageUpstreamSummary(
  params: UsageReportQuery & UpstreamUsageScope & { compact?: boolean },
  signal?: AbortSignal
) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageUpstreamView>>
  >(
    '/api/usage/admin/upstream-summary',
    signal ? { params, signal, disableDuplicate: true } : { params }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message_key ||
        response.data.message ||
        'Usage request failed'
    )
  }
  response.data.data.result.url_groups ??= []
  annotateUnpublishedDays(response.data.data)
  return response.data
}

function annotateUnpublishedDays(report: UsageAnalyticsEnvelope<unknown>) {
  const published = report.published_days
  if (!published) return
  for (const day of report.period.days) {
    day.unpublished = !day.future && !published.includes(day.date)
  }
}

export interface UsageExportBody extends UsagePeriodParam, UpstreamUsageScope {
  report_days?: UsageReportDayReference[]
  view?: 'self' | 'customer' | 'customers' | 'upstream'
  search?: string
  user_id?: number
  language?: string
  model_name?: string
  token_id?: number
}

export async function createUsageSelfExport(body: UsageExportBody) {
  const response = await api.post<ApiResponse<CustomerExportJobView>>(
    '/api/usage/self/exports',
    body
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message_key ||
        response.data.message ||
        'Usage request failed'
    )
  }
  return response.data
}

export async function createUsageAdminExport(body: UsageExportBody) {
  const response = await api.post<ApiResponse<CustomerExportJobView>>(
    '/api/usage/admin/exports',
    body
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(
      response.data.message_key ||
        response.data.message ||
        'Usage request failed'
    )
  }
  return response.data
}
