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
	// BillingEstimateTestAmountPending marks channel-test rows whose recorded
	// fee cannot be explained from frozen facts. Usage stays; the row counts
	// under usage_without_amount_rows carrying this reason.
	BillingEstimateTestAmountPending = "test_amount_pending"
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

// An auxiliary charge blocks the official-price restore by itself; it is not
// a missing price. Rows carrying it are counted under the auxiliary-charge
// reason instead of the generic missing-price bucket, so messages name the
// actual blocker. A row with both reason classes is counted once per class.
func accumulateBillingEstimateReasonQuality(quality *BillingReconciliationDataQuality, reasons []string) {
	if quality == nil || len(reasons) == 0 {
		return
	}
	priceEvidence := false
	for _, reason := range reasons {
		if reason == BillingEstimateAuxiliaryCharge {
			quality.AuxiliaryChargeRows++
		} else if reason != BillingEstimateTestAmountPending {
			priceEvidence = true
		}
	}
	if priceEvidence {
		quality.MissingHistoricalPriceRows++
	}
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
