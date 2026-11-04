package model

import "github.com/QuantumNous/new-api/pkg/billingexpr"

// A fixed/token pricing tree needs the settled branch, whereas task meters
// retain their frozen unit (for example seconds). The engine's legacy token
// unit alone cannot reclassify task usage as tokens.
func BillingStatementSettledExpressionMode(expression string, usageUnits map[string]string, unit any) string {
	if billingexpr.UsesFixedPricing(expression) {
		if unit == billingexpr.BillingUnitRequest || unit == "request" {
			return BillingReconciliationModePerCall
		}
		if unit == billingexpr.BillingUnitToken || unit == "token" {
			return BillingReconciliationModeToken
		}
	}
	return BillingStatementExpressionMode(expression, usageUnits)
}
