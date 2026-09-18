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

export interface ErrorLogItem {
  id: number
  created_at: number
  module: string
  event_type: string
  task_id: string
  method: string
  route: string
  status: number
  user_id: number
  username: string
  token_name: string
  model_name: string
  channel_id: number
  channel_name: string
  request_id: string
  upstream_request_id: string
  stage: string
  reason: string
  public_code: string
  protocol: string
  elapsed_ms: number
  detail: string
}

export interface ErrorLogFilters {
  p: number
  page_size: number
  module?: string
  event_type?: string
  task_id?: string
  status?: string
  user_id?: string
  username?: string
  token_name?: string
  model_name?: string
  channel?: string
  request_id?: string
  upstream_request_id?: string
  reason?: string
  start_timestamp?: number
  end_timestamp?: number
}

export async function getErrorLogs(
  params: ErrorLogFilters
): Promise<{ items: ErrorLogItem[]; total: number }> {
  const response = await api.get<
    ApiResponse<{ items: ErrorLogItem[]; total: number }>
  >('/api/error_log/', { params })
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Failed to load error records')
  }
  return response.data.data
}
