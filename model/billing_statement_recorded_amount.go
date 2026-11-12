package model

import "github.com/shopspring/decimal"

const (
	StatementOriginalQuotaLogKey        = "statement_original_quota"
	StatementFundingAdjustmentFailedKey = "statement_funding_adjustment_failed"
)

const billingOriginalQuotaNotRecorded = "original_quota_not_recorded"

// recordedStatementOriginalQuota returns the original amount for a funding
// event. Historical events can use the booked amount only when both discount
// factors are explicitly known to be one.
func recordedStatementOriginalQuota(log billingReconciliationLog, parsed parsedBillingReconciliationLog) (decimal.Decimal, bool) {
	if log.Quota < 0 || parsed.fundingAdjustmentFailed {
		return decimal.Zero, false
	}
	if parsed.recordedOriginalQuota != nil {
		return decimal.NewFromInt(signedStatementEventQuota(*parsed.recordedOriginalQuota, log.Type == LogTypeRefund)), true
	}
	quota, ok := bookedStatementOriginalQuota(int64(log.Quota), log.Type == LogTypeRefund, parsed.discountRatio,
		billingContractApplicableState(parsed), parsed.contractDiscountRatio, parsed.unavailable || parsed.hasAuxiliaryCharge)
	return decimal.NewFromInt(quota), ok
}

// A booked amount proves the original only if both discount factors are explicitly neutral.
func bookedStatementOriginalQuota(quota int64, refund bool, group *float64, contract string, contractRatio *float64, blocked bool) (int64, bool) {
	if blocked || quota < 0 || group == nil || *group != 1 ||
		(contract != combinationContractApplicableNo &&
			(contract != combinationContractApplicableYes || contractRatio == nil || *contractRatio != 1)) {
		return 0, false
	}
	return signedStatementEventQuota(quota, refund), true
}

func signedStatementEventQuota(quota int64, refund bool) int64 {
	if refund {
		return -quota
	}
	return quota
}

// RecordedAmountsOnly hides unsupported amounts in already-published versions
// without changing the frozen projection or archived exports.
func (s *BillingCustomerStatement) RecordedAmountsOnly() {
	if s.RecordedOriginalAmounts {
		return
	}
	s.OriginalQuota, s.DiscountQuota = nil, nil
	for i := range s.Groups {
		group := &s.Groups[i]
		group.OriginalQuota, group.DiscountQuota = nil, nil
		for j := range group.Models {
			group.Models[j].OriginalQuota, group.Models[j].DiscountQuota = nil, nil
		}
	}
	for i := range s.DiscountCombinations {
		combo := &s.DiscountCombinations[i]
		combo.OriginalKnown, combo.OriginalQuota, combo.DiscountQuota = false, nil, nil
	}
}
