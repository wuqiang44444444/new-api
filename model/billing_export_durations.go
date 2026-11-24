package model

import (
	"encoding/base64"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
)

// exportBillingDurations preserves individually recorded seconds when there is
// no single billable-seconds value for the CSV.
func exportBillingDurations(log billingReconciliationLog, parsed parsedBillingReconciliationLog, facts customerExportFacts) map[string]decimal.Decimal {
	if (parsed.billingMode != BillingReconciliationModeDurationAndCount && parsed.billingMode != BillingReconciliationModePerSecond) ||
		facts.string("task_billing_event") == "create" ||
		(log.Type == LogTypeRefund && !isProviderTaskUsageAdjustment(log)) {
		return nil
	}
	var units map[string]string
	var values map[string]json.RawMessage
	if common.Unmarshal(facts.raw("usage_units"), &units) != nil || common.Unmarshal(facts.raw("usage_facts"), &values) != nil {
		return nil
	}
	var result map[string]decimal.Decimal
	for _, key := range frozenSecondMeters(facts, units) {
		raw := string(values[key])
		if len(raw) >= 2 && raw[0] == '"' {
			raw = billingBreakdownString(values[key])
		}
		seconds, err := decimal.NewFromString(raw)
		if err != nil || seconds.IsNegative() {
			continue
		}
		if result == nil {
			result = make(map[string]decimal.Decimal)
		}
		result[key] = seconds
	}
	return result
}

// The price projection separates each priced term from its selection predicates.
// Use the frozen tier to choose a single branch, including for historical logs
// whose frozen metadata has no calculation trace.
func frozenSecondMeters(facts customerExportFacts, units map[string]string) []string {
	var meters []string
	encoded := facts.string("expr_b64")
	if encoded == "" {
		for key, unit := range units {
			if unit == "second" {
				meters = append(meters, key)
			}
		}
		if len(meters) == 1 {
			return meters
		}
		return nil
	}
	expression, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	fields := make(map[string]billingexpr.TaskUsageFieldInfo, len(units))
	for key, unit := range units {
		fields[key] = billingexpr.TaskUsageFieldInfo{Unit: unit}
	}
	var rate *billingexpr.ExchangeRateContext
	if raw := facts.raw("usd_exchange_rate"); len(raw) > 0 {
		_ = common.Unmarshal(raw, &rate)
	}
	projection, err := billingexpr.TaskDisplayProjectionForWithRate(string(expression), fields, rate)
	if err != nil || projection.Status != billingexpr.DisplayStatusExact {
		return nil
	}
	matched := facts.string("matched_tier")
	var selected *billingexpr.DisplayTier
	for i := range projection.Tiers {
		tier := &projection.Tiers[i]
		if tier.Label == matched || (matched == "" && len(projection.Tiers) == 1) {
			if selected != nil {
				return nil
			}
			selected = tier
		}
	}
	if selected != nil {
		for key := range selected.UnitPrices {
			if units[key] == "second" {
				meters = append(meters, key)
			}
		}
	}
	return meters
}
