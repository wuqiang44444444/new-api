import type { BillingDisplayProjection } from '../types'
import fixtures from './billing-display-fixtures.json'

// Captured from billingexpr.DisplayProjectionFor; no frontend expression parser.
export function billingDisplayFixture(expression?: string): BillingDisplayProjection | undefined {
 return (fixtures as unknown as Record<string, BillingDisplayProjection>)[expression || '']
}
