package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// FrozenChargeReplay is read-only. It can explain a discrepancy even when
// a trusted booked amount already exists.
type FrozenChargeReplay struct {
	TokenInputs   map[string]float64 `json:"token_inputs,omitempty"`
	MatchedTier   string             `json:"matched_tier,omitempty"`
	OriginalQuota *int64             `json:"original_quota,omitempty"`
	ExpectedQuota *int64             `json:"expected_quota,omitempty"`
	BookedQuota   *int64             `json:"booked_quota,omitempty"`
	DeltaQuota    *int64             `json:"delta_quota,omitempty"`
	Reason        string             `json:"reason,omitempty"`
}

// projectFrozenChargeReplay attaches the same per-row calculation to usage
// and reconciliation before either aggregates monetary amounts.
func projectFrozenChargeReplay(log billingReconciliationLog, parsed *parsedBillingReconciliationLog) {
	if parsed.hasExpression && !parsed.isTask && parsed.taskID == "" && parsed.taskBillingEvent == "" && log.Type == LogTypeConsume {
		parsed.frozenReplay = frozenChargeReplay(log, *parsed, loadCustomerExportFacts(log.Other))
	}
}

func frozenChargeReplay(log billingReconciliationLog, parsed parsedBillingReconciliationLog, facts customerExportFacts) *FrozenChargeReplay {
	replay := &FrozenChargeReplay{MatchedTier: facts.string("matched_tier")}
	actual := int64(log.Quota)
	if !parsed.isChannelTest {
		replay.BookedQuota = &actual
	}
	if log.Quota < 0 || log.Quota > common.MaxQuota {
		replay.Reason = BillingOriginalReplayUnverified
		return replay
	}
	if parsed.unavailable || parsed.discountRatio == nil || (parsed.isChannelTest && parsed.hasAuxiliaryCharge) ||
		(!parsed.isChannelTest && billingContractApplicableState(parsed) == combinationContractApplicableUnknown) {
		replay.Reason = BillingOriginalReplayMissingContext
		return replay
	}
	var admin map[string]json.RawMessage
	if raw := facts.other["admin_info"]; len(raw) > 0 && common.Unmarshal(raw, &admin) != nil {
		replay.Reason = BillingOriginalReplayUnverified
		return replay
	}
	estimated, _ := billingReconciliationBool(admin["local_count_tokens"])
	if len(admin["quota_saturation"]) > 0 || (parsed.isChannelTest && (estimated ||
		parsed.testPricing != nil && parsed.testPricing.Status != upstreamTestPricingStatusSettled)) {
		replay.Reason = BillingOriginalReplayUnverified
		return replay
	}
	result, reason := evaluateFrozenBillingExpression(log, facts, *parsed.discountRatio, false)
	if reason != "" {
		replay.Reason = reason
		return replay
	}
	original, expected, reason := frozenExpressionAmounts(facts, parsed, result.TieredResult)
	replay.Reason = reason
	if reason != "" {
		return replay
	}
	replay.OriginalQuota, replay.ExpectedQuota = &original, &expected
	if result.BillingUnit == billingexpr.BillingUnitToken {
		params := result.tokens
		for key, value := range map[string]float64{"p": params.P, "c": params.C, "len": params.Len, "cr": params.CR, "cc": params.CC, "cc1h": params.CC1h, "img": params.Img, "img_cr": params.ImgCR, "img_o": params.ImgO, "ai": params.AI, "ao": params.AO} {
			if result.used[key] {
				if replay.TokenInputs == nil {
					replay.TokenInputs = make(map[string]float64)
				}
				replay.TokenInputs[key] = value
			}
		}
	}
	if !parsed.isChannelTest {
		delta := actual - expected
		replay.DeltaQuota = &delta
		if delta != 0 {
			replay.Reason = BillingOriginalReplayMismatch
		}
	}
	if parsed.isChannelTest && expected != actual && parsed.testPricing == nil {
		replay.Reason = BillingOriginalReplayMismatch
	}
	if parsed.recordedOriginalQuota != nil && *parsed.recordedOriginalQuota != original {
		replay.Reason = BillingOriginalReplayMismatch
	}
	return replay
}
