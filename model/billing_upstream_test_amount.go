package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// upstreamTestAmount is the single test-amount projection used by the upstream
// summary, details, exports and daily/weekly views. It restores the recorded
// test price only from frozen historical facts; it never reads current prices,
// never divides by the tester's group or contract ratios, and never back-solves
// a rounded post-discount fee.
type upstreamTestAmount struct {
	known            bool
	pending          bool
	reason           string
	replayed         bool
	recordedOriginal bool
	recorded         int64
	original         decimal.Decimal
}

// upstreamTestAmountFor resolves one native channel test row. Priority:
// versioned test_pricing evidence → frozen fixed price → historical rule replay
// or a recorded undiscounted engine result. Only concrete unresolved inputs stay pending.
func upstreamTestAmountFor(log billingReconciliationLog, parsed parsedBillingReconciliationLog) upstreamTestAmount {
	if log.Quota < 0 {
		return upstreamTestAmount{pending: true, reason: "invalid_pricing_record"}
	}
	quota := int64(log.Quota)
	if parsed.testPricing != nil {
		record := parsed.testPricing
		if record.Version == 1 && (record.Mode == upstreamTestPricingModeRatio || record.Mode == upstreamTestPricingModeFixedPrice || record.Mode == upstreamTestPricingModeTieredExpr) && record.Status == upstreamTestPricingStatusSettled && record.OriginalQuota != nil && *record.OriginalQuota >= 0 && *record.OriginalQuota <= common.MaxQuota {
			return upstreamTestAmount{known: true, original: decimal.NewFromInt(*record.OriginalQuota)}
		}
		if record.Version == 0 && record.Status == upstreamTestPricingStatusSettled {
			return historicalChannelTestAmount(log, parsed)
		}
		if record.Status == upstreamTestPricingStatusEstimated {
			return upstreamTestAmount{pending: true, reason: "estimated_usage"}
		}
		return upstreamTestAmount{pending: true, reason: "invalid_pricing_record"}
	}
	// Replay only required frozen inputs. A successful undiscounted engine
	// result is itself an original amount, even without a duplicate field.
	if parsed.hasExpression {
		return historicalChannelTestAmount(log, parsed)
	}
	if parsed.modelPrice != nil && *parsed.modelPrice > 0 {
		if recompute, clamp := common.QuotaRoundChecked(*parsed.modelPrice * common.QuotaPerUnit); clamp == nil && int64(recompute) == quota {
			return upstreamTestAmount{known: true, original: decimal.NewFromInt(quota)}
		}
		return upstreamTestAmount{pending: true, reason: "fixed_price_mismatch"}
	}
	// Check the historical quote and then price recorded text/cache usage
	// using its frozen rates; do not declare the simplified quote a full cost.
	if parsed.modelRatio != nil && parsed.completionRatio != nil {
		return historicalChannelTestAmount(log, parsed)
	}
	return upstreamTestAmount{pending: true, reason: "missing_price_fields"}
}

// accumulateUpstreamTestAmount applies the test-amount projection to one
// upstream aggregate item. Known amounts join the original/reference decimals
// under the channel-month coefficient; pending rows keep usage, count as
// without-amount, and block amount completeness like any other unknown money.
func accumulateUpstreamTestAmount(item *ProviderBillingPlatformSummary, log billingReconciliationLog, parsed parsedBillingReconciliationLog, coefficient decimal.Decimal) {
	amount := upstreamTestAmountFor(log, parsed)
	quality := ensureBillingReconciliationQuality(&item.DataQuality)
	if amount.known {
		item.testMoneyRows++
		quality.TestPricedRows++
		if amount.replayed {
			quality.TestRecomputedRows++
		}
		if amount.recordedOriginal {
			quality.TestRecordedOriginalRows++
		}
		item.originalQuota = item.originalQuota.Add(amount.original)
		item.referenceQuota = item.referenceQuota.Add(UpstreamReferenceQuota(amount.original, coefficient))
		item.originalQuotaKnown = true
		return
	}
	quality.UsageWithoutAmountRows++
	if quality.TestAmountPendingReasons == nil {
		quality.TestAmountPendingReasons = make(map[string]int64)
	}
	quality.TestAmountPendingReasons[amount.reason]++
	if parsed.billingMode == BillingReconciliationModeToken {
		if parsed.cacheReadEvidence == upstreamCacheLegacyUnknown {
			quality.LegacyTestCacheReadRows++
		}
		if parsed.cacheWriteEvidence == upstreamCacheLegacyUnknown {
			quality.LegacyTestCacheWriteRows++
		}
	}
	item.EstimateReasons = mergeBillingEstimateReasons(item.EstimateReasons, BillingEstimateTestAmountPending)
	item.originalComplete = false
}

// UpstreamTestPricingProjection is the display record of how one test row was
// priced: the mode and whether the recorded fee was settled from actual usage
// or only estimated. Detail rows and exports show it; it never claims the
// upstream model identity or supplier pricing.
type UpstreamTestPricingProjection struct {
	Basis           string `json:"basis,omitempty"`
	Replayed        bool   `json:"replayed,omitempty"`
	RecordedQuota   *int64 `json:"recorded_quota,omitempty"`
	RecomputedQuota *int64 `json:"recomputed_quota,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Mode            string `json:"mode"`
	Status          string `json:"status"`
}

// upstreamTestPricingProjection classifies the pricing basis of one test row
// for display. Persisted evidence wins; otherwise the frozen price fields name
// the mode. Display status is priced / estimated / pending so historical rows
// never claim a settled fee the evidence cannot prove.
func upstreamTestPricingProjection(parsed parsedBillingReconciliationLog, amount upstreamTestAmount) UpstreamTestPricingProjection {
	mode := ""
	switch {
	case parsed.hasExpression:
		mode = upstreamTestPricingModeTieredExpr
	case parsed.modelPrice != nil && *parsed.modelPrice > 0:
		mode = upstreamTestPricingModeFixedPrice
	case parsed.modelRatio != nil && *parsed.modelRatio > 0:
		mode = upstreamTestPricingModeRatio
	}
	if parsed.testPricing != nil && parsed.testPricing.Mode != "" {
		mode = parsed.testPricing.Mode
	}

	if amount.known {
		projection := UpstreamTestPricingProjection{Mode: mode, Status: "priced", Replayed: amount.replayed}
		if amount.recordedOriginal {
			projection.Basis = "recorded_expression_result"
		}
		if amount.replayed {
			projection.Basis = "historical_replay"
			projection.RecordedQuota = &amount.recorded
			projection.RecomputedQuota = billingStatementOriginalQuota(amount.original)
		}
		return projection
	}
	if amount.reason == "estimated_usage" {
		return UpstreamTestPricingProjection{Mode: mode, Status: "estimated", Reason: amount.reason}
	}
	return UpstreamTestPricingProjection{Mode: mode, Status: "pending", Reason: amount.reason}
}
