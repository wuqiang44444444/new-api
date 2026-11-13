package model

import "math"

// The upstream usage view counts only the units used by the frozen pricing
// mode. Actual delivery counts remain independent observations in the report.
func setUpstreamUsageMeter(metrics *UsageAnalyticsMetrics, mode string, imageCount int64, imageKnown, isTest, settled bool, calls int64) {
	if mode == BillingReconciliationModeToken {
		metrics.BillingTokensApplicable = true
	} else {
		metrics.InputTokens, metrics.OutputTokens = 0, 0
		metrics.CacheReadTokens, metrics.CacheWriteTokens = 0, 0
		metrics.RowsMissingTokens = 0
		metrics.TokenDetails = nil
	}
	if mode == customerUsagePerImage {
		var count *int64
		if imageKnown {
			count = &imageCount
		}
		metrics.BillingImageTotals.observe(count, true, isTest)
	}
	if mode == BillingReconciliationModeUnknown && settled {
		metrics.BillingMeterUnknownRows += calls
	}
}

// Aggregate the pricing inputs rather than the raw token columns. Context
// length (len) is used only for tier selection and is never a second meter.
func setUpstreamExpressionTokenMeter(metrics *UsageAnalyticsMetrics, replay *FrozenChargeReplay, calls int64) {
	metrics.InputTokens, metrics.OutputTokens = 0, 0
	metrics.CacheReadTokens, metrics.CacheWriteTokens = 0, 0
	metrics.TokenDetails = nil
	metrics.RowsMissingTokens = 0
	if replay == nil || len(replay.TokenInputs) == 0 {
		metrics.RowsMissingTokens = max(calls, 1)
		return
	}
	for key, value := range replay.TokenInputs {
		if value < 0 || value >= float64(math.MaxInt64) || math.Trunc(value) != value {
			metrics.RowsMissingTokens = max(calls, 1)
			continue
		}
		count := int64(value)
		switch key {
		case "p", "ai", "img", "cr", "img_cr", "cc", "cc1h":
			addBillingStatementValue(&metrics.InputTokens, count)
		case "c", "ao", "img_o":
			addBillingStatementValue(&metrics.OutputTokens, count)
		}
		switch key {
		case "cr", "img_cr":
			addBillingStatementValue(&metrics.CacheReadTokens, count)
		case "cc", "cc1h":
			addBillingStatementValue(&metrics.CacheWriteTokens, count)
		}
	}
}
