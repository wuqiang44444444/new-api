package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Replay the historical synchronous text writer's token-ratio branch. That
// branch does not apply request price factors used by native per-call billing.
func replayNativeRatioOriginal(log billingReconciliationLog, facts customerExportFacts, parsed parsedBillingReconciliationLog) (int64, string) {
	path, semantic := facts.string("request_path"), facts.string("usage_semantic")
	textWriter := (path == "/v1/chat/completions" || strings.HasPrefix(path, "/v1/images/")) && semantic == "openai" ||
		(strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent")) && semantic == "gemini"
	price, priceKnown := facts.float("model_price")
	if !textWriter || !priceKnown || price != -1 || parsed.billingMode != BillingReconciliationModeToken ||
		parsed.hasAuxiliaryCharge || len(facts.other["other_ratios"]) != 0 || log.PromptTokens < 0 || log.CompletionTokens < 0 {
		return 0, BillingOriginalReplayUnverified
	}
	model, modelOK := facts.float("model_ratio")
	completion, completionOK := facts.float("completion_ratio")
	cacheRatio, cacheRatioOK := facts.float("cache_ratio")
	cache, cacheOK := billingBreakdownNonNegativeInt(facts.other["cache_tokens"])
	group := *parsed.discountRatio
	contract, contractOK := historicalStatementContractRatio(parsed)
	if !modelOK || !completionOK || !cacheRatioOK || !cacheOK || !contractOK ||
		model < 0 || completion < 0 || cacheRatio < 0 || group < 0 {
		return 0, BillingOriginalReplayMissingContext
	}
	for _, split := range []string{"cache_creation_tokens_5m", "cache_creation_tokens_1h"} {
		if len(facts.other[split]) != 0 {
			return 0, BillingOriginalReplayMissingUsage
		}
	}
	cacheWrite, cacheWriteOK := historicalNativeMeter(facts, "cache_creation_tokens")
	image, imageOK := historicalNativeMeter(facts, "image_output")
	audio, audioOK := historicalNativeMeter(facts, "audio_input_token_count")
	if !cacheWriteOK || !imageOK || !audioOK {
		return 0, BillingOriginalReplayMissingUsage
	}
	if len(facts.other["image"]) != 0 || image > 0 {
		marker, ok := billingReconciliationBool(facts.other["image"])
		if !ok || !marker || image == 0 {
			return 0, BillingOriginalReplayMissingUsage
		}
	}
	cacheWriteRatio, writeRatioOK := facts.float("cache_creation_ratio")
	imageRatio, imageRatioOK := facts.float("image_ratio")
	audioPrice, audioPriceOK := facts.float("audio_input_price")
	if cacheWrite > 0 && (!writeRatioOK || cacheWriteRatio < 0) ||
		image > 0 && (!imageRatioOK || imageRatio < 0) ||
		audio > 0 && (!audioPriceOK || audioPrice <= 0) || audio == 0 && len(facts.other["audio_input_price"]) != 0 {
		return 0, BillingOriginalReplayMissingContext
	}
	if cacheWrite == 0 {
		cacheWriteRatio = 0
	}
	if image == 0 {
		imageRatio = 0
	}
	if audio == 0 {
		audioPrice = 0
	}

	base := decimal.NewFromInt(int64(log.PromptTokens)).Sub(decimal.NewFromInt(cache)).
		Sub(decimal.NewFromInt(cacheWrite)).Sub(decimal.NewFromInt(image)).Sub(decimal.NewFromInt(audio))
	weighted := decimal.Max(decimal.Zero, base).Add(decimal.NewFromInt(cache).Mul(decimal.NewFromFloat(cacheRatio))).
		Add(decimal.NewFromInt(cacheWrite).Mul(decimal.NewFromFloat(cacheWriteRatio))).
		Add(decimal.NewFromInt(image).Mul(decimal.NewFromFloat(imageRatio))).
		Add(decimal.NewFromInt(int64(log.CompletionTokens)).Mul(decimal.NewFromFloat(completion)))
	audioOriginal := decimal.NewFromFloat(audioPrice).Div(decimal.NewFromInt(1_000_000)).
		Mul(decimal.NewFromInt(audio)).Mul(decimal.NewFromInt(historicalCustomerQuotaPerUnit))
	originalAmount := weighted.Mul(decimal.NewFromFloat(model)).Add(audioOriginal)
	groupRatio := decimal.NewFromFloat(group)
	bookedAmount := weighted.Mul(decimal.NewFromFloat(model).Mul(groupRatio)).
		Add(audioOriginal.Mul(groupRatio)).Mul(contract)
	if model > 0 && group > 0 && !bookedAmount.IsPositive() {
		bookedAmount = decimal.NewFromInt(1)
	}
	original, originalErr := common.QuotaFromDecimalStrict(originalAmount)
	booked, bookedErr := common.QuotaFromDecimalStrict(bookedAmount)
	if originalErr != nil || bookedErr != nil {
		return 0, BillingOriginalReplayMismatch
	}
	if log.PromptTokens == 0 && log.CompletionTokens == 0 {
		original, booked = 0, 0
	} else if model > 0 {
		original = max(1, original)
		if group > 0 {
			booked = max(1, booked)
		}
	}
	if booked != log.Quota {
		return 0, BillingOriginalReplayMismatch
	}
	return int64(original), ""
}

func historicalNativeMeter(facts customerExportFacts, name string) (int64, bool) {
	raw := facts.other[name]
	if len(raw) == 0 {
		return 0, true
	}
	return billingBreakdownNonNegativeInt(raw)
}
