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
import { describe, expect, it } from 'vitest'

import { billingDisplayFixture } from '@/features/pricing/__tests__/billing-display-fixtures'
import fixtures from '@/features/pricing/__tests__/billing-display-fixtures.json'

import type { LogOtherData } from '../../types'
import { getTieredBillingSummary } from '../format'

const expression = Object.keys(fixtures).find(
  (key) => key.includes('weekday(') && key.endsWith('/ 6.9')
)
const projection = billingDisplayFixture(expression)
if (!projection?.rules?.[0]) throw new Error('Missing time-price fixture')
const rule = projection.rules[0]
const log: LogOtherData = {
  billing_mode: 'tiered_expr',
  matched_tier: 'base',
  billing_display: projection,
  cache_tokens: 200000,
}

describe('historical time-price summaries', () => {
  it.each([false, true])(
    'uses recorded condition match %s for all unit prices',
    (matched) => {
      const summary = getTieredBillingSummary({
        ...log,
        request_rules: [{ cond: rule.text, multiplier: 2, matched }],
      })
      expect(
        summary?.priceEntries.find((entry) => entry.field === 'inputPrice')
          ?.price
      ).toBeCloseTo((matched ? 3 : 1.5) / 6.9, 12)
      expect(
        summary?.priceEntries.find((entry) => entry.field === 'outputPrice')
          ?.price
      ).toBeCloseTo((matched ? 9 : 4.5) / 6.9, 12)
      expect(
        summary?.priceEntries.find((entry) => entry.field === 'cacheReadPrice')
          ?.price
      ).toBeCloseTo((matched ? 0.1 : 0.05) / 6.9, 12)
      expect(projection.tiers?.[0].unit_prices.p).toBeCloseTo(1.5 / 6.9, 12)
    }
  )

  it.each([
    { name: 'missing', traces: undefined },
    { name: 'empty', traces: [] },
    {
      name: 'unrelated',
      traces: [{ cond: 'unrelated', multiplier: 2, matched: true }],
    },
  ])(
    'does not guess prices with $name historical traces',
    ({ traces: request_rules }) => {
      expect(
        getTieredBillingSummary({ ...log, request_rules })?.priceEntries
      ).toEqual([])
    }
  )
})

it('does not hide explicitly free time-price branches', () => {
  const freeProjection = {
    ...projection,
    rules: [{ ...rule, multiplier: 0 }],
    scenarios: [
      { matched: true, tiers: [{ label: 'base', unit_prices: { p: 0 } }] },
    ],
  }
  const summary = getTieredBillingSummary({
    ...log,
    billing_display: freeProjection,
    request_rules: [{ cond: rule.text, matched: true, multiplier: 0 }],
  })
  expect(summary?.priceEntries).toContainEqual({
    field: 'inputPrice',
    shortLabel: 'Input',
    price: 0,
    unit: 'token',
  })
})
