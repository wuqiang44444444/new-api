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
import type { TFunction } from 'i18next'
import { describe, expect, test } from 'vitest'

import { escapeCsvCell } from '../../lib'
import { buildBillingStatementCsv } from '../statement-csv'
import type { BillingStatementRow } from '../statement-rows'

describe('billing CSV safety', () => {
  test('neutralizes spreadsheet formulas in user-controlled strings', () => {
    expect(escapeCsvCell('=1+1')).toBe("'=1+1")
    expect(escapeCsvCell(' +SUM(A1:A2)')).toBe("' +SUM(A1:A2)")
    expect(escapeCsvCell('@cmd')).toBe("'@cmd")
    expect(escapeCsvCell('-2+3')).toBe("'-2+3")
  })

  test('keeps numeric values numeric and preserves CSV quoting', () => {
    expect(escapeCsvCell(-2)).toBe('-2')
    expect(escapeCsvCell('safe, value')).toBe('"safe, value"')
    expect(escapeCsvCell('"quoted"')).toBe('"""quoted"""')
  })

  test('exports stable raw quota and ratio columns alongside display values', () => {
    const row: BillingStatementRow = {
      id: 'detail:1:model-a',
      tokenId: 1,
      tokenName: 'customer-key',
      modelName: 'model-a',
      requests: 2,
      promptTokens: 1_000,
      completionTokens: 200,
      totalTokens: 1_200,
      grossQuota: 987_654_321,
      refundQuota: 123_456_789,
      netQuota: 864_197_532,
      averageUseTimeSeconds: 2,
      streamRequests: 1,
      latestRequestTimestamp: 20,
      breakdownComplete: true,
      unallocatedAdjustmentQuota: 200,
      statementSaturated: false,
    }
    const t = ((key: string) => key) as TFunction

    const csv = buildBillingStatementCsv({
      rows: [row],
      totalQuota: row.netQuota,
      locale: 'en-US',
      period: { start_timestamp: 1, end_timestamp: 2 },
      t,
      keyLabel: () => '=unsafe-key',
    })

    const [headers, values] = csv.split('\n')
    expect(headers).toContain('gross_quota_raw')
    expect(headers).toContain('refund_quota_raw')
    expect(headers).toContain('net_quota_raw')
    expect(headers).toContain('unallocated_adjustment_quota_raw')
    expect(headers).toContain('statement_saturated')
    expect(headers).toContain('cost_share_raw')
    expect(values).toContain("'=unsafe-key")
    expect(values).toContain('987654321')
    expect(values).toContain('123456789')
    expect(values).toContain('864197532')
    expect(values).toContain('Partial')
    expect(values?.endsWith(',1')).toBe(true)
  })
})
