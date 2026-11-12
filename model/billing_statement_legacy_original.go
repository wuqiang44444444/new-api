package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

func replayMidjourneyFixedOriginal(booked int, price, group float64, discount *float64) (int64, string) {
	if math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, BillingOriginalReplayMissingContext
	}
	original := price * historicalCustomerQuotaPerUnit
	amount := original * group
	if discount != nil {
		if *discount <= 0 || *discount > 1 {
			return 0, BillingOriginalReplayMissingContext
		}
		amount = decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(*discount)).InexactFloat64()
	}
	net, err := common.QuotaFromFloatStrict(amount)
	gross, grossErr := common.QuotaFromFloatStrict(original)
	if err != nil || grossErr != nil || net != booked {
		return 0, BillingOriginalReplayMismatch
	}
	return int64(gross), ""
}

func replayStandaloneViolationOriginal(log billingReconciliationLog, facts customerExportFacts, group float64) (int64, string) {
	marked, _ := billingReconciliationBool(facts.other["violation_fee"])
	fee, feeKnown := billingBreakdownNonNegativeInt(facts.other["fee_quota"])
	base, baseKnown := billingReconciliationFloat(facts.other["base_amount"])
	if !marked || !feeKnown || fee <= 0 || fee != int64(log.Quota) || !baseKnown || base <= 0 ||
		log.PromptTokens != 0 || log.CompletionTokens != 0 || group <= 0 {
		return 0, BillingOriginalReplayUnverified
	}
	amount := decimal.NewFromFloat(base).Mul(decimal.NewFromInt(historicalCustomerQuotaPerUnit))
	booked, err := common.QuotaFromDecimalStrict(amount.Mul(decimal.NewFromFloat(group)))
	original, originalErr := common.QuotaFromDecimalStrict(amount)
	if err != nil || originalErr != nil || booked != log.Quota {
		return 0, BillingOriginalReplayMismatch
	}
	return int64(original), ""
}
