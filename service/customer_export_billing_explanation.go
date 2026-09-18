package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Export only the shared customer-safe explanation, never raw log metadata.
// Each ledger event stays one CSV row even when it has several price components.
func attachCustomerExportBillingExplanation(log *model.Log, row *model.CustomerExportRow, quotaPerUnit float64) {
	row.ExplanationStatus = "not_applicable"
	if log.Type != model.LogTypeConsume || row.RequestCount == 0 {
		return
	}
	row.ExplanationStatus = "unavailable"
	var other map[string]any
	if common.UnmarshalJsonStr(log.Other, &other) != nil {
		return
	}
	row.MatchedTier, _ = other["matched_tier"].(string)
	lines := customerBillingLines(log, other, *row, quotaPerUnit)
	if len(lines) == 0 {
		return
	}
	type exportLine struct {
		customerBillingLine
		PriceQuantity int `json:"unit_price_quantity"`
	}
	safeLines := make([]exportLine, len(lines))
	for i, line := range lines {
		basis := 1
		if line.Unit == "token" {
			basis = 1000000
		}
		safeLines[i] = exportLine{line, basis}
	}
	raw, err := common.Marshal(safeLines)
	if err != nil {
		return
	}
	row.BillingLineItems = string(raw)
	row.ExplanationStatus = "available"
	row.PriceConversionBasis = "recorded_usd_price"
	if row.BillingMode == model.BillingReconciliationModeToken && other["expr_b64"] == nil {
		row.PriceConversionBasis = "frozen_quota_conversion"
	}
}
