package service

import (
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewReturnsActualUSDAndQuota(t *testing.T) {
	result := PreviewBillingExpressions([]BillingExprPreviewItem{{Key: "usd", Expression: `tier("base", p*2)`, Sample: &BillingExprPreviewSample{PromptTokens: 1_000_000}}})
	require.Empty(t, result[0].Error)
	require.NotNil(t, result[0].Evaluation)
	assert.Equal(t, 2.0, result[0].Evaluation.RawCostUSD)
	assert.Equal(t, common.QuotaRound(2*common.QuotaPerUnit), result[0].Evaluation.Quota)
}

func TestPreviewRejectsInvalidQuantities(t *testing.T) {
	for _, value := range []int64{-1, math.MaxInt32 + 1} {
		result := PreviewBillingExpressions([]BillingExprPreviewItem{{Expression: `tier("base", cr*2)`, Sample: &BillingExprPreviewSample{CacheReadTokens: value}}})
		assert.NotEmpty(t, result[0].Error)
		assert.Nil(t, result[0].Evaluation)
	}
}

func TestContractPriceResponseIncludesFrozenDisplay(t *testing.T) {
	price := buildCustomerContractPricePreview(model.Pricing{BillingMode: "tiered_expr", BillingExpr: flashDisplayExpr}, decimal.NewFromFloat(0.87))
	require.NotNil(t, price.BillingDisplay)
	require.Len(t, price.BillingDisplay.Scenarios, 2)
	// Contract multiplier remains outside the immutable base projection.
	assert.InDelta(t, 3/6.71, price.BillingDisplay.Scenarios[1].Tiers[0].UnitPrices["p"], 1e-12)
	encoded, err := common.Marshal(price)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"billing_display"`)
	assert.Contains(t, string(encoded), `"scenarios"`)
}

func TestPreviewBothOriginalModelsAndEditableExchangeRate(t *testing.T) {
	for _, rate := range []string{"6.71", "6.9"} {
		for _, pro := range []bool{false, true} {
			expression := strings.ReplaceAll(flashPreviewExpr, "6.71", rate)
			factor := 1.0
			if pro {
				expression = strings.ReplaceAll(expression, "p * 1.5 + cr * 0.05 + c * 4.5", "p * 4.5 + cr * 0.15 + c * 13.5")
				factor = 3
			}
			divisor := 6.71
			if rate == "6.9" {
				divisor = 6.9
			}
			for _, at := range []string{"2026-09-07T08:59:59+08:00", "2026-09-07T09:00:00+08:00", "2026-09-12T10:00:00+08:00"} {
				result := mustEvaluatePreview(t, expression, flashPreviewSample(at))
				want := 1.66 / divisor * factor
				if strings.Contains(at, "T09:00") {
					want *= 2
				}
				assert.InDelta(t, want, result.Evaluation.RawCostUSD, 1e-12)
			}
		}
	}
}

func TestPreviewCacheCreationMatchesSettlementUsage(t *testing.T) {
	for _, semantic := range []string{"openai", "anthropic"} {
		t.Run(semantic, func(t *testing.T) {
			result := PreviewBillingExpressions([]BillingExprPreviewItem{{
				Expression: `tier("base", p * 1 + cc * 2)`,
				Sample:     &BillingExprPreviewSample{UsageSemantic: semantic, PromptTokens: 1000, CacheCreationTokens: 200},
			}})[0]
			require.Empty(t, result.Error)
			require.NotNil(t, result.Evaluation)
			wantP, wantUSD := 800.0, 0.0012
			if semantic == "anthropic" {
				wantP, wantUSD = 1000, 0.0014
			}
			assert.Equal(t, wantP, result.Evaluation.Normalized.P)
			assert.Equal(t, 200.0, result.Evaluation.Normalized.CC)
			assert.InDelta(t, wantUSD, result.Evaluation.RawCostUSD, 1e-12)
			assert.Equal(t, common.QuotaRound(wantUSD*common.QuotaPerUnit), result.Evaluation.Quota)
		})
	}
}

func TestPreviewRejectsAnthropicOnlyCacheTTLForOpenAI(t *testing.T) {
	result := PreviewBillingExpressions([]BillingExprPreviewItem{{Expression: `tier("base", p + cc1h * 2)`, Sample: &BillingExprPreviewSample{PromptTokens: 1000, CacheCreation1hTokens: 200}}})[0]
	assert.NotEmpty(t, result.Error)
	assert.Nil(t, result.Evaluation)
}

func TestPreviewZeroUsageStillChargesFixedFee(t *testing.T) {
	result := PreviewBillingExpressions([]BillingExprPreviewItem{{Expression: `tier("base", p * 2 + 10000)`, Sample: &BillingExprPreviewSample{}}})[0]
	require.Empty(t, result.Error)
	require.NotNil(t, result.Evaluation)
	assert.Equal(t, 0.01, result.Evaluation.RawCostUSD)
	assert.Equal(t, common.QuotaRound(0.01*common.QuotaPerUnit), result.Evaluation.Quota)
}
