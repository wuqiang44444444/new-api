package model

import "encoding/json"

// Batch logs describe the frozen input lines of one completed job. Counting
// the lines matches the funding transaction's request_count projection.
func billingStatementBatchFacts(log billingReconciliationLog, other map[string]json.RawMessage, parsed *parsedBillingReconciliationLog) {
	if billingBreakdownString(other["billing_mode"]) != "azure_batch" || log.Type != LogTypeConsume {
		return
	}
	count, ok := billingBreakdownNonNegativeInt(other["batch_line_count"])
	if !ok || count <= 0 {
		parsed.unavailable = true
		return
	}
	parsed.isRequest, parsed.isRefund = true, false
	parsed.requestCount = count
	parsed.billingMode = BillingReconciliationModeToken
}

func billingStatementRequestCount(parsed parsedBillingReconciliationLog) int64 {
	if !parsed.isRequest {
		return 0
	}
	return max(parsed.requestCount, 1)
}
