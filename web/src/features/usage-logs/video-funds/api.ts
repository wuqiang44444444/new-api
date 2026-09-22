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
import type { ApiResponse } from '@/features/profile/types'
import { api } from '@/lib/api'

export interface VideoFundItem {
  kind: 'task' | 'attempt'
  id: number
  task_id: string
  request_id: string
  user_id: number
  app_id: number
  channel_id: number
  model: string
  source: string
  business_status: string
  fund_state: string
  quota: number
  refunded_quota: number
  refund_amount_known: boolean
  waived_quota: number
  created_at: number
  deadline_at: number
  refunded_at: number
  retry_at: number
  operator_id: number
  note: string
  failure: string
  delivery: string
  version: string
  can_refund: boolean
}
export interface VideoFundFilters {
  p: number
  page_size: number
  task_id?: string
  app_id?: string
  refund_from?: number
  refund_to?: number
  user_id?: string
  channel_id?: string
  state?: string
}
export interface RefundInstruction {
  kind: 'task' | 'attempt'
  id: number
  version: string
  note: string
}
export async function getVideoFunds(params: VideoFundFilters) {
  const { data } = await api.get<
    ApiResponse<{
      items: VideoFundItem[]
      total: number
      summary?: VideoFundSummary
    }>
  >('/api/video-funds', { params })
  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to load video fund records')
  }
  return data.data
}
export async function getVideoFund(item: Pick<VideoFundItem, 'kind' | 'id'>) {
  const { data } = await api.get<
    ApiResponse<VideoFundItem> & { timeline?: VideoFundEvent[] }
  >(`/api/video-funds/${item.kind}/${item.id}`)
  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to load video fund records')
  }
  return { ...data.data, timeline: data.timeline ?? [] }
}
export async function refundVideoFunds(
  instruction: RefundInstruction,
  proof: string
) {
  const { data } = await api.post<ApiResponse<VideoFundItem>>(
    `/api/video-funds/${instruction.kind}/${instruction.id}/refund`,
    instruction,
    { headers: { 'X-Security-Proof': proof } }
  )
  if (!data.success || !data.data) {
    throw new Error(data.message || 'Refund request failed')
  }
  return data.data
}

export interface VideoFundSummary {
  held_count: number
  held_quota: number
  abnormal_count: number
  abnormal_quota: number
  overdue_count: number
  overdue_quota: number
  failed_count: number
  failed_quota: number
  oldest_held_at: number
  returned_quota: number
}
export interface VideoFundEvent {
  event: string
  at: number
  before_quota: number
  after_quota: number
}
export type VideoFundProgress = Pick<
  VideoFundItem,
  | 'task_id'
  | 'request_id'
  | 'model'
  | 'fund_state'
  | 'quota'
  | 'refunded_quota'
  | 'refund_amount_known'
  | 'created_at'
  | 'deadline_at'
  | 'refunded_at'
  | 'retry_at'
>
export async function getSelfVideoFunds(p: number, taskId: string) {
  const { data } = await api.get<
    ApiResponse<{ items: VideoFundProgress[]; total: number }>
  >('/api/video-funds/self', { params: { p, task_id: taskId } })
  if (!data.success || !data.data) {
    throw new Error(data.message || 'Failed to load video fund records')
  }
  return data.data
}
