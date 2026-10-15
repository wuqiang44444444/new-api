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

import {
  previewBillingExpressions,
  type BillingExprPreviewItem,
  type BillingExprPreviewItemResult,
} from '@/features/pricing/lib/billing-display-preview'

/** Read-only server evaluation. A changed sample hides its old result immediately. */
export function useBillingExpressionPreview(
  item: BillingExprPreviewItem | null
) {
  const requestKey = item ? JSON.stringify(item) : ''
  const [outcome, setOutcome] = useState<{
    key: string
    result: BillingExprPreviewItemResult
  } | null>(null)

  useEffect(() => {
    if (!requestKey) return
    let cancelled = false
    let sequence = 0
    const request = JSON.parse(requestKey) as BillingExprPreviewItem
    const calculate = async () => {
      const current = ++sequence
      try {
        const results = await previewBillingExpressions([request])
        if (!cancelled && current === sequence) {
          const result = results.find((entry) => entry.key === request.key)
          setOutcome({
            key: requestKey,
            result:
              result?.error || result?.evaluation
                ? result
                : { key: 'preview', error: 'Preview failed' },
          })
        }
      } catch {
        if (!cancelled && current === sequence) {
          setOutcome({
            key: requestKey,
            result: { key: 'preview', error: 'Preview failed' },
          })
        }
      }
    }
    const debounce = setTimeout(() => void calculate(), 200)
    // Current server time and settings also refresh while the preview stays open.
    const refresh = setInterval(() => void calculate(), 60_000)
    return () => {
      cancelled = true
      clearTimeout(debounce)
      clearInterval(refresh)
    }
  }, [requestKey])

  const result =
    requestKey && outcome?.key === requestKey ? outcome.result : null
  return { result, pending: Boolean(requestKey && !result) }
}
