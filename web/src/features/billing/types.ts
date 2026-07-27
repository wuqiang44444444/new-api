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
export const BILLING_PRESETS = [
  'this-week',
  'last-week',
  'last-7-days',
  'last-30-days',
  'custom',
] as const

export type BillingPreset = (typeof BILLING_PRESETS)[number]
export type BillingView = 'detail' | 'model' | 'key'

export type BillingSearch = {
  preset?: BillingPreset
  startTime?: number
  endTime?: number
  tokenId?: number
  model?: string
  view?: BillingView
}

export type BillingPeriod = {
  start_timestamp: number
  end_timestamp: number
}

export type BillingFunds = {
  current_balance: number
  lifetime_consumed: number
}

export type BillingSummary = {
  requests: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  gross_quota: number
  refund_quota: number
  net_quota: number
  average_rpm: number
  average_tpm: number
  average_use_time_seconds: number
  stream_ratio: number
}

export type BillingStatementItem = {
  token_id: number
  token_name: string
  model_name: string
  requests: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  gross_quota: number
  refund_quota: number
  net_quota: number
  average_use_time_seconds: number
  stream_requests: number
  latest_request_timestamp: number
}

export type BillingStatementData = {
  period: BillingPeriod
  funds: BillingFunds
  summary: BillingSummary
  items: BillingStatementItem[]
  generated_at: number
  data_source: 'settlement_logs'
}

export type BillingStatementResponse = {
  success: boolean
  message: string
  data?: BillingStatementData
}
