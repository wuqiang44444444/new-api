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

// 上游对账按渠道读取；配置编辑独立放在渠道管理。
export async function getAdminUpstreamReconciliation(
  params: PeriodParams & {
    level?: 'channels' | 'models' | 'options'
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

export async function getAdminUpstreamDetails(
  params: PeriodParams & {
    channel_id?: number
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
