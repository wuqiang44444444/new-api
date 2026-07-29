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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

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
        cache: {
          hit_requests: 1,
          write_requests: 2,
          read_tokens: 100,
          write_tokens: 600,
          hit_request_gross_quota: 100,
          hit_request_ratio: 0.5,
        },
        context: {
          threshold_tokens: 1000,
          classified_requests: 2,
          short_requests: 1,
          long_requests: 1,
          short_gross_quota: 100,
          long_gross_quota: 200,
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
          read_tokens: 50,
          write_tokens: 0,
          hit_request_gross_quota: 50,
          hit_request_ratio: 1,
        },
      },
    ]

    const summary = summarizeBillingBreakdownItems(items)

    assert.equal(summary.requests, 3)
    assert.equal(summary.gross_quota, 350)
    assert.equal(summary.cache?.hit_requests, 2)
    assert.equal(summary.cache?.read_tokens, 150)
    assert.equal(summary.cache?.hit_request_ratio, 2 / 3)
    assert.equal(summary.context?.long_requests, 1)
    assert.equal(summary.billing_mode?.tiered_gross_quota, 100)
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

    assert.equal(summary.cache, undefined)
    assert.equal(summary.context, undefined)
    assert.equal(summary.billing_mode, undefined)
  })
})
