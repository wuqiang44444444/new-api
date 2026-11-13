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
const CustomerOriginalReplayVersion = 10

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
	var original int64
	var reason string
	state := billingContractApplicableState(*parsed)
	if replay := parsed.frozenReplay; replay != nil && (state == combinationContractApplicableNo || state == combinationContractApplicableYes) && !parsed.hasAuxiliaryCharge {
		reason = replay.Reason
		if replay.OriginalQuota != nil {
			original = *replay.OriginalQuota
		} else if reason == "" {
			reason = BillingOriginalReplayUnverified
		}
	} else {
		original, reason = replayCustomerOriginal(log, *parsed)
	}
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
	result, reason := evaluateFrozenBillingExpression(log, facts, *parsed.discountRatio, requireFixedTier)
	if reason != "" {
		return 0, reason
	}
	original, booked, reason := frozenExpressionAmounts(facts, parsed, result.TieredResult)
	if reason != "" {
		return 0, reason
	}
	if booked != int64(log.Quota) {
		return 0, BillingOriginalReplayMismatch
	}
	return original, ""
}

type frozenExpressionEvaluation struct {
	billingexpr.TieredResult
	tokens billingexpr.TokenParams
	used   map[string]bool
}

func evaluateFrozenBillingExpression(log billingReconciliationLog, facts customerExportFacts, group float64, requireFixedTier bool) (frozenExpressionEvaluation, string) {
	if facts.duplicateRequestRules {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
	}
	encoded := billingBreakdownString(facts.raw("expr_b64"))
	tier := billingBreakdownString(facts.raw("matched_tier"))
	unit := billingexpr.BillingUnit(billingBreakdownString(facts.raw("billing_unit")))
	if encoded == "" || tier == "" {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
	}
	expression := string(raw)
	program, err := billingexpr.CompileFromCache(expression)
	if err != nil {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
	}
	if unit == "" && !billingexpr.UsesFixedPricing(expression) {
		// fixed() is the only expression operation that switches to per-request pricing.
		unit = billingexpr.BillingUnitToken
	}
	if unit != billingexpr.BillingUnitToken && unit != billingexpr.BillingUnitRequest {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
	}
	version := billingexpr.ExprVersion(expression)
	normalizationVars := billingexpr.UsedVars(expression)
	request := billingexpr.RequestInput{}
	var used map[string]bool
	var traces []billingexpr.RequestRuleTrace
	probed := ast.Find(program.Node(), historicalBillingProbe) != nil
	rules := facts.raw("request_rules")
	if !requireFixedTier && (len(rules) != 0 || probed && unit == billingexpr.BillingUnitRequest) {
		if len(rules) != 0 {
			var valid bool
			traces, valid = frozenStatementRequestRules(rules)
			if !valid {
				return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
			}
		}
		var fixedPrice *float64
		if unit == billingexpr.BillingUnitRequest {
			var price *float64
			if common.Unmarshal(facts.raw("fixed_price"), &price) == nil {
				fixedPrice = price
			}
		}
		request, used, err = billingexpr.WithFrozenRequestRules(expression, request, traces, tier, fixedPrice)
		if err != nil {
			return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
		}
		probed = false
	}
	if requireFixedTier || probed {
		selected, fixedTier := historicalStatementTierExpression(expression, tier)
		if requireFixedTier && !fixedTier {
			return frozenExpressionEvaluation{}, BillingOriginalReplayUnverified
		}
		expression = selected
		if expression == "" {
			return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
		}
	}
	if used == nil {
		used = billingexpr.UsedVars(expression)
	}
	var rate *billingexpr.ExchangeRateContext
	if used[billingexpr.UsdExchangeRateFunc] {
		var rateFact map[string]any
		if common.Unmarshal(facts.raw("usd_exchange_rate"), &rateFact) == nil {
			rate = billingexpr.ParseExchangeRateFact(rateFact)
		}
		if rate == nil {
			return frozenExpressionEvaluation{}, BillingOriginalReplayMissingContext
		}
	}
	if used["image_count"] {
		raw := facts.raw("image_count")
		count, ok := billingBreakdownNonNegativeInt(raw)
		if !ok || count <= 0 || count > int64(dto.MaxImageN) {
			return frozenExpressionEvaluation{}, BillingOriginalReplayMissingUsage
		}
		n := int(count)
		request.ImageCount = &n
	}
	params, ok := historicalCustomerTokenParams(log, facts, used, normalizationVars)
	if !ok {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMissingUsage
	}
	snapshot := billingexpr.BillingSnapshot{
		ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
		ExprVersion: version, GroupRatio: group,
		QuotaPerUnit: historicalCustomerQuotaPerUnit, UsdExchangeRate: rate,
	}
	result, err := billingexpr.ComputeTieredQuotaWithRequest(&snapshot, params, request)
	if err != nil || result.Clamp != nil || result.ActualQuotaBeforeGroup < 0 ||
		result.MatchedTier != tier || result.BillingUnit != unit {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMismatch
	}
	if traces != nil && len(result.RequestRules) != len(traces) {
		return frozenExpressionEvaluation{}, BillingOriginalReplayMismatch
	}
	for i, rule := range traces {
		if result.RequestRules[i].Matched != rule.Matched {
			return frozenExpressionEvaluation{}, BillingOriginalReplayMismatch
		}
	}
	if unit == billingexpr.BillingUnitRequest && len(facts.raw("fixed_price")) > 0 {
		raw := facts.raw("fixed_price")
		fixed, ok := billingReconciliationFloat(raw)
		if !ok || string(raw) == "null" || result.FixedPrice == nil || fixed != *result.FixedPrice {
			return frozenExpressionEvaluation{}, BillingOriginalReplayMismatch
		}
	}
	return frozenExpressionEvaluation{TieredResult: result, tokens: params, used: used}, ""
}

