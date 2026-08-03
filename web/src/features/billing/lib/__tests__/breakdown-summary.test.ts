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
import { describe, expect, test } from 'vitest'

import { summarizeBillingBreakdownItems } from '../../lib'
import type { BillingBreakdownItem } from '../../types'

describe('billing breakdown summary', () => {
  test('keeps overlapping dimensions while recomputing cache hit ratio', () => {
    const items: BillingBreakdownItem[] = [
      {
        token_id: 1,
        token_name: 'key',
        model_name: 'model-a',
        requests: 2,
        gross_quota: 300,
        unallocated_adjustment_quota: 25,
        cache: {
          hit_requests: 1,
          write_requests: 2,
          denominator_requests: 2,
          denominator_scope: 'all_settled_requests',
          read_tokens: 100,
          write_tokens: 600,
          write_tokens_5m: 200,
          write_tokens_1h: 200,
          hit_request_gross_quota: 100,
          hit_request_ratio: 0.5,
        },
        context: {
          threshold_tokens: 1000,
          threshold_source: 'current_model_config',
          classified_requests: 2,
          unclassified_requests: 1,
          short_requests: 1,
          long_requests: 1,
          short_input_tokens: 800,
          long_input_tokens: 1200,
          short_gross_quota: 100,
          long_gross_quota: 200,
          classification_coverage: 2 / 3,
        },
        billing_mode: {
          tiered_requests: 1,
          tiered_gross_quota: 100,
        },
      },
      {
        token_id: 2,
        token_name: 'key-2',
        model_name: 'model-b',
        requests: 1,
        gross_quota: 50,
        cache: {
          hit_requests: 1,
          write_requests: 0,
          denominator_requests: 1,
          denominator_scope: 'all_settled_requests',
          read_tokens: 50,
          write_tokens: 0,
          write_tokens_5m: 0,
          write_tokens_1h: 0,
          hit_request_gross_quota: 50,
          hit_request_ratio: 1,
        },
      },
    ]

    const summary = summarizeBillingBreakdownItems(items)

    expect(summary.requests).toBe(3)
    expect(summary.gross_quota).toBe(350)
    expect(summary.unallocated_adjustment_quota).toBe(25)
    expect(summary.cache?.hit_requests).toBe(2)
    expect(summary.cache?.read_tokens).toBe(150)
    expect(summary.cache?.write_tokens_5m).toBe(200)
    expect(summary.cache?.denominator_requests).toBe(3)
    expect(summary.cache?.hit_request_ratio).toBe(2 / 3)
    expect(summary.context?.long_requests).toBe(1)
    expect(summary.context?.long_input_tokens).toBe(1200)
    expect(summary.context?.unclassified_requests).toBe(1)
    expect(summary.context?.classification_coverage).toBe(2 / 3)
    expect(summary.billing_mode?.tiered_gross_quota).toBe(100)
  })

  test('leaves optional dimensions absent when no log can be classified', () => {
    const summary = summarizeBillingBreakdownItems([
      {
        token_id: 1,
        token_name: 'key',
        model_name: 'unconfigured-model',
        requests: 1,
        gross_quota: 10,
      },
    ])

    expect(summary.cache).toBeUndefined()
    expect(summary.context).toBeUndefined()
    expect(summary.billing_mode).toBeUndefined()
  })
})
