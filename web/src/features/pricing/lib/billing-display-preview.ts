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
/**
 * 管理员只读表达式试算客户端：完整表达式（含倍率与除法）提交给后端
 * 引擎执行；试算不落账、不读取真实凭据。sample 未提供时仅返回投影。
 */
import { api } from '@/lib/api'

import type { BillingDisplayProjection } from '../types'

export type BillingExprPreviewSample = {
  /** openai: prompt 已含缓存等子类；anthropic: prompt 为纯文本输入。 */
  usage_semantic?: 'openai' | 'anthropic'
  prompt_tokens?: number
  completion_tokens?: number
  cache_read_tokens?: number
  cache_creation_tokens?: number
  cache_creation_tokens_1h?: number
  image_tokens?: number
  image_output_tokens?: number
  audio_input_tokens?: number
  audio_output_tokens?: number
  /** RFC3339 模拟时刻（可含时区偏移）；缺省为服务器当前时刻。 */
  pricing_time?: string
  /** 合成探针上下文（如 param("_task.resolution")），仅本次试算可见。 */
  body?: Record<string, unknown>
  headers?: Record<string, string>
}

export type BillingExprPreviewItem = {
  key: string
  expression: string
  sample?: BillingExprPreviewSample
}

export type BillingExprPreviewEvaluation = {
  pricing_time: string
  usage_semantic: string
  normalized_usage: Record<string, number>
  raw_cost_usd: number
  quota: number
  matched_tier: string
  request_rules?: Array<{
    cond: string
    multiplier: number
    matched: boolean
  }>
  saturated?: boolean
}

export type BillingExprPreviewItemResult = {
  key: string
  projection?: BillingDisplayProjection
  evaluation?: BillingExprPreviewEvaluation
  error?: string
}

type BillingExprPreviewResponse = {
  success: boolean
  message?: string
  data?: BillingExprPreviewItemResult[]
}

export const BILLING_EXPR_PREVIEW_MAX_ITEMS = 16

export async function previewBillingExpressions(
  items: BillingExprPreviewItem[]
): Promise<BillingExprPreviewItemResult[]> {
  const res = await api.post<BillingExprPreviewResponse>(
    '/api/option/billing-expression/preview',
    { items }
  )
  if (!res.data?.success || !res.data.data) {
    throw new Error(res.data?.message || 'billing expression preview failed')
  }
  return res.data.data
}
