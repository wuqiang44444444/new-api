package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// Older expression traces defaulted to token even for a literal request
// price. Only the unique frozen tier and its verified settlement can supply
// the missing mode.
func recoverHistoricalFixedRequestMode(log billingReconciliationLog, other, snapshot map[string]json.RawMessage, parsed *parsedBillingReconciliationLog) {
	if parsed.billingMode != BillingReconciliationModeUnknown || !parsed.isRequest || parsed.isChannelTest ||
		log.UserId <= 0 || parsed.hasAuxiliaryCharge || parsed.hasImageCount || parsed.completedImageCount != 0 ||
		billingBreakdownString(other["billing_mode"]) != "tiered_expr" ||
		billingBreakdownString(other["billing_unit"]) != string(billingexpr.BillingUnitToken) {
		return
	}
	switch billingBreakdownString(snapshot["billing_mode"]) {
	case "", BillingReconciliationModeUnknown, BillingReconciliationModeToken:
	default:
		return
	}
	original, reason := replayCustomerOriginalFromEvidence(log, *parsed, true)
	if reason == "" && (parsed.recordedOriginalQuota == nil || *parsed.recordedOriginalQuota == original) {
		parsed.billingMode = BillingReconciliationModePerCall
	}
}
