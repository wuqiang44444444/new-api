package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
)

// appendChannelTestCacheUsage preserves the response facts used to price a
// channel test. Logging must not discard writes simply because the test uses
// GenerateTextOtherInfo for every response format.
func appendChannelTestCacheUsage(other *model.LogOther, usage *dto.Usage) {
	if usage.CacheReadTokensReported != nil {
		other.SetPublic("cache_read_tokens_reported", *usage.CacheReadTokensReported)
	}
	if usage.CacheWriteTokensReported != nil {
		other.SetPublic("cache_write_tokens_reported", *usage.CacheWriteTokensReported)
	}
	creationTokens := usage.PromptTokensDetails.CacheCreationTokensTotal()
	other.SetPublic("cache_creation_tokens", creationTokens)
	other.SetPublic("cache_creation_tokens_5m", usage.ClaudeCacheCreation5mTokens)
	other.SetPublic("cache_creation_tokens_1h", usage.ClaudeCacheCreation1hTokens)
	other.SetPublic("cache_write_tokens", max(creationTokens, usage.ClaudeCacheCreation5mTokens+usage.ClaudeCacheCreation1hTokens))
	if usage.UsageSemantic != "" {
		other.SetPublic("usage_semantic", usage.UsageSemantic)
	}
}

// Canonical billing evidence takes precedence over a transient local estimate.
// A plain usage object is not proof of metering when the adaptor counted locally.
func isTestUsageValue(usageAny any, localCount bool) bool {
	var usage *dto.Usage
	switch value := usageAny.(type) {
	case *dto.Usage:
		usage = value
	case dto.Usage:
		usage = &value
	}
	if usage == nil {
		return false
	}
	if usage.BillingUsage != nil {
		_, valid := usage.BillingUsage.CanonicalUsage()
		return valid && !usage.BillingUsage.Estimated
	}
	return !localCount
}

// appendChannelTestPricing persists the test pricing evidence at the test fee
// record boundary: the pricing mode, whether the fee was settled from actual
// usage, and the engine's pre-discount original where one exists. Expression
// failures and estimated usage keep status "estimated" without an original.
func appendChannelTestPricing(other *model.LogOther, info *relaycommon.RelayInfo, quota int, tieredResult *billingexpr.TieredResult, priceData hosttypes.PriceData, usageEstimated bool) string {
	mode, status := "", "settled"
	original := quota
	switch {
	case info != nil && info.TieredBillingSnapshot != nil && info.TieredBillingSnapshot.BillingMode == "tiered_expr":
		mode = "tiered_expr"
		if tieredResult == nil || usageEstimated {
			status = "estimated"
			original = 0
		}
	case priceData.UsePrice:
		mode = "fixed_price"
	default:
		mode = "ratio"
		if usageEstimated {
			status = "estimated"
			original = 0
		}
	}
	if info != nil && info.QuotaClamp != nil {
		status = "estimated"
	}
	record := map[string]any{"version": 1, "mode": mode, "status": status}
	if status == "settled" {
		record["original_quota"] = original
	}
	if mode == "tiered_expr" && info != nil && info.TieredBillingSnapshot != nil {
		record["expr_version"] = info.TieredBillingSnapshot.ExprVersion
	}
	other.SetPublic("test_pricing", record)
	other.SetPublic("cache_creation_ratio", priceData.CacheCreationRatio)
	other.SetPublic("cache_creation_ratio_5m", priceData.CacheCreation5mRatio)
	other.SetPublic("cache_creation_ratio_1h", priceData.CacheCreation1hRatio)
	other.SetPublic("image_ratio", priceData.ImageRatio)
	return status
}
