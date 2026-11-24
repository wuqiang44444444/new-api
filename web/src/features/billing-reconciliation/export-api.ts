import { isAxiosError } from 'axios'

import i18n from '@/i18n/config'
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
  | 'statement_archive'
  | 'statement_summary'
  | 'customer_statements'

export type ExportJobStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'expired'
  | 'cancelling'

export type CustomerExportFiltersDto = {
  billing_date?: string
  billing_report_coverage?: {
    incomplete_days: number
    published_days?: number
    expected_days?: number
    estimated_at?: number
  }
  billing_report_version?: string
  statement_draft_id?: string
  usage_export_grain?: 'period'
  usage_view?: string
  usage_url_key?: string
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
  request_net_amount?: string
  missing_report_dates?: string[]
  line_count: number
  size_bytes: number
  generated_at: number
  expires_at: number
}

export type CustomerExportJobView = {
  confirm_existing?: boolean
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
    last_committed_at?: number
    waiting_for_resources?: boolean
    archive?: {
      packaged_files: number
      total_files: number
      uploading: boolean
    }
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
  confirm_existing?: boolean
  force_new?: boolean
  all_channels?: boolean
  report_version?: string
  statement_version?: string
  url_key?: string
  evidence_filter?: string
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

export function hasExportCoverageFile(job: CustomerExportJobView) {
  if (
    job.job_type !== 'statement_details' &&
    job.job_type !== 'upstream_details'
  ) {
    return false
  }
  const coverage = job.filters.billing_report_coverage
  return Boolean(
    coverage &&
    ((coverage.published_days ?? 0) < (coverage.expected_days ?? 0) ||
      coverage.incomplete_days ||
      coverage.estimated_at)
  )
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

export type CustomerStatementExportDay = {
  date: string
  status:
    | 'complete'
    | 'incomplete'
    | 'empty'
    | 'unpublished'
    | 'in_progress'
    | 'rebuilding'
    | 'source_unavailable'
  report_version?: string
}

type CustomerStatementsExportScope = (
  | {
      start_timestamp: number
      end_timestamp: number
      billing_date?: never
      expected_incomplete?: never
    }
  | {
      billing_date: string
      expected_incomplete: boolean
      start_timestamp?: never
      end_timestamp?: never
    }
) & { report_version: string; language?: string }

export async function getCustomerStatementExportDays(
  period: { start_timestamp: number; end_timestamp: number },
  signal: AbortSignal
) {
  const response = await api.get<
    ApiResponse<{ days: CustomerStatementExportDay[]; default_date?: string }>
  >('/api/billing/admin/customer-statement-export-days', {
    params: period,
    signal,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message)
  }
  return response.data.data
}

export async function getCustomerStatementDetailExportDays(
  params: {
    start_timestamp: number
    end_timestamp: number
    report_version?: string
    statement_version?: string
    user_id?: number
  },
  signal: AbortSignal
) {
  const endpoint =
    params.user_id == null
      ? '/api/billing/statement/self/detail-export-days'
      : '/api/billing/admin/customer-detail-export-days'
  const response = await api.get<
    ApiResponse<{
      days: {
        date: string
        start_timestamp: number
        end_timestamp: number
        incomplete?: boolean
      }[]
    }>
  >(endpoint, {
    params,
    signal,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message)
  }
  return response.data.data.days
}

export const billingExportLanguage = 'zh'

function assertOk(data: ExportEnvelope) {
  if (!data.success || !data.data) {
    throw new Error(data.message || 'Export request failed.')
  }
  return data.data
}

export async function createSelfExport(payload: CustomerExportSubmitPayload) {
  const response = await api.post<ExportEnvelope>('/api/billing/exports', {
    ...payload,
    language: billingExportLanguage,
  })
  return assertOk(response.data) as unknown as CustomerExportJobView
}

export async function createAdminExport(
  userId: number | string,
  payload: CustomerExportSubmitPayload
) {
  const response = await api.post<ExportEnvelope>(
    '/api/billing/admin/customer-exports',
    { ...payload, language: billingExportLanguage },
    {
      params:
        typeof userId === 'number' ? { user_id: userId } : { username: userId },
    }
  )
  return assertOk(response.data) as unknown as CustomerExportJobView
}

export async function createCustomerStatementsExport(
  payload: CustomerStatementsExportScope
) {
  return submitBillingExport('/api/billing/admin/customer-exports', {
    ...payload,
    job_type: 'customer_statements',
    language: billingExportLanguage,
  })
}

// 重新生成（6.2）：按任务自身冻结的范围重新提交一次新尝试。发起人即当前
// 用户；管理员对历史代客任务继续作用于同一目标客户。
export async function resubmitExport(job: CustomerExportJobView) {
  if (job.job_type === 'customer_statements') {
    if (job.filters.billing_date) {
      return submitBillingExport('/api/billing/admin/customer-exports', {
        job_type: 'customer_statements',
        source_job_id: job.job_id,
      })
    }
    return createCustomerStatementsExport({
      start_timestamp: job.filters.start_timestamp,
      end_timestamp: job.filters.end_timestamp,
      report_version: job.filters.billing_report_version ?? '',
    })
  }
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
  }
  if (job.filters.log_types?.length) payload.log_types = job.filters.log_types
  if (job.filters.token_id != null) payload.token_id = job.filters.token_id
  if (job.filters.channel_id != null) {
    payload.channel_id = job.filters.channel_id
  }
  payload.token_name = job.filters.token_name
  payload.group = job.filters.group
  payload.request_id = job.filters.request_id
  if (
    job.job_type === 'statement_details' ||
    job.job_type === 'statement_summary'
  ) {
    if (job.filters.statement_draft_id) {
      payload.statement_version = job.filters.statement_draft_id
    } else if (job.filters.billing_report_version) {
      payload.report_version = job.filters.billing_report_version
    }
  }
  payload.upstream_request_id = job.filters.upstream_request_id
  payload.username = job.filters.username
  if (job.filters.model_name) payload.model_name = job.filters.model_name
  if (job.filters.billing_mode) payload.billing_mode = job.filters.billing_mode
  if (
    job.job_type === 'upstream_details' ||
    job.job_type === 'upstream_summary'
  ) {
    return submitBillingExport('/api/billing/admin/upstream-exports', {
      source_job_id: job.job_id,
    })
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

export async function requestExportDownload(
  jobId: string,
  verifyExisting = false
) {
  const url = `/api/billing/exports/${encodeURIComponent(jobId)}/download`
  const response = verifyExisting
    ? await api.get<ExportEnvelope>(url, {
        params: { verify_existing: true },
        skipErrorHandler: true,
        skipBusinessError: true,
      })
    : await api.get<ExportEnvelope>(url)
  const data = assertOk(response.data)
  return {
    files: data.files ?? [],
    emptyResult: data.empty_result === true,
    generatedAt: data.generated_at ?? 0,
  }
}

export async function downloadExportArchive(jobId: string) {
  const url = `/api/billing/exports/${encodeURIComponent(jobId)}/archive.zip`
  await api.post(url, undefined, { skipErrorHandler: true })
  downloadExportFile(url, true)
}

export async function createExportShareArchive(jobId: string) {
  return submitBillingExport(
    `/api/billing/exports/${encodeURIComponent(jobId)}/share-archive`
  )
}

export async function requestExportShareLink(jobId: string) {
  const { data } = await api.get<
    ApiResponse<{ url: string; expires_at: number }>
  >(`/api/billing/exports/${encodeURIComponent(jobId)}/share-link`, {
    skipErrorHandler: true,
  })
  if (!data.success || !data.data?.url) throw new Error('Missing download link')
  return data.data.url
}

export function downloadExportFile(url: string, attachment = false) {
  // The signed GET serves an attachment. A same-tab link also works after
  // awaiting the signing request, when a new window may be blocked.
  const link = document.createElement('a')
  link.href = url
  link.rel = 'noreferrer'
  if (attachment) link.download = ''
  link.click()
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
  return submitBillingExport('/api/billing/admin/upstream-exports', {
    ...payload,
    language: billingExportLanguage,
  })
}

// Callers own the single toast. Normalize both HTTP and business failures here
// so new submissions and regeneration preserve the server's specific reason.
async function submitBillingExport(
  path: string,
  payload?:
    | CustomerExportSubmitPayload
    | (CustomerStatementsExportScope & { job_type: 'customer_statements' })
    | { source_job_id: string; job_type?: 'customer_statements' }
) {
  try {
    const response = await api.post<ExportEnvelope>(path, payload, {
      skipErrorHandler: true,
      skipBusinessError: true,
    })
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
