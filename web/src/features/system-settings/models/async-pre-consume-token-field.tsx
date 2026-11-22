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
import { useEffect, useRef, useState } from 'react'
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
  // 保留原始输入，区分主动清空与不合法的替换。
  const [draft, setDraft] = useState(() => formatPreConsumeValue(value))
  const [focused, setFocused] = useState(false)
  const [error, setError] = useState(false)
  const valueBeforeEdit = useRef(value)

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
      // 回退本次输入前的值，避免保留输入过程中的空值或数字前缀。
      setError(true)
      onChange?.(valueBeforeEdit.current ?? 0)
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
            'Token budget used to calculate the pre-consume amount at task creation, not a final charge cap. Settlement refunds or charges the difference using verified usage. Insufficient funds enter billing debt; missing usage is handled by the task protocol.'
          )}
        </p>
      </div>
      <Input
        inputMode='numeric'
        max={MAX_TASK_PRE_CONSUME_TOKENS}
        value={draft}
        placeholder='250000'
        data-billing-invalid={error || undefined}
        onFocus={(e) => {
          valueBeforeEdit.current = value
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
