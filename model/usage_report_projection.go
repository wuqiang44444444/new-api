package model

import (
	"github.com/shopspring/decimal"
)

// A report stores the unrounded accumulators so week assembly uses the same
// amount and discount finalizers as the ordinary day/week calculator.
type usageReportMetrics struct {
	Metrics            UsageAnalyticsMetrics
	Seconds            decimal.Decimal
	SecondsKnown       bool
	Original           decimal.Decimal
	OriginalKnown      bool
	OriginalIncomplete bool
	Reference          decimal.Decimal
	Coefficients       map[string]struct{}
	Discounts          []usageReportDiscount
}

type usageReportDiscount struct {
	Combination BillingDiscountCombination
	Original    decimal.Decimal
	Complete    bool
}

func makeUsageReportMetrics(a *usageMetricsAcc) usageReportMetrics {
	record := usageReportMetrics{
		Metrics: a.m, Seconds: a.seconds, SecondsKnown: a.secondsKnown, Original: a.original,
		OriginalKnown: a.originalKnown, OriginalIncomplete: a.originalIncomplete,
		Reference: a.reference, Coefficients: a.coefficients,
	}
	if a.discounts != nil {
		for _, entry := range a.discounts.combinations {
			record.Discounts = append(record.Discounts, usageReportDiscount{entry.combo, entry.originalQuota, entry.originalComplete})
		}
		if entry := a.discounts.overflow; entry != nil {
			record.Discounts = append(record.Discounts, usageReportDiscount{entry.combo, entry.originalQuota, entry.originalComplete})
		}
	}
	return record
}

func (r usageReportMetrics) accumulator() *usageMetricsAcc {
	acc := &usageMetricsAcc{
		m: r.Metrics, seconds: r.Seconds, secondsKnown: r.SecondsKnown, original: r.Original,
		originalKnown: r.OriginalKnown, originalIncomplete: r.OriginalIncomplete,
		reference: r.Reference, coefficients: r.Coefficients,
	}
	if len(r.Discounts) == 0 {
		return acc
	}
	acc.discounts = newBillingDiscountCombinationAccumulator()
	for _, row := range r.Discounts {
		c := row.Combination
		entry := &billingDiscountCombinationEntry{combo: c, originalQuota: row.Original, originalComplete: row.Complete}
		if c.Other {
			acc.discounts.overflow = entry
			continue
		}
		key := billingDiscountKeyFromProjection(c)
		acc.discounts.combinations[key] = entry
	}
	return acc
}

func billingDiscountKeyFromProjection(c BillingDiscountCombination) billingDiscountCombinationKey {
	key := billingDiscountCombinationKey{
		groupId: c.GroupId, groupName: c.GroupName, groupRatioSource: c.GroupRatioSource,
		model: c.ModelName, mode: c.BillingMode, contractApplicable: c.ContractApplicable,
		contractId: c.ContractId, contractIdKnown: c.ContractIdKnown, contractVersion: c.ContractVersion,
	}
	if c.GroupRatio != nil {
		key.hasGroupRatio, key.groupRatio = true, *c.GroupRatio
	}
	if c.ContractRatio != nil {
		key.hasContractRatio, key.contractRatio = true, *c.ContractRatio
	}
	return key
}
