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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import {
  classifyPreConsumeDraft,
  formatPreConsumeValue,
  MAX_TASK_PRE_CONSUME_TOKENS,
} from './async-pre-consume-token-field-helpers'

// 真实计费配置：异步任务创建时预扣的可计费 token 上界，落库为
// task_billing_setting.preconsume_tokens[model]。与费用试算器严格分离——
// 试算器仅做本地预览，绝不写入本字段（避免试算值污染真实预扣）。
// 独立成文件以缩小上游 tiered-pricing-editor.tsx 的改动面（最小入侵）。

export type AsyncPreConsumeTokenFieldProps = {
  value?: number
  onChange?: (next: number) => void
}

export function AsyncPreConsumeTokenField({
  value,
  onChange,
}: AsyncPreConsumeTokenFieldProps) {
  const { t } = useTranslation()
  // 字符串草稿：保留用户原始输入（含非法字符），仅在分类为合法时才回写受控 number 值。
  // 这样可区分「清空（空串，合法）」与「0/负数/小数（非法）」——原先用 number 无法区分，
  // 导致 -1/1.9 被静默 floor 或归零，保存时连带删除既有预扣配置（见验收要求 9.2.6）。
  const [draft, setDraft] = useState(() => formatPreConsumeValue(value))
  const [focused, setFocused] = useState(false)
  const [error, setError] = useState(false)

  // 非聚焦时跟随受控值（模型切换 / 外部重置时回填到合法值）
  useEffect(() => {
    if (!focused) {
      setDraft(formatPreConsumeValue(value))
      setError(false)
    }
  }, [focused, value])

  const handleChange = (raw: string) => {
    setDraft(raw)
    const kind = classifyPreConsumeDraft(raw)
    if (kind === 'empty') {
      setError(false)
      // 0 为「清空」哨兵：保存时 persistPricingData 从映射删除该 model
      onChange?.(0)
    } else if (kind === 'valid') {
      setError(false)
      onChange?.(Number(raw.trim()))
    } else {
      // 0 / 负数 / 小数 / 非数字：绝不回写受控值，避免静默删除既有预扣配置。
      // 仅在字段内提示；离开字段时回退到上一个合法值，确保无法把非法值留在字段里提交。
      setError(true)
    }
  }

  const handleBlur = (raw: string) => {
    setFocused(false)
    if (classifyPreConsumeDraft(raw) === 'invalid') {
      setDraft(formatPreConsumeValue(value))
    }
    setError(false)
  }

  return (
    <div className='bg-muted/30 border-primary/40 space-y-2 rounded-md border p-3'>
      <div className='space-y-1'>
        <h4 className='text-sm font-medium'>
          {t('Async task pre-consume token upper bound')}
        </h4>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Billable token upper bound frozen at async task creation. Settlement uses completion_tokens, then total_tokens; missing usage keeps the pre-consume amount, while an overrun enters billing debt instead of charging beyond the bound.'
          )}
        </p>
      </div>
      <Input
        inputMode='numeric'
        max={MAX_TASK_PRE_CONSUME_TOKENS}
        value={draft}
        placeholder='250000'
        onFocus={(e) => {
          setFocused(true)
          e.currentTarget.select()
        }}
        onChange={(e) => handleChange(e.target.value)}
        onBlur={(e) => handleBlur(e.currentTarget.value)}
        className={cn(
          'w-48',
          error && 'border-destructive focus-visible:ring-destructive'
        )}
      />
      {error ? (
        <p className='text-destructive text-xs'>
          {t(
            'Must be an integer from 1 to {{max}}; leave empty for synchronous models.',
            { max: MAX_TASK_PRE_CONSUME_TOKENS.toLocaleString() }
          )}
        </p>
      ) : (
        <p className='text-muted-foreground text-xs'>
          {t('Enter 1 to {{max}}; leave empty for synchronous models.', {
            max: MAX_TASK_PRE_CONSUME_TOKENS.toLocaleString(),
          })}
        </p>
      )}
    </div>
  )
}
