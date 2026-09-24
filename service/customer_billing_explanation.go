package service

import (
	"encoding/base64"
	"math"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type customerBillingLine struct {
	Label     string  `json:"label"`
	Quantity  float64 `json:"quantity"`
	Unit      string  `json:"unit"`
	UnitPrice float64 `json:"unit_price_usd"`
	Subtotal  float64 `json:"subtotal_usd"`
}

// Response-only arithmetic explanation. Price extraction stays in billingexpr;
// normalization stays in BuildTieredTokenParams. No expression is re-executed.
func attachCustomerBillingExplanations(logs []*model.Log) {
	for _, log := range logs {
		if log.Type != model.LogTypeConsume {
			continue
		}
		var other map[string]any
		if common.UnmarshalJsonStr(log.Other, &other) != nil || other == nil {
			continue
		}
		row := model.CustomerBillingLogRow(log)
		lines := customerBillingLines(log, other, row, common.QuotaPerUnit)
		if lines == nil {
			lines = []customerBillingLine{}
		}
		explanation := map[string]any{"lines": lines, "original_quota_estimated": row.OriginalEstimate, "has_auxiliary_charge": row.HasAuxiliaryCharge}
		if len(lines) > 0 && row.BillingMode == model.BillingReconciliationModeToken && other["expr_b64"] == nil {
			explanation["current_quota_conversion"] = true
		}
		other["billing_explanation"] = explanation
		if raw, err := common.Marshal(other); err == nil {
			log.Other = string(raw)
		}
	}
}

func customerBillingNumber(other map[string]any, key string) (float64, bool) {
	value, ok := other[key].(float64)
	return value, ok && value >= 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func customerBillingLines(log *model.Log, other map[string]any, row model.CustomerExportRow, quotaPerUnit float64) []customerBillingLine {
	lines := make([]customerBillingLine, 0)
	// Refunds and adjustments are ledger deltas, not new metered requests.
	if row.RequestCount == 0 {
		return lines
	}
	if encoded, ok := other["expr_b64"].(string); ok && encoded != "" {
		expression, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return lines
		}
		// 历史账单按日志冻结汇率展开；未记录汇率的旧日志明确不可展开。
		frozenRate := frozenLogExchangeRate(other)
		projection, err := billingexpr.DisplayProjectionForWithRate(string(expression), frozenRate)
		units := make(map[string]billingexpr.TaskUsageFieldInfo)
		if rawUnits, ok := other["usage_units"].(map[string]any); ok {
			for name, value := range rawUnits {
				if unit, ok := value.(string); ok {
					units[name] = billingexpr.TaskUsageFieldInfo{Unit: unit}
				}
			}
			projection, err = billingexpr.TaskDisplayProjectionForWithRate(string(expression), units, frozenRate)
		}
		if err != nil || projection.Status != billingexpr.DisplayStatusExact {
			return lines
		}
		tiers := projection.Tiers
		if len(projection.Rules) > 0 {
			if len(projection.Rules) != 1 {
				return lines
			}
			traces, _ := other["request_rules"].([]any)
			matches := 0
			for _, raw := range traces {
				trace, ok := raw.(map[string]any)
				if !ok || trace["cond"] != projection.Rules[0].Text {
					continue
				}
				matched, ok := trace["matched"].(bool)
				if !ok {
					return lines
				}
				for _, scenario := range projection.Scenarios {
					if scenario.Matched == matched {
						tiers = scenario.Tiers
						matches++
					}
				}
			}
			if matches != 1 {
				return lines
			}
		}
		matched, _ := other["matched_tier"].(string)
		var tier *billingexpr.DisplayTier
		for i := range tiers {
			if tiers[i].Label == matched {
				if tier != nil {
					return lines
				}
				tier = &tiers[i]
			}
		}
		if tier == nil {
			return lines
		}
		quantities := make(map[string]float64)
		labels := map[string]string{"p": "Input", "c": "Output", "cr": "Cache Read", "cc": "Cache Write", "cc1h": "Cache Write (1h)", "img": "Image In", "img_o": "Image Out", "ai": "Audio In", "ao": "Audio Out"}
		if len(units) > 0 {
			if facts, ok := other["usage_facts"].(map[string]any); ok {
				for key := range tier.UnitPrices {
					if value, ok := customerBillingNumber(facts, key); ok {
						quantities[key] = value
					}
				}
			}
		} else {
			usage := dto.Usage{PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens}
			semantic, _ := other["usage_semantic"].(string)
			claude := semantic == "anthropic" || other["claude"] == true
			usage.UsageSemantic = semantic
			if claude {
				usage.UsageSemantic = "anthropic"
			}
			usage.PromptTokensDetails.CachedTokens = int(row.CacheReadTokens)
			// Missing multimodal dimensions cannot be reconstructed from totals.
			used := billingexpr.UsedVars(string(expression))
			for _, key := range []string{"img", "img_o", "ai", "ao"} {
				if used[key] {
					return lines
				}
			}
			if row.InputTokensUnavailable {
				return lines
			}
			usage.PromptTokens = int(row.InputTokens)
			if claude {
				usage.PromptTokens = int(row.InputTokens - row.CacheReadTokens - row.CacheWriteTokens)
			}
			cc5, has5 := customerBillingNumber(other, "cache_creation_tokens_5m")
			cc1, has1 := customerBillingNumber(other, "cache_creation_tokens_1h")
			if row.CacheWriteTokens > 0 && claude && ((!has5 && !has1) || cc5+cc1 != float64(row.CacheWriteTokens)) {
				return lines
			}
			usage.PromptTokensDetails.CacheWriteTokens = int(row.CacheWriteTokens)
			usage.ClaudeCacheCreation5mTokens = int(cc5)
			usage.ClaudeCacheCreation1hTokens = int(cc1)
			params := BuildTieredTokenParams(&usage, claude, used)
			quantities = map[string]float64{"p": params.P, "c": params.C, "cr": params.CR, "cc": params.CC, "cc1h": params.CC1h}
		}
		keys := make([]string, 0, len(tier.UnitPrices))
		for key := range tier.UnitPrices {
			keys = append(keys, key)
		}
		priority := map[string]int{"p": 1, "c": 2, "cr": 3, "cc": 4, "cc1h": 5}
		sort.Slice(keys, func(i, j int) bool {
			if priority[keys[i]] != priority[keys[j]] {
				return priority[keys[i]] < priority[keys[j]]
			}
			return keys[i] < keys[j]
		})
		hasPositiveSubtotal := false
		for _, key := range keys {
			quantity, known := quantities[key]
			if !known {
				return nil
			}
			unit, label := "token", labels[key]
			if len(units) > 0 {
				unit = units[key].Unit
				label = key
			}
			if label == "" {
				label = key
			}
			price := tier.UnitPrices[key]
			subtotal := quantity * price
			if unit == "token" {
				subtotal /= 1000000
			}
			if math.IsNaN(subtotal) || math.IsInf(subtotal, 0) || subtotal < 0 {
				return nil
			}
			hasPositiveSubtotal = hasPositiveSubtotal || subtotal > 0
			lines = append(lines, customerBillingLine{label, quantity, unit, price, subtotal})
		}
		constant := tier.Constant
		if projection.ConstantCharge != nil {
			constant += *projection.ConstantCharge
		}
		if constant > 0 {
			lines = append(lines, customerBillingLine{"Additional charge", 1, "request", constant, constant})
			hasPositiveSubtotal = true
		}
		// Old task logs can contain an expression but no frozen usage. Zero
		// columns are not evidence of a free request when a charge was recorded.
		if row.Quota > 0 && !hasPositiveSubtotal {
			return nil
		}
		// Known model-only settlements use the same reconciliation as native
		// pricing. Unknown discounts or separate fees allow only independently
		// proven components; they cannot establish the final charged amount.
		if row.FinalRatio != nil && !row.HasAuxiliaryCharge {
			return reconciledCustomerBillingLines(lines, row, quotaPerUnit)
		}
		return lines
	}
	if row.BillingMode == model.BillingReconciliationModePerCall {
		if price, ok := customerBillingNumber(other, "model_price"); ok {
			lines = append(lines, customerBillingLine{"Model Price", float64(row.RequestCount), "request", price, price * float64(row.RequestCount)})
		}
		return reconciledCustomerBillingLines(lines, row, quotaPerUnit)
	}
	// Native token prices require a known input total and frozen facts for
	// each separately priced dimension. Cache normalization remains unsupported here.
	if row.InputTokensUnavailable || row.CacheReadTokens > 0 || row.CacheWriteTokens > 0 || row.HasAuxiliaryCharge {
		return lines
	}
	price, ok := customerBillingNumber(other, "model_ratio")
	if !ok {
		return lines
	}
	if quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return lines
	}
	// Native model_ratio is quota/token. USD here follows the same current
	// quota conversion as the statement, not an invented historical USD price.
	price *= 1000000 / quotaPerUnit
	input := float64(row.InputTokens)
	var dimensions []customerBillingLine
	if other["image"] == true {
		quantity, hasQuantity := customerBillingNumber(other, "image_output")
		ratio, hasRatio := customerBillingNumber(other, "image_ratio")
		if !hasQuantity || !hasRatio || quantity > input {
			return lines
		}
		input -= quantity
		dimensions = append(dimensions, customerBillingLine{"Image In", quantity, "token", price * ratio, quantity * price * ratio / 1000000})
	}
	if other["audio_input_seperate_price"] == true {
		quantity, hasQuantity := customerBillingNumber(other, "audio_input_token_count")
		audioPrice, hasPrice := customerBillingNumber(other, "audio_input_price")
		if !hasQuantity || !hasPrice || quantity > input {
			return lines
		}
		input -= quantity
		dimensions = append(dimensions, customerBillingLine{"Audio In", quantity, "token", audioPrice, quantity * audioPrice / 1000000})
	}
	lines = append(lines, customerBillingLine{"Input", input, "token", price, input * price / 1000000})
	if row.OutputTokens > 0 {
		ratio, ok := customerBillingNumber(other, "completion_ratio")
		if !ok {
			return nil
		}
		lines = append(lines, customerBillingLine{"Output", float64(row.OutputTokens), "token", price * ratio, float64(row.OutputTokens) * price * ratio / 1000000})
	}
	lines = append(lines, dimensions...)
	return reconciledCustomerBillingLines(lines, row, quotaPerUnit)
}

// Historical logs do not freeze every other multiplier. Only publish a
// complete arithmetic explanation when it reconciles to the settled quota
// within one quota unit (native truncation/minimum charge). Never backsolve a
// missing multiplier or overwrite the settled amount to make an equation fit.
func reconciledCustomerBillingLines(lines []customerBillingLine, row model.CustomerExportRow, quotaPerUnit float64) []customerBillingLine {
	if row.FinalRatio == nil || row.HasAuxiliaryCharge || row.QualityStatus == "unrecorded" || quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return nil
	}
	total := decimal.Zero
	for _, line := range lines {
		if math.IsNaN(line.Subtotal) || math.IsInf(line.Subtotal, 0) {
			return nil
		}
		total = total.Add(decimal.NewFromFloat(line.Subtotal))
	}
	if total.IsZero() && row.Quota > 0 {
		return nil
	}
	quota := total.Mul(decimal.NewFromFloat(*row.FinalRatio)).Mul(decimal.NewFromFloat(quotaPerUnit))
	if quota.Sub(decimal.NewFromInt(row.Quota)).Abs().GreaterThan(decimal.NewFromInt(1)) {
		return nil
	}
	return lines
}
