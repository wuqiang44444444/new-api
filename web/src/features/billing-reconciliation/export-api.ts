import { isAxiosError } from 'axios'

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
import { getServerErrorMessageKey } from '@/lib/server-error-message'

import type { ApiResponse, BillingMode } from './types'

export type ExportJobType =
  | 'usage_summary'
  | 'upstream_details'
  | 'upstream_summary'
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
  usage_view?: string
  usage_search?: string
  upstream?: {
    all_channels?: boolean
    group_name?: string
    url_key?: string
    channel_ids: number[]
    provider_model_fallback?: boolean
  }
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
  evidence_filter?: string
  url_key?: string
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
  provider_model_fallback?: boolean
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
  total?: number
  page?: number
  page_size?: number
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
  if (job.job_type === 'usage_summary') {
    const self = job.target_user_id === job.user_id
    const response = await api.post<ExportEnvelope>(
      self ? '/api/usage/self/exports' : '/api/usage/admin/exports',
      { source_job_id: job.job_id },
      { skipErrorHandler: true }
    )
    return assertOk(response.data) as unknown as CustomerExportJobView
  }
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
  if (
    job.job_type === 'upstream_details' ||
    job.job_type === 'upstream_summary'
  ) {
    return submitUpstreamExport({ source_job_id: job.job_id })
  }
  if (job.target_user_id !== job.user_id) {
    return createAdminExport(job.target_user_id, payload)
  }
  return createSelfExport(payload)
}

export async function listSelfExports(page = 1, pageSize = 20) {
  const response = await api.get<ExportEnvelope>('/api/billing/exports', {
    params: { p: page, page_size: pageSize },
  })
  const data = assertOk(response.data)
  return {
    items: data.items ?? [],
    total: data.total ?? 0,
    page: data.page ?? page,
    page_size: data.page_size ?? pageSize,
  }
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

export async function createUpstreamExport(
  payload: CustomerExportSubmitPayload
) {
  return submitUpstreamExport({
    ...payload,
    language: (payload.language ?? currentExportLanguage()).startsWith('zh')
      ? 'zh'
      : 'en',
  })
}

// Callers own the single toast. Normalize both HTTP and business failures here
// so new submissions and regeneration preserve the server's specific reason.
async function submitUpstreamExport(
  payload: CustomerExportSubmitPayload | { source_job_id: string }
) {
  try {
    const response = await api.post<ExportEnvelope>(
      '/api/billing/admin/upstream-exports',
      payload,
      { skipErrorHandler: true, skipBusinessError: true }
    )
    if (!response.data.success) {
      const key = getServerErrorMessageKey(response.data)
      if (key) throw new Error(i18n.t(key))
    }
    return assertOk(response.data) as unknown as CustomerExportJobView
  } catch (error) {
    const key = getServerErrorMessageKey(error)
    const serverMessage: unknown = isAxiosError(error)
      ? error.response?.data?.message
      : undefined
    if (key) throw new Error(i18n.t(key))
    if (typeof serverMessage === 'string' && serverMessage.trim()) {
      throw new Error(i18n.t(serverMessage))
    }
    throw new Error(
      error instanceof Error && error.message
        ? i18n.t(error.message)
        : i18n.t('Unable to submit export.')
    )
  }
}
