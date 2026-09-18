import { api } from '@/lib/api'

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
import { currentExportLanguage } from './export-api'
import type {
  ApiResponse,
  BillingDataQuality,
  BillingDiscountCombination,
} from './types'

// 客户月账单版本固化 API（docs/80-dev/2026-09-17 方案第 13 节）。
// 版本绑定后的汇总/明细/下载全部携带 draft_public_id，金额只来自服务端冻结事实。

export type BillingStatementVersionStatus =
  | 'queued'
  | 'generating'
  | 'pending'
  | 'confirmed'
  | 'failed'
  | 'cancelled'
  | 'invalid'
  | 'cleaning'

export type BillingStatementVersionInfo = {
  id: number
  draft_public_id: string
  status: BillingStatementVersionStatus
  version_number: number | null
  user_id: number
  period_start: number
  period_end_exclusive: number
  confirmed_at: number
  created_at: number
  updated_at: number
  public_reason: string
  corrects_version_id: number | null
  quota_per_unit: number
  currency: string
  currency_rate: number
  data_quality?: BillingDataQuality | null
  internal_note?: string
}

export type BillingStatementVersionMonthStatus = {
  switch_enabled: boolean
  topology_ok: boolean
  retention_status: 'unknown' | 'none' | 'intact' | 'partial'
  current_version: BillingStatementVersionInfo | null
  active_draft: BillingStatementVersionInfo | null
  has_active_draft: boolean
  versions: BillingStatementVersionInfo[]
}

export type BillingStatementAmountDiff = {
  base_usd?: string
  compare_usd?: string
  delta_usd?: string
  base: number | null
  compare: number | null
  delta?: number
}

export type BillingStatementVersionDiff = {
  usage: Record<
    string,
    { base: string | null; compare: string | null; delta: string | null }
  >
  discounts: {
    base: BillingDiscountCombination[]
    compare: BillingDiscountCombination[]
  }
  base_version_id: number
  compare_version_id: number
  items: Record<string, BillingStatementAmountDiff>
  groups: {
    group_id: number
    name: string
    net: BillingStatementAmountDiff
  }[]
  models: {
    group_id: number
    model_name: string
    billing_mode: string
    net: BillingStatementAmountDiff
  }[]
  quality: Record<string, BillingStatementAmountDiff>
}

export type BillingStatementVersionLine = {
  id: string
  sequence: number
  request_id: string
  token_id: number
  token_name: string
  customer_model: string
  group: string
  log_type: number
  created_at: number
  billing_mode: string
  input_tokens: string
  output_tokens: string
  cache_read_tokens: string
  cache_write_tokens: string
  quota: string
  facts: {
    input_tokens_unavailable: boolean
    group_name: string
    group_ratio?: number
    contract_applicable: 'yes' | 'no' | 'unrecorded' | 'unknown'
    contract_name: string
    contract_ratio?: number
    final_ratio?: number
    billing_line_items: string
    explanation_status: string
  }
}

type PeriodParams = {
  start_timestamp: number
  end_timestamp: number
}

export async function verifyAdminStatementSource(
  params: PeriodParams & { user_id: number },
  body: {
    fingerprint: string
    backup_evidence: string
    retention_evidence: string
  }
) {
  const response = await api.post<{ success: boolean; message?: string }>(
    '/api/billing/admin/customer-statement-source-verification',
    body,
    { params }
  )
  return response.data
}

export async function getAdminVersionMonthStatus(
  params: PeriodParams & { user_id: number }
) {
  const response = await api.get<
    ApiResponse<BillingStatementVersionMonthStatus>
  >('/api/billing/admin/customer-statement-version-month-status', { params })
  return response.data
}

export async function getSelfVersionMonthStatus(params: PeriodParams) {
  const response = await api.get<
    ApiResponse<BillingStatementVersionMonthStatus>
  >('/api/billing/statement/self/version-month-status', { params })
  return response.data
}

export async function createAdminVersion(
  params: PeriodParams & { user_id: number }
) {
  const response = await api.post<{
    success: boolean
    message?: string
    draft_public_id?: string
    status?: string
  }>('/api/billing/admin/customer-statement-versions', null, {
    params: { ...params, language: currentExportLanguage() },
  })
  return response.data
}

export async function createAdminVersionCorrection(
  params: PeriodParams & { user_id: number },
  body: {
    base_version_id?: number
    public_reason: string
    internal_note?: string
  }
) {
  const response = await api.post<{
    success: boolean
    message?: string
    draft_public_id?: string
    status?: string
  }>('/api/billing/admin/customer-statement-versions/correction', body, {
    params: { ...params, language: currentExportLanguage() },
  })
  return response.data
}

export async function confirmAdminVersion(
  draftPublicId: string,
  body: {
    base_version_id?: number | null
    idempotency_key: string
    acknowledged_quality: string
    public_reason?: string
  }
) {
  const response = await api.post<{
    success: boolean
    message?: string
    committed?: boolean
    version?: BillingStatementVersionInfo
  }>(
    `/api/billing/admin/customer-statement-versions/${draftPublicId}/confirm`,
    body
  )
  return response.data
}

export async function abandonAdminVersion(
  draftPublicId: string,
  reason?: string
) {
  const response = await api.post<{ success: boolean; message?: string }>(
    `/api/billing/admin/customer-statement-versions/${draftPublicId}/abandon`,
    { reason }
  )
  return response.data
}

export async function cleanupAdminVersion(draftPublicId: string) {
  const response = await api.post<{ success: boolean; message?: string }>(
    `/api/billing/admin/customer-statement-versions/${draftPublicId}/cleanup`
  )
  return response.data
}

export async function getAdminVersionDiff(
  params: PeriodParams & {
    user_id: number
    base_id?: number
    compare_id?: number
  }
) {
  const response = await api.get<{
    success: boolean
    message?: string
    data?: {
      diff: BillingStatementVersionDiff
      base: BillingStatementVersionInfo
      compare: BillingStatementVersionInfo
    }
  }>('/api/billing/admin/customer-statement-version-diff', { params })
  return response.data
}

export async function getAdminVersionLines(
  draftPublicId: string,
  params: {
    page: number
    page_size: number
    channel_id?: number
    token_id?: number
    model_name?: string
    billing_mode?: string
  }
) {
  const response = await api.get<{
    success: boolean
    message?: string
    data?: { lines: BillingStatementVersionLine[]; total: number }
  }>(`/api/billing/admin/customer-statement-versions/${draftPublicId}/lines`, {
    params,
  })
  return response.data
}

export async function getSelfVersionLines(
  draftPublicId: string,
  params: {
    page: number
    page_size: number
    token_id?: number
    model_name?: string
    billing_mode?: string
  }
) {
  const response = await api.get<{
    success: boolean
    message?: string
    data?: { lines: BillingStatementVersionLine[]; total: number }
  }>(`/api/billing/statement/self/versions/${draftPublicId}/lines`, { params })
  return response.data
}

export async function getAdminVersionDownload(
  draftPublicId: string,
  role?: string
) {
  const response = await api.get<{
    success: boolean
    message?: string
    data?: { url: string; file_name: string; expires_at: number }
  }>(
    `/api/billing/admin/customer-statement-versions/${draftPublicId}/download`,
    {
      params: role ? { role } : {},
    }
  )
  return response.data
}

export async function getSelfVersionDownload(
  draftPublicId: string,
  role?: string
) {
  const response = await api.get<{
    success: boolean
    message?: string
    data?: { url: string; file_name: string; expires_at: number }
  }>(`/api/billing/statement/self/versions/${draftPublicId}/download`, {
    params: role ? { role } : {},
  })
  return response.data
}
