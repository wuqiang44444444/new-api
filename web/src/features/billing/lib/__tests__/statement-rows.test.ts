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

import type { BillingBreakdownItem, BillingStatementItem } from '../../types'
import {
  buildBillingStatementRows,
  countBillingBreakdownMismatches,
  countOrphanBillingBreakdownItems,
  selectReliableBillingBreakdownItems,
} from '../statement-rows'

const statementItems: BillingStatementItem[] = [
  {
    token_id: 1,
    token_name: '=unsafe-key',
    model_name: 'model-a',
    requests: 2,
    prompt_tokens: 1_000,
    completion_tokens: 200,
    total_tokens: 1_200,
    gross_quota: 320,
    refund_quota: 20,
    net_quota: 300,
    average_use_time_seconds: 2,
    stream_requests: 1,
    latest_request_timestamp: 20,
  },
  {
    token_id: 2,
    token_name: 'key-b',
    model_name: 'model-a',
    requests: 1,
    prompt_tokens: 600,
    completion_tokens: 100,
    total_tokens: 700,
    gross_quota: 150,
    refund_quota: 0,
    net_quota: 150,
    average_use_time_seconds: 5,
    stream_requests: 0,
    latest_request_timestamp: 30,
  },
]

const breakdownItems: BillingBreakdownItem[] = [
  {
    token_id: 1,
    token_name: '=unsafe-key',
    model_name: 'model-a',
    requests: 2,
    gross_quota: 320,
    unallocated_adjustment_quota: 20,
    cache: {
      hit_requests: 1,
      write_requests: 1,
      denominator_requests: 2,
      denominator_scope: 'all_settled_requests',
      read_tokens: 400,
      write_tokens: 200,
      write_tokens_5m: 150,
      write_tokens_1h: 50,
      hit_request_gross_quota: 120,
      hit_request_ratio: 0.5,
    },
    context: {
      threshold_tokens: 1_000,
      threshold_source: 'current_model_config',
      classified_requests: 1,
      unclassified_requests: 1,
      short_requests: 1,
      long_requests: 0,
      short_input_tokens: 900,
      long_input_tokens: 0,
      short_gross_quota: 120,
      long_gross_quota: 0,
      classification_coverage: 0.5,
    },
    billing_mode: {
      tiered_requests: 1,
      tiered_gross_quota: 200,
    },
  },
  {
    token_id: 2,
    token_name: 'key-b',
    model_name: 'model-a',
    requests: 1,
    gross_quota: 150,
    cache: {
      hit_requests: 1,
      write_requests: 0,
      denominator_requests: 1,
      denominator_scope: 'all_settled_requests',
      read_tokens: 100,
      write_tokens: 0,
      write_tokens_5m: 0,
      write_tokens_1h: 0,
      hit_request_gross_quota: 150,
      hit_request_ratio: 1,
    },
    context: {
      threshold_tokens: 2_000,
      threshold_source: 'current_model_config',
      classified_requests: 1,
      unclassified_requests: 0,
      short_requests: 0,
      long_requests: 1,
      short_input_tokens: 0,
      long_input_tokens: 2_100,
      short_gross_quota: 0,
      long_gross_quota: 150,
      classification_coverage: 1,
    },
  },
]

