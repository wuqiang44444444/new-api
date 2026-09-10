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
import { useCallback, useEffect, useRef, useState } from 'react'

import { previewBillingExpressions } from '@/features/pricing/lib/billing-display-preview'
import type { BillingDisplayProjection } from '@/features/pricing/types'

/**
 * 管理页表达式投影的批量获取缓存：对当页表达式做 describe（无 sample），
 * 去重后分批请求。读取是同步的；投影未到达或请求失败时返回 null，调用方
 * 按「暂无法展开」处理，不本地猜价。
 */
export function useBillingDisplayCache(
  expressions: string[]
): (expression: string) => BillingDisplayProjection | null {
  const cacheRef = useRef(new Map<string, BillingDisplayProjection>())
  const [version, setVersion] = useState(0)
  const joined = JSON.stringify(expressions)

  useEffect(() => {
    const list: string[] = JSON.parse(joined) as string[]
    const active = new Set(list)
    for (const expression of cacheRef.current.keys()) {
      if (!active.has(expression)) cacheRef.current.delete(expression)
    }
    const missing = [
      ...new Set(list.filter((expr) => expr && !cacheRef.current.has(expr))),
    ]
    if (missing.length === 0) return
    let cancelled = false
    const chunkSize = 16
    const requests = []
    for (let start = 0; start < missing.length; start += chunkSize) {
      const chunk = missing.slice(start, start + chunkSize)
      requests.push(
        previewBillingExpressions(
          chunk.map((expr, index) => ({
            key: String(start + index),
            expression: expr,
          }))
        )
      )
    }
    Promise.allSettled(requests)
      .then((batches) => {
        if (cancelled) return
        for (const batch of batches) {
          if (batch.status !== 'fulfilled') continue
          for (const result of batch.value) {
            const expr = missing[Number(result.key)]
            if (expr == null) continue
            if (result.projection) cacheRef.current.set(expr, result.projection)
          }
        }
        setVersion((v) => v + 1)
      })
      .catch(() => {
        setVersion((v) => v + 1)
      })
    return () => {
      cancelled = true
    }
  }, [joined])

  return useCallback(
    (expression: string) => {
      void version
      if (!expression) return null
      return cacheRef.current.get(expression) ?? null
    },
    [version]
  )
}
