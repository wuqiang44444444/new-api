import { api } from '@/lib/api'

import type {
  UsageAnalyticsEnvelope,
  UsageCustomerView,
  UsageCustomersOverview,
  UsageExportJobView,
  UsageUpstreamView,
} from './types'

interface ApiResponse<T> {
  success: boolean
  message: string
  data: T
}

export type UsagePeriodParam = {
  period: 'day' | 'week'
  date: string
}

export async function getUsageSelfSummary(params: UsagePeriodParam) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageCustomerView>>
  >('/api/usage/self/summary', { params })
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Usage request failed')
  }
  response.data.data.result.keys ??= []
  return response.data
}

export async function getAdminUsageCustomers(
  params: UsagePeriodParam & { search?: string }
) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageCustomersOverview>>
  >('/api/usage/admin/customers', { params })
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Usage request failed')
  }
  response.data.data.result.customers ??= []
  return response.data
}

export async function getAdminUsageCustomerSummary(
  params: UsagePeriodParam & { user_id: number }
) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageCustomerView>>
  >('/api/usage/admin/customer-summary', { params })
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Usage request failed')
  }
  response.data.data.result.keys ??= []
  return response.data
}

export async function getAdminUsageUpstreamSummary(params: UsagePeriodParam) {
  const response = await api.get<
    ApiResponse<UsageAnalyticsEnvelope<UsageUpstreamView>>
  >('/api/usage/admin/upstream-summary', { params })
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Usage request failed')
  }
  response.data.data.result.url_groups ??= []
  return response.data
}

export interface UsageExportBody extends UsagePeriodParam {
  view?: 'self' | 'customer' | 'customers' | 'upstream'
  search?: string
  user_id?: number
  language?: string
}

export async function createUsageSelfExport(
  body: UsagePeriodParam & { language?: string }
) {
  const response = await api.post<ApiResponse<UsageExportJobView>>(
    '/api/usage/self/exports',
    body
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Usage request failed')
  }
  return response.data
}

export async function createUsageAdminExport(body: UsageExportBody) {
  const response = await api.post<ApiResponse<UsageExportJobView>>(
    '/api/usage/admin/exports',
    body
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Usage request failed')
  }
  return response.data
}