func frozenExpressionAmounts(facts customerExportFacts, parsed parsedBillingReconciliationLog, result billingexpr.TieredResult) (original, booked int64, reason string) {
	if parsed.hasAuxiliaryCharge {
		return frozenTieredAmountsWithToolFees(facts, parsed, result.ActualQuotaBeforeGroup)
	}
	booked = int64(result.ActualQuotaAfterGroup)
	var err error
	if parsed.contractDiscountRatio != nil {
		discount := *parsed.contractDiscountRatio
		if discount <= 0 || discount > 1 || math.IsNaN(discount) {
			return 0, 0, BillingOriginalReplayMissingContext
		}
		amount := decimal.NewFromFloat(result.ActualQuotaBeforeGroup).
			Mul(decimal.NewFromFloat(*parsed.discountRatio)).Mul(decimal.NewFromFloat(discount))
		var quota int
		quota, err = common.QuotaRoundStrict(amount.InexactFloat64())
		booked = int64(quota)
	}
	originalQuota, originalErr := common.QuotaRoundStrict(result.ActualQuotaBeforeGroup)
	if err != nil || originalErr != nil {
		return 0, 0, BillingOriginalReplayMismatch
	}
	return int64(originalQuota), booked, ""
}

func historicalCustomerTokenParams(log billingReconciliationLog, facts customerExportFacts, used, normalization map[string]bool) (billingexpr.TokenParams, bool) {
	var params billingexpr.TokenParams
	if raw := facts.raw("billing_tokens"); len(raw) != 0 {
		var frozen map[string]json.RawMessage
		if common.Unmarshal(raw, &frozen) != nil {
			return params, false
		}
		for key, field := range map[string]*float64{"p": &params.P, "c": &params.C, "len": &params.Len,
			"cr": &params.CR, "cc": &params.CC, "cc1h": &params.CC1h, "img": &params.Img,
			"img_cr": &params.ImgCR, "img_o": &params.ImgO, "ai": &params.AI, "ao": &params.AO} {
			value, ok := billingBreakdownNonNegativeInt(frozen[key])
			if !ok && used[key] {
				return params, false
			}
			*field = float64(value)
		}
		return params, true
	}
	if log.PromptTokens < 0 || log.CompletionTokens < 0 {
		return params, false
	}
	semantic := billingBreakdownString(facts.other["usage_semantic"])
	// The evaluator may use only the selected tier, but settlement normalized
	// p/c with the entire expression before evaluating any branch.
	for _, key := range []string{"img_cr", "img_o", "ai", "ao"} {
		if used[key] || (semantic != "anthropic" && normalization[key] &&
			(used["p"] && (key == "img_cr" || key == "ai") || used["c"] && (key == "img_o" || key == "ao"))) {
			return params, false
		}
	}
	params.P, params.C = float64(log.PromptTokens), float64(log.CompletionTokens)
	params.Len = params.P
	if used["img"] || (used["p"] && semantic != "anthropic" && normalization["img"]) {
		imageTokens, ok := historicalStatementImageTokens(facts, semantic)
		if !ok {
			return params, false
		}
		params.Img = float64(imageTokens)
	}
	cacheRead, readKnown := billingBreakdownNonNegativeInt(facts.other["cache_tokens"])
	cacheWrite := normalizedBillingBreakdownCacheWriteTokens(facts.other)
	if (used["cr"] || (semantic == "anthropic" && used["len"]) ||
		(used["p"] && ((semantic == "anthropic") != normalization["cr"]))) && !readKnown {
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
		if !normalization["cr"] {
			params.P += params.CR
		}
	case "openai", "gemini":
		needsWrite := used["cc"] || used["p"] && normalization["cc"]
		if needsWrite && ((!cacheWrite.known && !nativeUsageLogOmitsZeroCacheWrite(log, facts.other) &&
			!historicalTestOmitsCacheWrite(log, facts.other)) || cacheWrite.oneHour != 0) {
			return params, false
		}
		params.CC = float64(cacheWrite.total)
		if normalization["cr"] {
			params.P -= params.CR
		}
		if normalization["cc"] {
			params.P -= params.CC
		}
		if normalization["img"] {
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
