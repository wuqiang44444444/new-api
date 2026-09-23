package model

import "github.com/QuantumNous/new-api/common"

// usageConsumeResult projects the persisted final stream evidence, independently
// of its charge. Legacy/non-stream consumes retain their existing success meaning.
// Only controlled terminal reasons and exact adapter tokens classify failures;
// soft error counts and arbitrary error text do not prove generation failure.
func usageConsumeResult(row usageLogRow) string {
	raw, recorded := row.other["stream_status"]
	if !recorded {
		return usageResultSuccess
	}
	var stream struct {
		EndReason string   `json:"end_reason"`
		Errors    []string `json:"errors"`
	}
	if err := common.Unmarshal(raw, &stream); err != nil {
		return usageResultOther
	}
	switch stream.EndReason {
	case "timeout", "scanner_error", "panic", "ping_fail":
		return usageResultFailure
	case "client_gone":
		return usageResultCancelled
	}
	for _, token := range stream.Errors {
		switch token {
		case "response_failed", "response_incomplete":
			return usageResultFailure
		case "response_cancelled":
			return usageResultCancelled
		}
	}
	switch stream.EndReason {
	case "done", "eof", "handler_stop":
		return usageResultSuccess
	default:
		return usageResultOther
	}
}
