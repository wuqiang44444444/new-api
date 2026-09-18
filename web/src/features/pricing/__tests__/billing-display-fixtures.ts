import type { BillingDisplayProjection } from '../types'
import fixtures from './billing-display-fixtures.json'
import taskFixtures from './task-billing-display-fixtures.json'

// Captured from billingexpr.DisplayProjectionFor; no frontend expression parser.
export function billingDisplayFixture(
  expression?: string
): BillingDisplayProjection | undefined {
  return (fixtures as unknown as Record<string, BillingDisplayProjection>)[
    expression || ''
  ]
}

// Captured from billingexpr.TaskDisplayProjectionFor with each expression's
// declared field/unit context; no frontend expression parser.
export function taskBillingDisplayFixture(
  expression?: string
): BillingDisplayProjection | undefined {
  const entry = (
    taskFixtures as unknown as Record<
      string,
      { projection?: BillingDisplayProjection }
    >
  )[expression || '']
  return entry?.projection
}
