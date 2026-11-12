package model

import (
	"encoding/base64"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
	"github.com/shopspring/decimal"
)

// Bound the work per log independently of the maximum usage accepted by the
// gateway. Large or unsupported historical proofs remain unresolved.
const maxUniqueOutputReplayCandidates = 16_384

func recoverUniqueImageOutputOriginal(log billingReconciliationLog, parsed parsedBillingReconciliationLog) (int64, int, bool) {
	if log.Type != LogTypeConsume || parsed.isTask || parsed.taskID != "" || parsed.taskBillingEvent != "" ||
		parsed.unavailable || parsed.fundingAdjustmentFailed || parsed.hasAuxiliaryCharge ||
		log.Quota <= 0 || log.Quota > common.MaxQuota || log.CompletionTokens <= 0 ||
		log.CompletionTokens >= maxUniqueOutputReplayCandidates || parsed.discountRatio == nil || *parsed.discountRatio <= 0 {
		return 0, 0, false
	}
	facts := loadCustomerExportFacts(log.Other)
	if facts.string("usage_semantic") != "gemini" || billingBreakdownString(facts.other["billing_mode"]) != "tiered_expr" ||
		facts.string("billing_unit") != string(billingexpr.BillingUnitToken) ||
		!strings.HasSuffix(facts.string("request_path"), ":generateContent") ||
		len(facts.other["billing_tokens"]) != 0 || len(facts.other["image_token_usage"]) != 0 {
		return 0, 0, false
	}
	raw, err := base64.StdEncoding.DecodeString(facts.string("expr_b64"))
	if err != nil || len(raw) == 0 {
		return 0, 0, false
	}
	expression := string(raw)
	program, err := billingexpr.CompileFromCache(expression)
	if err != nil || billingexpr.UsesFixedPricing(expression) || billingexpr.UsesExchangeRate(expression) ||
		ast.Find(program.Node(), historicalBillingProbe) != nil {
		return 0, 0, false
	}
	used := billingexpr.UsedVars(expression)
	if !used["img_o"] || used["image_count"] {
		return 0, 0, false
	}
	known := make(map[string]bool, len(used))
	for name, active := range used {
		if name != "img_o" {
			known[name] = active
		}
	}
	params, ok := historicalCustomerTokenParams(log, facts, known)
	contract, contractOK := historicalStatementContractRatio(parsed)
	tier := facts.string("matched_tier")
	if !ok || !contractOK || tier == "" {
		return 0, 0, false
	}
	group := *parsed.discountRatio
	snapshot := billingexpr.BillingSnapshot{
		ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), ExprVersion: billingexpr.ExprVersion(expression),
		GroupRatio: group, QuotaPerUnit: historicalCustomerQuotaPerUnit,
	}
	groupRatio := decimal.NewFromFloat(group)
	found := -1
	var original int64
	for candidate := 0; candidate <= log.CompletionTokens; candidate++ {
		params.ImgO, params.C = float64(candidate), float64(log.CompletionTokens-candidate)
		result, err := billingexpr.ComputeTieredQuota(&snapshot, params)
		if err != nil || result.Clamp != nil || result.ActualQuotaBeforeGroup < 0 || result.BillingUnit != billingexpr.BillingUnitToken {
			return 0, 0, false
		}
		if result.MatchedTier != tier {
			continue
		}
		amount, err := common.QuotaRoundStrict(result.ActualQuotaBeforeGroup)
		if err != nil {
			return 0, 0, false
		}
		booked := result.ActualQuotaAfterGroup
		if parsed.contractDiscountRatio != nil {
			booked, err = common.QuotaRoundStrict(decimal.NewFromFloat(result.ActualQuotaBeforeGroup).Mul(groupRatio).Mul(contract).InexactFloat64())
			if err != nil {
				return 0, 0, false
			}
		}
		if booked == log.Quota {
			if found >= 0 {
				return 0, 0, false
			}
			original, found = int64(amount), candidate
		}
	}
	return original, found, found >= 0
}
