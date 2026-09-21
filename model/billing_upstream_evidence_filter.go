package model

import "errors"

// A filter selects evidence, never changes the price or repairs stored logs.
// The same classifier drives coverage, interactive pages and full exports.
func upstreamEvidenceCoverageCategory(row UpstreamBillingDetailItem) string {
	q := row.DataQuality
	if q == nil || q.Status == "complete" {
		return "complete"
	}
	if row.OriginalAmount == nil {
		return "amount_gap"
	}
	if q.CacheReadUnavailableRequests > 0 || q.CacheWriteUnavailableRequests > 0 || q.SecondsUnavailableRows > 0 || q.InputTokensUnavailableRequests > 0 {
		return "usage_gap"
	}
	return "other_gap"
}

func ValidateUpstreamEvidenceFilter(filter string) error {
	_, valid := matchUpstreamEvidenceFilter(UpstreamBillingDetailItem{}, filter)
	if !valid {
		return errors.New("invalid evidence_filter")
	}
	return nil
}

func matchUpstreamEvidenceFilter(row UpstreamBillingDetailItem, filter string) (matches, valid bool) {
	q := row.DataQuality
	if q == nil {
		q = &BillingReconciliationDataQuality{}
	}
	switch filter {
	case "":
		return true, true
	case "incomplete":
		return upstreamEvidenceCoverageCategory(row) != "complete", true
	case "complete", "amount_gap", "usage_gap", "other_gap":
		return upstreamEvidenceCoverageCategory(row) == filter, true
	case "usage_without_amount_rows":
		return q.UsageWithoutAmountRows > 0, true
	case "auxiliary_charge_rows":
		return q.AuxiliaryChargeRows > 0, true
	case "legacy_test_cache_read_rows":
		return q.LegacyTestCacheReadRows > 0, true
	case "cache_read_unreported_requests":
		return q.CacheReadUnreportedRequests > 0, true
	case "seconds_task_link_missing_rows":
		return q.SecondsTaskLinkMissingRows > 0, true
	case "input_tokens_unavailable_requests":
		return q.InputTokensUnavailableRequests > 0, true
	case "legacy_test_cache_write_rows":
		return q.LegacyTestCacheWriteRows > 0, true
	case "cache_write_unreported_requests":
		return q.CacheWriteUnreportedRequests > 0, true
	case "unavailable_requests":
		return q.UnavailableRequests > 0, true
	case "unknown_billing_mode_requests":
		return q.UnknownBillingModeRequests > 0, true
	case "provider_model_fallback_rows":
		return q.ProviderModelFallbackRows > 0, true
	case "missing_historical_price_rows":
		return q.MissingHistoricalPriceRows > 0, true
	case "test_recomputed_rows":
		return q.TestRecomputedRows > 0, true
	case "test_recorded_original_rows":
		return q.TestRecordedOriginalRows > 0, true
	case "test_priced_rows":
		return q.TestPricedRows > 0, true
	case "recovered_billing_seconds_rows":
		return q.RecoveredBillingSecondsRows > 0, true
	case "refunded_task_hold_rows":
		return q.RefundedTaskHoldRows > 0, true

	case "cache_read_historical":
		return q.CacheReadUnavailableRequests-q.CacheReadUnreportedRequests-q.LegacyTestCacheReadRows > 0, true
	case "cache_write_historical":
		return q.CacheWriteUnavailableRequests-q.CacheWriteUnreportedRequests-q.LegacyTestCacheWriteRows > 0, true
	case "seconds_unit_missing":
		return q.SecondsUnavailableRows-q.SecondsValueMissingRows > 0, true
	case "seconds_value_missing":
		return q.SecondsValueMissingRows-q.SecondsTaskLinkMissingRows > 0, true
	case "test:estimated_usage", "test:fixed_price_mismatch", "test:missing_price_fields", "test:invalid_pricing_record", "test:missing_cache_write", "test:missing_cache_read", "test:missing_cache_ttl", "test:missing_cache_write_price", "test:missing_cache_read_price", "test:missing_usage_semantic", "test:missing_multimodal_usage", "test:missing_expression_context", "test:missing_group_ratio", "test:recorded_amount_mismatch", "test:missing_tool_price":
		return q.TestAmountPendingReasons[filter[len("test:"):]] > 0, true
	default:
		return false, false
	}
}
