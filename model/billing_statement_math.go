package model

import "math"

func addBillingStatementValue(target *int64, value int64) bool {
	if value <= 0 {
		return false
	}
	if *target > math.MaxInt64-value {
		*target = math.MaxInt64
		return true
	}
	*target += value
	return false
}

func markBillingStatementSaturated(target **BillingStatementDataQuality) {
	if *target == nil {
		*target = &BillingStatementDataQuality{}
	}
	(*target).Saturated = true
}
