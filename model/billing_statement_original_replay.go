package model

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/expr-lang/expr/ast"
	"github.com/shopspring/decimal"
)

// CustomerOriginalReplayVersion identifies the rules used to construct a
// customer billing projection from frozen evidence.
const CustomerOriginalReplayVersion = 5

// Native historical logs used the gateway's fixed quota denomination.
const historicalCustomerQuotaPerUnit = 500_000

const (
	BillingOriginalReplayMissingContext = "original_replay_missing_context"
	BillingOriginalReplayMissingUsage   = "original_replay_missing_usage"
	BillingOriginalReplayMismatch       = "original_replay_settlement_mismatch"
	BillingOriginalReplayUnverified     = "original_replay_unverified"
	customerOriginalReplayed            = "replayed"
	customerOriginalUniqueSettlement    = "unique_settlement_replay"
)

// Verify a historical customer original against the frozen rule and actual settlement.
func applyCustomerOriginalReplay(log billingReconciliationLog, parsed *parsedBillingReconciliationLog) {
	if (log.Type != LogTypeConsume && log.Type != LogTypeRefund) || parsed.isChannelTest || log.UserId <= 0 || parsed.replayedOriginalQuota != nil || parsed.originalReplayReason != "" {
		return
	}
	if _, ok := recordedStatementOriginalQuota(log, *parsed); ok {
		return
	}
	original, reason := replayCustomerOriginal(log, *parsed)
	if reason == BillingOriginalReplayMissingUsage {
		if amount, count, ok := recoverUniqueImageOutputOriginal(log, *parsed); ok {
			parsed.replayedOriginalQuota = &amount
			parsed.derivedOutputImageTokens = &count
			return
		}
	}
	if reason != "" {
		parsed.originalReplayReason = reason
		return
	}
	parsed.replayedOriginalQuota = &original
}

func customerStatementOriginalQuota(log billingReconciliationLog, parsed parsedBillingReconciliationLog) (decimal.Decimal, bool) {
	if original, ok := recordedStatementOriginalQuota(log, parsed); ok {
		return original, true
	}
	if parsed.replayedOriginalQuota != nil && !parsed.unavailable && !parsed.fundingAdjustmentFailed {
		return decimal.NewFromInt(signedStatementEventQuota(*parsed.replayedOriginalQuota, log.Type == LogTypeRefund)), true
	}
	return decimal.Zero, false
}

func replayCustomerOriginal(log billingReconciliationLog, parsed parsedBillingReconciliationLog) (int64, string) {
	return replayCustomerOriginalFromEvidence(log, parsed, false)
}

