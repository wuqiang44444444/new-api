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

import type {
  ApiResponse,
  BillingEnvelope,
  CustomerStatement,
  CustomerStatementList,
  CustomerStatementListQuality,
  CustomerStatementListSortBy,
  CustomerStatementListSortOrder,
  ProviderSummary,
} from './types'

type PeriodParams = {
  start_timestamp: number
  end_timestamp: number
}

export async function getSelfCustomerStatement(params: PeriodParams) {
  const response = await api.get<
    ApiResponse<BillingEnvelope<CustomerStatement>>
  >('/api/billing/statement/self', { params })
  return response.data
}

export async function getAdminCustomerStatement(
  params: PeriodParams & { user_id: number; dimension: string }
) {
  const response = await api.get<
    ApiResponse<BillingEnvelope<CustomerStatement>>
  >('/api/billing/admin/customer-summary', { params })
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
  return response.data
}

export async function getAdminUpstreamStatement(params: PeriodParams) {
  const response = await api.get<ApiResponse<BillingEnvelope<ProviderSummary>>>(
    '/api/billing/admin/upstream-summary',
    { params }
  )
  return response.data
}
