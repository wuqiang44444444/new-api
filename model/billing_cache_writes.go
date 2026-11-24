package model

import "encoding/json"

// BillingCacheWrites preserves observed tiers separately from historical totals.
// A nil quantity has no evidence. An observed zero remains present in JSON.
type BillingCacheWrites struct {
	Standard     *int64 `json:"standard,omitempty"`
	FiveMinutes  *int64 `json:"five_minutes,omitempty"`
	OneHour      *int64 `json:"one_hour,omitempty"`
	TwelveHours  *int64 `json:"twelve_hours,omitempty"`
	Unclassified int64  `json:"unclassified,omitempty"`
}

func observedBillingCacheWrites(other map[string]json.RawMessage, total, aggregate int64, aggregateKnown bool) *BillingCacheWrites {
	result := BillingCacheWrites{Unclassified: total}
	for _, tier := range []struct {
		key      string
		quantity **int64
	}{
		{"cache_creation_tokens_5m", &result.FiveMinutes},
		{"cache_creation_tokens_1h", &result.OneHour},
		{"cache_creation_tokens_12h", &result.TwelveHours},
	} {
		if count, ok := billingBreakdownNonNegativeInt(other[tier.key]); ok {
			if count > result.Unclassified {
				return mergeBillingCacheWrites(nil, nil, total)
			}
			*tier.quantity = &count
			result.Unclassified -= count
		}
	}
	if total == 0 && aggregateKnown {
		return zeroBillingCacheWrites()
	}
	claude, _ := billingReconciliationBool(other["claude"])
	if _, priced := billingReconciliationFloat(other["cache_creation_ratio"]); priced &&
		aggregateKnown && aggregate == total &&
		(result.FiveMinutes != nil || result.OneHour != nil || result.TwelveHours != nil ||
			billingBreakdownString(other["usage_semantic"]) == "openai" ||
			(claude && billingBreakdownString(other["usage_semantic"]) == "anthropic")) {
		standard := result.Unclassified
		result.Standard, result.Unclassified = &standard, 0
	}
	if result == (BillingCacheWrites{}) {
		return nil
	}
	return &result
}

func zeroBillingCacheWrites() *BillingCacheWrites {
	var zero int64
	return &BillingCacheWrites{Standard: &zero, FiveMinutes: &zero, OneHour: &zero, TwelveHours: &zero}
}

// mergeBillingCacheWrites also accepts an older projection containing just a
// total. Operands are immutable since summaries may reuse published metrics.
func mergeBillingCacheWrites(target, source *BillingCacheWrites, historicalTotal int64) *BillingCacheWrites {
	if source == nil && historicalTotal <= 0 {
		return target
	}
	var merged BillingCacheWrites
	if target != nil {
		merged = *target
	}
	if source == nil {
		addBillingStatementValue(&merged.Unclassified, historicalTotal)
		return &merged
	}
	for _, tier := range []struct {
		destination **int64
		metered     *int64
	}{
		{&merged.Standard, source.Standard},
		{&merged.FiveMinutes, source.FiveMinutes},
		{&merged.OneHour, source.OneHour},
		{&merged.TwelveHours, source.TwelveHours},
	} {
		if tier.metered == nil {
			continue
		}
		count := *tier.metered
		if *tier.destination != nil {
			addBillingStatementValue(&count, **tier.destination)
		}
		*tier.destination = &count
	}
	addBillingStatementValue(&merged.Unclassified, source.Unclassified)
	return &merged
}