describe('billing statement rows', () => {
  test('joins detail rows by API key and model without changing settled amounts', () => {
    const rows = buildBillingStatementRows(
      statementItems,
      breakdownItems,
      'detail'
    )

    expect(rows).toHaveLength(2)
    expect(rows[0]?.tokenName).toBe('=unsafe-key')
    expect(rows[0]?.grossQuota).toBe(320)
    expect(rows[0]?.refundQuota).toBe(20)
    expect(rows[0]?.netQuota).toBe(300)
    expect(rows[0]?.cache?.write_tokens_5m).toBe(150)
    expect(rows[0]?.context?.short_input_tokens).toBe(900)
    expect(rows[0]?.unallocatedAdjustmentQuota).toBe(20)
    expect(rows[0]?.breakdownComplete).toBe(true)
  })

  test('aggregates model rows and recomputes denominators, coverage, and weighted latency', () => {
    const [row] = buildBillingStatementRows(
      statementItems,
      breakdownItems,
      'model'
    )

    expect(row).toBeDefined()
    expect(row?.requests).toBe(3)
    expect(row?.grossQuota).toBe(470)
    expect(row?.refundQuota).toBe(20)
    expect(row?.netQuota).toBe(450)
    expect(row?.averageUseTimeSeconds).toBe(3)
    expect(row?.cache?.denominator_requests).toBe(3)
    expect(row?.cache?.hit_request_ratio).toBe(2 / 3)
    expect(row?.context?.classification_coverage).toBe(2 / 3)
    expect(row?.context?.threshold_tokens).toBeUndefined()
    expect(row?.context?.long_input_tokens).toBe(2_100)
    expect(row?.billingMode?.tiered_gross_quota).toBe(200)
    expect(row?.unallocatedAdjustmentQuota).toBe(20)
  })

  test('does not create statement rows from unmatched breakdown data', () => {
    const orphan: BillingBreakdownItem = {
      token_id: 99,
      token_name: 'orphan',
      model_name: 'model-orphan',
      requests: 1,
      gross_quota: 10,
    }
    const withOrphan = [...breakdownItems, orphan]

    expect(
      buildBillingStatementRows(statementItems, withOrphan, 'detail')
    ).toHaveLength(2)
    expect(countOrphanBillingBreakdownItems(statementItems, withOrphan)).toBe(1)
    expect(countBillingBreakdownMismatches(statementItems, withOrphan)).toBe(1)
    expect(
      selectReliableBillingBreakdownItems(statementItems, withOrphan)
    ).toEqual(breakdownItems)
  })

  test('marks a row unavailable when statement and breakdown gross costs differ', () => {
    const changedBreakdown = structuredClone(breakdownItems)
    const firstBreakdown = changedBreakdown[0]
    expect(firstBreakdown).toBeDefined()
    if (!firstBreakdown) throw new Error('missing billing fixture')
    firstBreakdown.gross_quota = 319

    const [row] = buildBillingStatementRows(
      statementItems,
      changedBreakdown,
      'detail'
    )

    expect(row?.breakdownComplete).toBe(false)
    expect(row?.cache).toBeUndefined()
    expect(row?.context).toBeUndefined()
    expect(
      countBillingBreakdownMismatches(statementItems, changedBreakdown)
    ).toBe(1)
  })

  test('keeps a consume-only async adjustment reconcilable without adding a request', () => {
    const adjustmentStatement: BillingStatementItem = {
      ...statementItems[0],
      requests: 0,
      prompt_tokens: 0,
      completion_tokens: 10,
      total_tokens: 10,
      gross_quota: 200,
      refund_quota: 0,
      net_quota: 200,
      stream_requests: 0,
    }
    const adjustmentBreakdown: BillingBreakdownItem = {
      token_id: adjustmentStatement.token_id,
      token_name: adjustmentStatement.token_name,
      model_name: adjustmentStatement.model_name,
      requests: 0,
      gross_quota: 200,
      unallocated_adjustment_quota: 200,
    }

    const [row] = buildBillingStatementRows(
      [adjustmentStatement],
      [adjustmentBreakdown],
      'detail'
    )

    expect(row?.breakdownComplete).toBe(true)
    expect(row?.requests).toBe(0)
    expect(row?.unallocatedAdjustmentQuota).toBe(200)
    expect(
      countBillingBreakdownMismatches(
        [adjustmentStatement],
        [adjustmentBreakdown]
      )
    ).toBe(0)
  })

  test('excludes rows with unavailable breakdown metadata from customer summaries', () => {
    const unavailableBreakdown = structuredClone(breakdownItems)
    const firstBreakdown = unavailableBreakdown[0]
    expect(firstBreakdown).toBeDefined()
    if (!firstBreakdown) throw new Error('missing billing fixture')
    firstBreakdown.data_quality = { unavailable_requests: 1 }

    const rows = buildBillingStatementRows(
      statementItems,
      unavailableBreakdown,
      'detail'
    )

    expect(rows[0]?.breakdownComplete).toBe(false)
    expect(rows[0]?.cache).toBeUndefined()
    expect(
      selectReliableBillingBreakdownItems(statementItems, unavailableBreakdown)
    ).toEqual([unavailableBreakdown[1]])
    expect(
      countBillingBreakdownMismatches(statementItems, unavailableBreakdown)
    ).toBe(1)
  })
})
