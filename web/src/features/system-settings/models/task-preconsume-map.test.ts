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

import { applyTaskPreConsumeTokenChanges } from './task-preconsume-map'

describe('task pre-consume option map updates', () => {
  test('adds or updates only the selected model', () => {
    const values = { existing: 100, untouched: 200 }
    applyTaskPreConsumeTokenChanges(values, ['existing'], 300)
    expect(values).toEqual({ existing: 300, untouched: 200 })
  })

  test('clears only the selected model', () => {
    const values = { selected: 100, untouched: 200 }
    applyTaskPreConsumeTokenChanges(values, ['selected'])
    expect(values).toEqual({ untouched: 200 })
  })

  test('batch copy applies the same upper bound to every target', () => {
    const values = { source: 100, targetA: 10, untouched: 20 }
    applyTaskPreConsumeTokenChanges(
      values,
      ['source', 'targetA', 'targetB'],
      520000
    )
    expect(values).toEqual({
      source: 520000,
      targetA: 520000,
      targetB: 520000,
      untouched: 20,
    })
  })
})
