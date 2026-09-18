package model

import (
	"github.com/shopspring/decimal"
	"slices"
)

// Stable, customer-safe reasons for a missing estimate. These describe the
// settled evidence only; they never read current prices or change money.
const (
	BillingEstimateMissingContract  = "missing_contract"
	BillingEstimateMissingGroup     = "missing_group"
	BillingEstimateInvalidFacts     = "invalid_facts"
	BillingEstimateAuxiliaryCharge  = "auxiliary_charge"
	BillingEstimateAmountOutOfRange = "amount_out_of_range"
	BillingEstimateCombinationLimit = "combination_limit"
)

func billingStatementEstimateReasons(parsed parsedBillingReconciliationLog) []string {
	if parsed.unavailable {
		return []string{BillingEstimateInvalidFacts}
	}
	var reasons []string
	if parsed.discountRatio == nil || *parsed.discountRatio <= 0 {
		reasons = append(reasons, BillingEstimateMissingGroup)
	}
	if billingContractApplicableState(parsed) == combinationContractApplicableUnknown {
		reasons = append(reasons, BillingEstimateMissingContract)
	}
	if parsed.hasAuxiliaryCharge {
		reasons = append(reasons, BillingEstimateAuxiliaryCharge)
	}
	slices.Sort(reasons)
	return reasons
}

// Preserve all blockers in mixed aggregates, with deterministic output for
// frozen statements, comparisons and CSVs.
func mergeBillingEstimateReasons(target []string, source ...string) []string {
	for _, reason := range source {
		if !slices.Contains(target, reason) {
			target = append(target, reason)
		}
	}
	slices.Sort(target)
	return target
}

// BillingStatementEstimatedSavings uses only the displayed frozen original
// estimate and settled net. Decimal subtraction avoids signed integer overflow.
func BillingStatementEstimatedSavings(original *int64, net int64) *int64 {
	if original == nil {
		return nil
	}
	return billingStatementOriginalQuota(decimal.NewFromInt(*original).Sub(decimal.NewFromInt(net)))
}

func finalizeBillingModelSavings(summary *BillingReconciliationModelSummary) {
	summary.DiscountQuota = BillingStatementEstimatedSavings(summary.OriginalQuota, summary.Usage.NetQuota)
	if summary.OriginalQuota != nil && summary.DiscountQuota == nil {
		summary.EstimateReasons = mergeBillingEstimateReasons(summary.EstimateReasons, BillingEstimateAmountOutOfRange)
	}
}
