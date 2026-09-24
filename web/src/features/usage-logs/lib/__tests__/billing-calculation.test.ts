import { createInstance } from 'i18next'
import { expect, it } from 'vitest'

import {
  calculationFormula,
  recordedExpressionRows,
} from '../billing-calculation'

it('renders actual short circuit results without inventing values for the skipped branch', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en' })
  const rows = recordedExpressionRows(
    {
      version: 1,
      quota: 5,
      nodes: [
        { id: 1, op: 'literal', literal: 'true' },
        { id: 2, op: 'literal', literal: '5' },
        { id: 3, op: 'call' },
        { id: 4, op: 'if', args: [1, 2, 3] },
      ],
      values: [{ node: 4, value: '5' }],
    },
    i18n.t
  )
  expect(rows).toMatchObject([
    {
      id: 4,
      formula: 'Condition true → 5 (Other branch not executed)',
      result: '5',
    },
  ])
})

it('keeps integer and unary operations explicit and does not label a predicate definition as skipped', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en' })
  expect(calculationFormula('integer_division', ['5', '2'], i18n.t)).toBe(
    'Integer division(5 ÷ 2)'
  )
  expect(calculationFormula('unary:-', ['5'], i18n.t)).toBe('−5')
  const rows = recordedExpressionRows(
    {
      version: 1,
      quota: 6,
      nodes: [
        { id: 1, op: 'predicate' },
        { id: 2, op: 'map', args: [1] },
      ],
      values: [{ node: 2, value: '[2, 4]' }],
    },
    i18n.t
  )
  expect(rows[0].formula).toBe('map(#1)')
  expect(rows[0].result).toBe('[2, 4]')
})

it('marks skipped operands on each loop iteration instead of reusing a previous value', async () => {
  const i18n = createInstance()
  await i18n.init({ lng: 'en' })
  for (const [op, first, second] of [
    ['&&', 'true', 'false'],
    ['and', 'true', 'false'],
    ['||', 'false', 'true'],
    ['or', 'false', 'true'],
    ['??', 'null', '7'],
  ] as const) {
    const rows = recordedExpressionRows(
      {
        version: 1,
        quota: 1,
        nodes: [
          { id: 1, op: 'item' },
          { id: 2, op: 'call' },
          { id: 3, op, args: [1, 2] },
        ],
        values: [
          { node: 1, value: first },
          { node: 2, value: 'true' },
          { node: 3, value: 'true' },
          { node: 1, value: second },
          { node: 3, value: second },
        ],
      },
      i18n.t
    )
    expect(rows.at(-1)?.formula).toContain('Not executed')
  }
})
