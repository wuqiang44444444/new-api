import { api } from '@/lib/api'

import type { ApiResponse } from './types'

export type SourceReviewScope = {
  user_id: number
  start_timestamp: number
  end_timestamp: number
}
export type SourceDecision =
  | 'keep_record'
  | 'investigate'
  | 'request_waiver'
  | 'request_duplicate_exclusion'
  | 'request_period_correction'
  | 'withdraw_request'
  | 'reject_request'
export type SourceDecisionInput = {
  fingerprint: string
  issue_id: string
  decision: SourceDecision
  reason: string
  note: string
  request_id: string
  previous_id: number
}
export type SourceDecisionRecord = SourceDecisionInput & {
  id: number
  version: number
  actor_id: number
  created_at: number
  model: string
  log_type: number
  recorded_quota: string
  kind: string
  blocking: boolean
  target_quota?: string
  evidence_gaps: string[]
  status: string
  statement_delta: string
  balance_delta: string
  current_source: boolean
  superseded: boolean
}
export type SourceIssue = {
  decision_id: number
  id: string
  kind: string
  blocking: boolean
  log_id: number
  task_row_id: number
  created_at: number
  model: string
  log_type: number
  quota: string
  target_quota?: string
  related_logs?: {
    id: number
    created_at: number
    log_type: number
    quota: string
    identity_matches: boolean
  }[]
  related_log_ids: number[]
  reasons: string[]
  reviewed: boolean
  note: string
  actor_id: number
  reviewed_at: number
}
export type SourceReview = {
  decision_version: number
  pending_requests: SourceDecisionRecord[]
  audit_issues: { id: number; actor_id: number; created_at: number }[]
  user_id: number
  start: number
  end: number
  captured_at: number
  fingerprint: string
  rows: number
  net_quota: string
  blockers: number
  pending: number
  issues: SourceIssue[]
  retention_status: string
  history: SourceDecisionRecord[]
}
export async function getSourceReview(params: SourceReviewScope) {
  const response = await api.get<ApiResponse<SourceReview>>(
    '/api/billing/admin/customer-statement-source-review',
    { params }
  )
  return response.data
}
export async function recordSourceReview(
  params: SourceReviewScope,
  body: SourceDecisionInput
) {
  const response = await api.post<{
    success: boolean
    message?: string
    decision_version: number
  }>('/api/billing/admin/customer-statement-source-review', body, { params })
  return response.data
}
