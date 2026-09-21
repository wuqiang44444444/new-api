package model

import (
	"encoding/base64"
	"encoding/json"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/expr-lang/expr/ast"
	"github.com/shopspring/decimal"
)

// Historical replay requires only the inputs used by the frozen rule. A missing
// format version alone is not a missing price. It never reads today's prices or
// modifies the recorded test fee. Native expression evaluation remains authoritative.
func replayHistoricalTestAmount(log billingReconciliationLog, parsed parsedBillingReconciliationLog) upstreamTestAmount {
	pending := upstreamTestAmount{pending: true, reason: "invalid_pricing_record"}
	var other map[string]json.RawMessage
	if common.UnmarshalJsonStr(log.Other, &other) != nil || log.PromptTokens < 0 || log.CompletionTokens < 0 || log.Quota > common.MaxQuota || parsed.contractApplicable {
		return pending
	}
	var admin map[string]json.RawMessage
	if len(other["admin_info"]) > 0 && common.Unmarshal(other["admin_info"], &admin) != nil {
		return pending
	}
	if len(admin["quota_saturation"]) > 0 {
		return pending
	}
	if raw := admin["local_count_tokens"]; len(raw) > 0 {
		if _, valid := billingReconciliationBool(raw); !valid {
			return pending
		}
	}
	if estimated, _ := billingReconciliationBool(admin["local_count_tokens"]); estimated {
		pending.reason = "estimated_usage"
		return pending
	}
	if parsed.hasAuxiliaryCharge {
		pending.reason = "missing_tool_price"
		return pending
	}
	semantic := historicalTestUsageSemantic(other)
	semanticKnown := semantic == "openai" || semantic == "anthropic" || semantic == "gemini" || semantic == "embedding"
	p, c := float64(log.PromptTokens), float64(log.CompletionTokens)
	cr, crKnown := billingBreakdownNonNegativeInt(other["cache_tokens"])
	cc := parsed.cacheWrite
	if parsed.hasExpression {
		decoded, err := base64.StdEncoding.DecodeString(billingBreakdownString(other["expr_b64"]))
		if err != nil {
			return pending
		}
		expression := string(decoded)
		prog, err := billingexpr.CompileFromCache(expression)
		if err != nil {
			return pending
		}
		// These functions require their original request or pricing time. A log's
		// completion timestamp is not proof of the exact time the rule evaluated.
		if ast.Find(prog.Node(), func(n ast.Node) bool {
			id, ok := n.(*ast.IdentifierNode)
			if !ok {
				return false
			}
			switch id.Value {
			case "param", "header", "hour", "minute", "weekday", "month", "day", "u":
				return true
			}
			return false
		}) != nil {
			pending.reason = "missing_expression_context"
			return pending
		}
		vars := billingexpr.UsedVars(expression)
		// Plain p/c rules use the recorded counts directly. Only normalization
		// or context-length rules need to know how subcategories are included.
		if !semanticKnown && (vars["len"] || vars["cr"] || vars["cc"] || vars["cc1h"] || vars["img"] || vars["img_o"] || vars["ai"] || vars["ao"]) {
			pending.reason = "missing_usage_semantic"
			return pending
		}
		if vars["img"] || vars["img_o"] || vars["ai"] || vars["ao"] {
			pending.reason = "missing_multimodal_usage"
			return pending
		}
		if (vars["cr"] || (vars["len"] && semantic == "anthropic")) && !crKnown {
			pending.reason = "missing_cache_read"
			return pending
		}
		needsClaudeCache := semantic == "anthropic" && (vars["cc"] || vars["cc1h"] || vars["len"])
		if (vars["cc"] || needsClaudeCache) && !cc.known {
			pending.reason = "missing_cache_write"
			return pending
		}
		cc5, cc1 := float64(cc.total), float64(0)
		if semantic == "anthropic" && (vars["cc"] || vars["cc1h"]) {
			cc5raw, ok5 := billingBreakdownNonNegativeInt(other["cache_creation_tokens_5m"])
			cc1raw, ok1 := billingBreakdownNonNegativeInt(other["cache_creation_tokens_1h"])
			if cc.total > 0 && ((vars["cc"] && !ok5) || (vars["cc1h"] && !ok1)) {
				pending.reason = "missing_cache_ttl"
				return pending
			}
			cc5, cc1 = float64(cc5raw), float64(cc1raw)
		}
		inputLen := p
		if semantic == "anthropic" {
			inputLen += float64(cr) + float64(cc.total)
		} else {
			if vars["cr"] {
				p -= float64(cr)
			}
			if vars["cc"] {
				p -= cc5
			}
			if vars["cc1h"] {
				p -= cc1
			}
		}
		if parsed.discountRatio == nil || *parsed.discountRatio <= 0 {
			pending.reason = "missing_group_ratio"
			return pending
		}
		tr, err := billingexpr.ComputeTieredQuota(&billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), ExprVersion: billingexpr.ExprVersion(expression), GroupRatio: *parsed.discountRatio, QuotaPerUnit: common.QuotaPerUnit}, billingexpr.TokenParams{P: max(p, 0), C: c, Len: inputLen, CR: float64(cr), CC: cc5, CC1h: cc1})
		if err != nil || tr.Clamp != nil || math.IsNaN(tr.ActualQuotaBeforeGroup) || math.IsInf(tr.ActualQuotaBeforeGroup, 0) || tr.ActualQuotaBeforeGroup < 0 {
			return pending
		}
		original, clamp := common.QuotaRoundChecked(tr.ActualQuotaBeforeGroup)
		if clamp != nil {
			return pending
		}
		if tr.ActualQuotaAfterGroup != log.Quota {
			pending.reason = "recorded_amount_mismatch"
			return pending
		}
		return upstreamTestAmount{known: true, original: decimal.NewFromInt(int64(original)), replayed: true, recorded: int64(log.Quota)}
	}
	if !semanticKnown {
		pending.reason = "missing_usage_semantic"
		return pending
	}
	// Old text tests contained a fixed text-only probe. Image endpoints have a
	// separate payload/usage contract and cannot borrow that zero-media rule.
	path := billingBreakdownString(other["request_path"])
	if path != "/v1/chat/completions" && path != "/v1/messages" && path != "/v1/responses" && !(semantic == "embedding" && path == "/v1/embeddings" && log.CompletionTokens == 0) {
		pending.reason = "missing_multimodal_usage"
		return pending
	}
	if !crKnown {
		pending.reason = "missing_cache_read"
		return pending
	}
	if !cc.known {
		pending.reason = "missing_cache_write"
		return pending
	}
	if cc.total != 0 {
		pending.reason = "missing_cache_write_price"
		return pending
	}
	if parsed.modelRatio == nil || parsed.completionRatio == nil || *parsed.modelRatio < 0 || *parsed.completionRatio < 0 {
		pending.reason = "missing_price_fields"
		return pending
	}
	cacheRatio, ok := billingReconciliationFloat(other["cache_ratio"])
	if cr > 0 && (!ok || cacheRatio < 0) {
		pending.reason = "missing_cache_read_price"
		return pending
	}
	// Verify the old simplified quote before correcting its omission of cache
	// discounts and its intermediate rounding. All conversions fail on clamp.
	completion, clamp := common.QuotaRoundChecked(c * *parsed.completionRatio)
	if clamp != nil {
		return pending
	}
	legacy, clamp := common.QuotaRoundChecked((p + float64(completion)) * *parsed.modelRatio)
	if clamp != nil {
		return pending
	}
	if *parsed.modelRatio != 0 && legacy <= 0 {
		legacy = 1
	}
	if legacy != log.Quota {
		pending.reason = "recorded_amount_mismatch"
		return pending
	}
	base := decimal.NewFromInt(int64(log.PromptTokens))
	if semantic != "anthropic" {
		base = base.Sub(decimal.NewFromInt(cr))
	}
	if base.IsNegative() {
		base = decimal.Zero
	}
	amount := base.Add(decimal.NewFromInt(cr).Mul(decimal.NewFromFloat(cacheRatio))).Add(decimal.NewFromInt(int64(log.CompletionTokens)).Mul(decimal.NewFromFloat(*parsed.completionRatio))).Mul(decimal.NewFromFloat(*parsed.modelRatio))
	quota, clamp := common.QuotaFromDecimalChecked(amount)
	if clamp != nil {
		return pending
	}
	if *parsed.modelRatio != 0 && quota == 0 && (log.PromptTokens > 0 || log.CompletionTokens > 0) {
		quota = 1
	}
	return upstreamTestAmount{known: true, original: decimal.NewFromInt(int64(quota)), replayed: true, recorded: int64(log.Quota)}
}

