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
import { api } from '@/lib/api'

import { assertCustomerBillingPrecision } from './billing-precision'
import type {
  ApiResponse,
  BillingEnvelope,
  CustomerStatement,
  CustomerStatementList,
  CustomerStatementListQuality,
  CustomerStatementListSortBy,
  CustomerStatementListSortOrder,
  ProviderUrlSummary,
  UpstreamDetails,
} from './types'

type PeriodParams = {
  start_timestamp: number
  end_timestamp: number
}

export async function getSelfCustomerStatement(params: PeriodParams) {
  const response = await api.get<
    ApiResponse<BillingEnvelope<CustomerStatement>>
  >('/api/billing/statement/self', { params })
  assertCustomerBillingPrecision(response.data)
  return response.data
}

export async function getAdminCustomerStatement(
  params: PeriodParams & { user_id: number; dimension: string }
) {
  const response = await api.get<
    ApiResponse<BillingEnvelope<CustomerStatement>>
  >('/api/billing/admin/customer-summary', { params })
  assertCustomerBillingPrecision(response.data)
  return response.data
}

export async function getAdminCustomerStatements(
  params: PeriodParams & {
    search?: string
    quality_status?: Exclude<CustomerStatementListQuality, 'all'>
    sort_by: CustomerStatementListSortBy
    sort_order: CustomerStatementListSortOrder
    page: number
    page_size: number
  }
) {
  const response = await api.get<
    ApiResponse<BillingEnvelope<CustomerStatementList>>
  >('/api/billing/admin/customer-statements', { params })
  assertCustomerBillingPrecision(response.data)
  return response.data
}

// 统一上游对账：URL 分组汇总 + 原价/折后参考金额 + 渠道月度折扣编辑区。
export async function getAdminUpstreamReconciliation(
  params: PeriodParams & {
    url_key?: string
    level?: 'groups' | 'channels' | 'models' | 'options'
    channel_id?: number
    search?: string
    page?: number
    page_size?: number
  },
  signal?: AbortSignal
) {
  const response = await api.get<
    ApiResponse<BillingEnvelope<ProviderUrlSummary>>
  >('/api/billing/admin/upstream-summary', {
    // React Query owns deduplication and cancellation for this request.
    disableDuplicate: !!signal,
    skipErrorHandler: true,
    signal,
    params: {
      ...params,
      p: params.page ?? 1,
      page_size: params.page_size ?? 20,
      page: undefined,
    },
  })
  return response.data
}

export async function putAdminUpstreamDiscount(payload: {
  period_start: number
  channel_id: number
  discount: string
  expected_version: number
}) {
  const response = await api.put<ApiResponse<unknown>>(
    '/api/billing/admin/upstream-discounts',
    payload
  )
  return response.data
}

// 上游名称：按 URL 分组键保存或清空（空名称恢复默认安全 URL 展示）。
export async function putAdminUpstreamURLName(payload: {
  url_key: string
  name: string
}) {
  const response = await api.put<ApiResponse<unknown>>(
    '/api/billing/admin/upstream-url-names',
    payload
  )
  return response.data
}

export async function postAdminUpstreamDiscountInit(payload: {
  period_start: number
  channel_ids?: number[]
  end_timestamp?: number
}) {
  const response = await api.post<
    ApiResponse<{
      outcomes?: Array<{ channel_id: number; outcome: string }>
      counts?: Record<string, number>
    }>
  >('/api/billing/admin/upstream-discounts/initialize', payload)
  return response.data
}

export async function getAdminUpstreamDetails(
  params: PeriodParams & {
    channel_id?: number
    url_key?: string
    evidence_filter?: string
    model_name?: string
    provider_model_fallback?: boolean
    billing_mode?: string
    request_id?: string
    upstream_request_id?: string
    page?: number
    page_size?: number
  }
) {
  const response = await api.get<ApiResponse<BillingEnvelope<UpstreamDetails>>>(
    '/api/billing/admin/upstream-details',
    { params: { ...params, p: params.page, page: undefined } }
  )
  return response.data
}