func replayCustomerOriginalFromEvidence(log billingReconciliationLog, parsed parsedBillingReconciliationLog, requireFixedTier bool) (int64, string) {
	if log.Type != LogTypeConsume || parsed.isTask || parsed.taskID != "" ||
		parsed.taskBillingEvent != "" || parsed.unavailable ||
		log.Quota < 0 || log.Quota > common.MaxQuota {
		return 0, BillingOriginalReplayUnverified
	}
	facts := loadCustomerExportFacts(log.Other)
	var admin map[string]json.RawMessage
	if raw := facts.other["admin_info"]; len(raw) != 0 && common.Unmarshal(raw, &admin) != nil {
		return 0, BillingOriginalReplayUnverified
	}
	if len(admin["quota_saturation"]) != 0 || parsed.discountRatio == nil {
		return 0, BillingOriginalReplayMissingContext
	}
	if violation, _ := billingReconciliationBool(facts.other["violation_fee"]); parsed.hasAuxiliaryCharge && violation {
		return replayStandaloneViolationOriginal(log, facts, *parsed.discountRatio)
	}
	if billingContractApplicableState(parsed) != combinationContractApplicableNo &&
		billingContractApplicableState(parsed) != combinationContractApplicableYes {
		return 0, BillingOriginalReplayMissingContext
	}
	if billingBreakdownString(facts.other["billing_mode"]) != "tiered_expr" {
		if requireFixedTier {
			return 0, BillingOriginalReplayUnverified
		}
		if strings.Contains(billingBreakdownString(facts.other["request_path"]), "/mj/") &&
			parsed.billingMode == BillingReconciliationModePerCall && parsed.modelPrice != nil && *parsed.modelPrice >= 0 {
			return replayMidjourneyFixedOriginal(log.Quota, *parsed.modelPrice, *parsed.discountRatio, parsed.contractDiscountRatio)
		}
		return replayNativeRatioOriginal(log, facts, parsed)
	}
	encoded := billingBreakdownString(facts.raw("expr_b64"))
	tier := billingBreakdownString(facts.raw("matched_tier"))
	unit := billingexpr.BillingUnit(billingBreakdownString(facts.raw("billing_unit")))
	if encoded == "" || tier == "" {
		return 0, BillingOriginalReplayMissingContext
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return 0, BillingOriginalReplayMissingContext
	}
	expression := string(raw)
	program, err := billingexpr.CompileFromCache(expression)
	if err != nil {
		return 0, BillingOriginalReplayMissingContext
	}
	if unit == "" && !billingexpr.UsesFixedPricing(expression) {
		// fixed() is the only expression operation that switches to per-request pricing.
		unit = billingexpr.BillingUnitToken
	}
	if unit != billingexpr.BillingUnitToken && unit != billingexpr.BillingUnitRequest {
		return 0, BillingOriginalReplayMissingContext
	}
	version := billingexpr.ExprVersion(expression)
	used := billingexpr.UsedVars(expression)
	if requireFixedTier || ast.Find(program.Node(), historicalBillingProbe) != nil {
		selected, fixedTier := historicalStatementTierExpression(expression, tier, unit, facts)
		if requireFixedTier && !fixedTier {
			return 0, BillingOriginalReplayUnverified
		}
		expression = selected
		if expression == "" {
			return 0, BillingOriginalReplayMissingContext
		}
	}
	var rate *billingexpr.ExchangeRateContext
	if billingexpr.UsesExchangeRate(expression) {
		var rateFact map[string]any
		if common.Unmarshal(facts.raw("usd_exchange_rate"), &rateFact) == nil {
			rate = billingexpr.ParseExchangeRateFact(rateFact)
		}
		if rate == nil {
			return 0, BillingOriginalReplayMissingContext
		}
	}
	request := billingexpr.RequestInput{}
	if raw := facts.raw("image_count"); len(raw) != 0 {
		count, ok := billingBreakdownNonNegativeInt(raw)
		if !ok || count <= 0 || count > int64(dto.MaxImageN) {
			return 0, BillingOriginalReplayMissingUsage
		}
		n := int(count)
		request.ImageCount = &n
	} else if used["image_count"] {
		return 0, BillingOriginalReplayMissingUsage
	}
	params, ok := historicalCustomerTokenParams(log, facts, used)
	if !ok {
		return 0, BillingOriginalReplayMissingUsage
	}
	group := *parsed.discountRatio
	snapshot := billingexpr.BillingSnapshot{
		ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
		ExprVersion: version, GroupRatio: group,
		QuotaPerUnit: historicalCustomerQuotaPerUnit, UsdExchangeRate: rate,
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(&snapshot, params, request)
	if err != nil || result.Clamp != nil || result.ActualQuotaBeforeGroup < 0 ||
		result.MatchedTier != tier || result.BillingUnit != unit {
		return 0, BillingOriginalReplayMismatch
	}
	if parsed.hasAuxiliaryCharge {
		return replayTieredWithToolFees(log, facts, parsed, result.ActualQuotaBeforeGroup)
	}
	booked := result.ActualQuotaAfterGroup
	if parsed.contractDiscountRatio != nil {
		discount := *parsed.contractDiscountRatio
		if discount <= 0 || discount > 1 || math.IsNaN(discount) {
			return 0, BillingOriginalReplayMissingContext
		}
		amount := decimal.NewFromFloat(result.ActualQuotaBeforeGroup).
			Mul(decimal.NewFromFloat(group)).Mul(decimal.NewFromFloat(discount))
		booked, err = common.QuotaRoundStrict(amount.InexactFloat64())
	}
	original, originalErr := common.QuotaRoundStrict(result.ActualQuotaBeforeGroup)
	if err != nil || originalErr != nil || booked != log.Quota {
		return 0, BillingOriginalReplayMismatch
	}
	return int64(original), ""
}

func historicalCustomerTokenParams(log billingReconciliationLog, facts customerExportFacts, used map[string]bool) (billingexpr.TokenParams, bool) {
	var params billingexpr.TokenParams
	if raw := facts.other["billing_tokens"]; len(raw) != 0 {
		var frozen map[string]json.RawMessage
		if common.Unmarshal(raw, &frozen) != nil {
			return params, false
		}
		for key, field := range map[string]*float64{"p": &params.P, "c": &params.C, "len": &params.Len,
			"cr": &params.CR, "cc": &params.CC, "cc1h": &params.CC1h, "img": &params.Img,
			"img_cr": &params.ImgCR, "img_o": &params.ImgO, "ai": &params.AI, "ao": &params.AO} {
			value, ok := billingBreakdownNonNegativeInt(frozen[key])
			if !ok && (used[key] || len(frozen[key]) != 0) {
				return params, false
			}
			*field = float64(value)
		}
		return params, true
	}
	if log.PromptTokens < 0 || log.CompletionTokens < 0 {
		return params, false
	}
	for _, key := range []string{"img_cr", "img_o", "ai", "ao"} {
		if used[key] {
			return params, false
		}
	}
	params.P, params.C = float64(log.PromptTokens), float64(log.CompletionTokens)
	params.Len = params.P
	semantic := billingBreakdownString(facts.other["usage_semantic"])
	if used["img"] {
		imageTokens, ok := historicalStatementImageTokens(facts, semantic)
		if !ok {
			return params, false
		}
		params.Img = float64(imageTokens)
	}
	cacheRead, readKnown := billingBreakdownNonNegativeInt(facts.other["cache_tokens"])
	cacheWrite := normalizedBillingBreakdownCacheWriteTokens(facts.other)
	if (used["cr"] || semantic == "anthropic" && (used["p"] || used["len"])) && !readKnown {
		return params, false
	}
	params.CR = float64(cacheRead)
	switch semantic {
	case "anthropic":
		if (used["cc"] || used["cc1h"] || used["len"]) &&
			(!cacheWrite.known || cacheWrite.total-cacheWrite.fiveMinutes != cacheWrite.oneHour) {
			return params, false
		}
		params.CC, params.CC1h = float64(cacheWrite.fiveMinutes), float64(cacheWrite.oneHour)
		params.Len += params.CR + params.CC + params.CC1h
		if !used["cr"] {
			params.P += params.CR
		}
	case "openai", "gemini":
		if used["cc"] && !cacheWrite.known {
			// The synchronous text writer omitted zero cache creation fields.
			if len(facts.other["cache_creation_tokens"]) != 0 || len(facts.other["cache_write_tokens"]) != 0 ||
				len(facts.other["cache_creation_tokens_5m"]) != 0 || len(facts.other["cache_creation_tokens_1h"]) != 0 {
				return params, false
			}
		}
		if cacheWrite.oneHour != 0 {
			return params, false
		}
		params.CC = float64(cacheWrite.total)
		if used["cr"] {
			params.P -= params.CR
		}
		if used["cc"] {
			params.P -= params.CC
		}
		if used["img"] {
			params.P -= params.Img
		}
		params.P = max(params.P, 0)
	default:
		if used["p"] || used["len"] || used["cr"] || used["cc"] || used["cc1h"] {
			return params, false
		}
	}
	return params, true
}