// A successful historical expression test already recorded its engine result.
// Its writer injected expr_b64 + matched_tier only after successful evaluation
// (never on the estimate fallback). With an explicit group factor of exactly 1,
// that result IS the rounded original; no division or missing-meter inference
// is involved. Replay contradictions and explicit estimates always take priority.
func historicalChannelTestAmount(log billingReconciliationLog, parsed parsedBillingReconciliationLog) upstreamTestAmount {
	amount := replayHistoricalTestAmount(log, parsed)
	if amount.known || !parsed.hasExpression {
		return amount
	}
	switch amount.reason {
	case "missing_cache_read", "missing_cache_write", "missing_cache_ttl", "missing_multimodal_usage", "missing_usage_semantic", "missing_expression_context":
	default:
		return amount
	}
	var other map[string]json.RawMessage
	if common.UnmarshalJsonStr(log.Other, &other) != nil {
		return amount
	}
	var admin map[string]json.RawMessage
	_ = common.Unmarshal(other["admin_info"], &admin)
	if estimated, _ := billingReconciliationBool(admin["local_count_tokens"]); estimated {
		return upstreamTestAmount{pending: true, reason: "estimated_usage"}
	}
	if len(admin["quota_saturation"]) > 0 || parsed.hasAuxiliaryCharge || log.Quota < 0 || log.Quota > common.MaxQuota {
		return amount
	}
	group, ok := billingReconciliationFloat(other["group_ratio"])
	if !ok || group != 1 || parsed.discountRatio == nil || *parsed.discountRatio != 1 {
		return amount
	}
	if parsed.contractApplicable {
		return amount
	}
	var tier string
	if common.Unmarshal(other["matched_tier"], &tier) != nil || billingBreakdownString(other["billing_mode"]) != "tiered_expr" {
		return amount
	}
	decoded, err := base64.StdEncoding.DecodeString(billingBreakdownString(other["expr_b64"]))
	if err != nil {
		return amount
	}
	if _, err = billingexpr.CompileFromCache(string(decoded)); err != nil {
		return amount
	}
	return upstreamTestAmount{known: true, original: decimal.NewFromInt(int64(log.Quota)), recordedOriginal: true, recorded: int64(log.Quota)}
}
