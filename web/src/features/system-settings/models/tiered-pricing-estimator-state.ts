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
import type { ExtraTokenValues } from '@/features/pricing/lib/tier-expr'

// 费用估算器的「浏览器本地草稿」。仅用于本地预览表达式结果，不参与真实计费、不进入
// buildSubmitData、不调用后端 API。与真实计费配置 task_billing_setting.preconsume_tokens
// 严格分离（见 AsyncPreConsumeTokenField）。

export type EstimatorResolution = '480p' | '720p' | '1080p' | '4k'

export type EstimatorTaskProbe = {
  hasVideoInput: boolean
  resolution: EstimatorResolution
}

export type EstimatorDraft = {
  promptTokens: number
  completionTokens: number
  extras: ExtraTokenValues
  taskProbe: EstimatorTaskProbe
}

// 版本写入 key 前缀：将来字段演进时升 v2，旧 v1 草稿自然失效回退默认值。
const KEY_PREFIX = 'model-pricing-estimator:v1:'

export const DEFAULT_PROBE: EstimatorTaskProbe = {
  hasVideoInput: false,
  resolution: '720p',
}

export function createDefaultExtras(): ExtraTokenValues {
  return {
    cacheReadTokens: 0,
    cacheCreateTokens: 0,
    cacheCreate1hTokens: 0,
    imageTokens: 0,
    imageOutputTokens: 0,
    audioInputTokens: 0,
    audioOutputTokens: 0,
  }
}

export function createDefaultDraft(): EstimatorDraft {
  return {
    promptTokens: 0,
    completionTokens: 0,
    extras: createDefaultExtras(),
    taskProbe: { ...DEFAULT_PROBE },
  }
}

const RESOLUTIONS: readonly EstimatorResolution[] = [
  '480p',
  '720p',
  '1080p',
  '4k',
] as const

function asNonNegativeFinite(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? value
    : 0
}

function normalizeExtras(raw: unknown): ExtraTokenValues {
  const base = createDefaultExtras()
  if (!raw || typeof raw !== 'object') return base
  const src = raw as Record<string, unknown>
  for (const key of Object.keys(base) as (keyof ExtraTokenValues)[]) {
    base[key] = asNonNegativeFinite(src[key])
  }
  return base
}

function normalizeProbe(raw: unknown): EstimatorTaskProbe {
  const probe: EstimatorTaskProbe = { ...DEFAULT_PROBE }
  if (!raw || typeof raw !== 'object') return probe
  const src = raw as Record<string, unknown>
  probe.hasVideoInput = src.hasVideoInput === true
  // Migrate the original display-only value to the real backend probe value.
  if (src.resolution === '480p720p') {
    probe.resolution = '720p'
    return probe
  }
  if (
    typeof src.resolution === 'string' &&
    (RESOLUTIONS as readonly string[]).includes(src.resolution)
  ) {
    probe.resolution = src.resolution as EstimatorResolution
  }
  return probe
}

// 合法化任意输入（JSON 解析失败、版本不兼容、负数、NaN、Infinity 等）为安全草稿。
export function normalizeDraft(raw: unknown): EstimatorDraft {
  if (!raw || typeof raw !== 'object') return createDefaultDraft()
  const src = raw as Record<string, unknown>
  return {
    promptTokens: asNonNegativeFinite(src.promptTokens),
    completionTokens: asNonNegativeFinite(
      src.completedTokens ?? src.completionTokens
    ),
    extras: normalizeExtras(src.extras),
    taskProbe: normalizeProbe(src.taskProbe),
  }
}

export function draftStorageKey(modelName: string): string {
  return KEY_PREFIX + modelName
}

export function loadDraft(modelName: string): EstimatorDraft {
  if (!modelName) return createDefaultDraft()
  try {
    const raw = window.localStorage.getItem(draftStorageKey(modelName))
    if (!raw) return createDefaultDraft()
    return normalizeDraft(JSON.parse(raw))
  } catch {
    return createDefaultDraft()
  }
}

export function saveDraft(modelName: string, draft: EstimatorDraft): void {
  if (!modelName) return
  try {
    window.localStorage.setItem(
      draftStorageKey(modelName),
      JSON.stringify(draft)
    )
  } catch {
    // 配额耗尽 / 隐私模式 / 禁用 localStorage 时静默放弃，草稿只是便利性预览
  }
}

// 构造本地请求 body，供 param("_task.*") 读取——使六档视频表达式能命中正确分支。
export function probeToRequestBody(
  probe: EstimatorTaskProbe
): Record<string, unknown> {
  return {
    _task: {
      has_video_input: probe.hasVideoInput,
      resolution: probe.resolution,
    },
  }
}

// 与后端口径一致的费用换算（不含用户组倍率，与估算器文案声明一致）：
//   USD = rawCost / 1,000,000；quota = USD × quotaPerUnit
// quotaPerUnit 非正时配额不可计算（返回 null），由调用方标记为不可用而非猜测。
export function convertRawCost(
  rawCost: number,
  quotaPerUnit: number
): { usd: number; quota: number | null } {
  const usd = Number.isFinite(rawCost) ? rawCost / 1_000_000 : 0
  const quota = quotaPerUnit > 0 ? usd * quotaPerUnit : null
  return { usd, quota }
}
