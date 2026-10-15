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
import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { useBillingExpressionPreview } from '../use-billing-expression-preview'

afterEach(() => vi.useRealTimers())

it('refreshes the authoritative rate while mounted and stops requests after unmount', async () => {
  vi.useFakeTimers()
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          key: 'preview',
          evaluation: { raw_cost_usd: 1, usd_exchange_rate: { rate: 7 } },
        },
      ],
    },
  })
  const view = renderHook(() =>
    useBillingExpressionPreview({
      key: 'preview',
      expression: 'tier("base", 7 / usd_exchange_rate())',
      sample: { usage_semantic: 'normalized', normalized_usage: {} },
    })
  )
  await act(() => vi.advanceTimersByTimeAsync(200))
  expect(view.result.current.result?.evaluation?.usd_exchange_rate?.rate).toBe(
    7
  )
  post.mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          key: 'preview',
          evaluation: { raw_cost_usd: 0.7, usd_exchange_rate: { rate: 10 } },
        },
      ],
    },
  })
  await act(() => vi.advanceTimersByTimeAsync(60000))
  expect(view.result.current.result?.evaluation?.usd_exchange_rate?.rate).toBe(
    10
  )
  expect(view.result.current.result?.evaluation?.raw_cost_usd).toBe(0.7)
  view.unmount()
  await act(() => vi.advanceTimersByTimeAsync(60000))
  expect(post).toHaveBeenCalledTimes(2)
})
