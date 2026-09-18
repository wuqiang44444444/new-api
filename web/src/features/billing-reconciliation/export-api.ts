import i18n from '@/i18n/config'
import { toIntlLocale } from '@/i18n/languages'
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

import type { ApiResponse, BillingMode } from './types'

export type ExportJobType =
  | 'usage_logs'
  | 'statement_details'
  | 'statement_summary'

export type ExportJobStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'expired'
  | 'cancelling'

export type CustomerExportFiltersDto = {
  currency?: string
  field_version: number
  start_timestamp: number
  end_timestamp: number
  log_types?: number[]
  token_id?: number
  channel_id?: number
  token_name?: string
  group?: string
  request_id?: string
  upstream_request_id?: string
  username?: string
  model_name?: string
  billing_mode?: BillingMode
  timezone: string
  language?: string
}

export type CustomerExportArtifactFileDto = {
  object_key: string
  file_name: string
  size_bytes: number
  line_count: number
  sha256: string
}

export type CustomerExportArtifactDto = {
  files: CustomerExportArtifactFileDto[]
  line_count: number
  size_bytes: number
  generated_at: number
  expires_at: number
}

export type CustomerExportJobView = {
  job_id: string
  user_id: number
  target_user_id: number
  job_type: ExportJobType
  status: ExportJobStatus
  filters: CustomerExportFiltersDto
  progress: {
    scanned: number
    matched: number
    written: number
    files: number
    waiting_for_resources?: boolean
  }
  cancel_requested: boolean
  error_code?: string
  error?: string
  artifact?: CustomerExportArtifactDto
  created_at: number
  started_at?: number
  finished_at?: number
  expires_at?: number
}

export type CustomerExportSubmitPayload = {
  job_type: ExportJobType
  start_timestamp: number
  end_timestamp: number
  log_types?: number[]
  token_id?: number
  channel_id?: number
  token_name?: string
  group?: string
  request_id?: string
  upstream_request_id?: string
  username?: string
  model_name?: string
  billing_mode?: string
  language?: string
}

export type CustomerExportDownloadFile = {
  file_name: string
  url: string
  expires_at: number
  size_bytes: number
  line_count: number
  sha256: string
}

type ExportEnvelope = ApiResponse<{
  items?: CustomerExportJobView[]
  job?: CustomerExportJobView
  files?: CustomerExportDownloadFile[]
  empty_result?: boolean
  generated_at?: number
}>

export function currentExportLanguage(): string {
  const language = toIntlLocale(i18n.language) ?? 'en'
  const supported = new Set(['en', 'zh', 'zh-TW', 'fr', 'ru', 'ja', 'vi'])
  if (supported.has(language)) return language
  const base = language.split(/[-_]/)[0]
  if (base === 'zh') {
    const region = language.split(/[-_]/)[1]?.toLowerCase()
    return region === 'tw' ||
      region === 'hk' ||
      region === 'mo' ||
      region === 'hant'
      ? 'zh-TW'
      : 'zh'
  }
  return supported.has(base) ? base : 'en'
}

function assertOk(data: ExportEnvelope) {
  if (!data.success || !data.data) {
    throw new Error(data.message || 'Export request failed.')
  }
  return data.data
}

export async function createSelfExport(payload: CustomerExportSubmitPayload) {
  const response = await api.post<ExportEnvelope>('/api/billing/exports', {
    ...payload,
    language: currentExportLanguage(),
  })
  return assertOk(response.data) as unknown as CustomerExportJobView
}

export async function createAdminExport(
  userId: number | string,
  payload: CustomerExportSubmitPayload
) {
  const response = await api.post<ExportEnvelope>(
    '/api/billing/admin/customer-exports',
    { ...payload, language: currentExportLanguage() },
    {
      params:
        typeof userId === 'number' ? { user_id: userId } : { username: userId },
    }
  )
  return assertOk(response.data) as unknown as CustomerExportJobView
}

// 重新生成（6.2）：按任务自身冻结的范围重新提交一次新尝试。发起人即当前
// 用户；管理员对历史代客任务继续作用于同一目标客户。
export async function resubmitExport(job: CustomerExportJobView) {
  const payload: CustomerExportSubmitPayload = {
    job_type: job.job_type,
    start_timestamp: job.filters.start_timestamp,
    end_timestamp: job.filters.end_timestamp,
    language: job.filters.language,
  }
  if (job.filters.log_types?.length) payload.log_types = job.filters.log_types
  if (job.filters.token_id != null) payload.token_id = job.filters.token_id
  if (job.filters.channel_id != null) {
    payload.channel_id = job.filters.channel_id
  }
  payload.token_name = job.filters.token_name
  payload.group = job.filters.group
  payload.request_id = job.filters.request_id
  payload.upstream_request_id = job.filters.upstream_request_id
  payload.username = job.filters.username
  if (job.filters.model_name) payload.model_name = job.filters.model_name
  if (job.filters.billing_mode) payload.billing_mode = job.filters.billing_mode
  if (job.target_user_id !== job.user_id) {
    return createAdminExport(job.target_user_id, payload)
  }
  return createSelfExport(payload)
}

export async function listSelfExports() {
  const response = await api.get<ExportEnvelope>('/api/billing/exports')
  const data = assertOk(response.data)
  return data.items ?? []
}

export async function getExport(jobId: string) {
  const response = await api.get<ExportEnvelope>(
    `/api/billing/exports/${encodeURIComponent(jobId)}`
  )
  return assertOk(response.data) as unknown as CustomerExportJobView
}

export async function cancelExport(jobId: string) {
  const response = await api.post<ExportEnvelope>(
    `/api/billing/exports/${encodeURIComponent(jobId)}/cancel`
  )
  return assertOk(response.data) as unknown as CustomerExportJobView
}

export async function requestExportDownload(jobId: string) {
  const response = await api.get<ExportEnvelope>(
    `/api/billing/exports/${encodeURIComponent(jobId)}/download`
  )
  const data = assertOk(response.data)
  return {
    files: data.files ?? [],
    emptyResult: data.empty_result === true,
    generatedAt: data.generated_at ?? 0,
  }
}

export const activeExportStatuses: ExportJobStatus[] = [
  'queued',
  'running',
  'cancelling',
]

export function isExportJobActive(job: CustomerExportJobView) {
  return activeExportStatuses.includes(job.status)
}
