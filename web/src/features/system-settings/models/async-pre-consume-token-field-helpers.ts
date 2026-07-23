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
// 异步预扣上界字段的纯函数：草稿分类与受控值格式化。
// 独立为 .ts（非 .tsx）有两个目的：
//  1) 满足 react/only-export-components——组件文件只导出组件；
//  2) 让单测可直接导入纯函数，无需引入 UI（Input/cn）依赖。

export type PreConsumeDraftKind = 'empty' | 'valid' | 'invalid'

// Keep the browser contract aligned with billing_setting.MaxTaskPreConsumeTokens.
export const MAX_TASK_PRE_CONSUME_TOKENS = 1_073_741_823

// 预扣上界草稿分类：
//  - empty   → 清空（合法；保存时 persistPricingData 从映射删除该 model，与方案 4.1 一致）
//  - valid   → 正整数（合法）
//  - invalid → 0 / 负数 / 小数 / 非数字（非法；不回写受控值，仅提示并阻止提交）
export function classifyPreConsumeDraft(raw: string): PreConsumeDraftKind {
  const trimmed = raw.trim()
  if (trimmed === '') return 'empty'
  const n = Number(trimmed)
  if (Number.isSafeInteger(n) && n >= 1 && n <= MAX_TASK_PRE_CONSUME_TOKENS) {
    return 'valid'
  }
  return 'invalid'
}

export function formatPreConsumeValue(value?: number): string {
  return value && value > 0 ? String(value) : ''
}
