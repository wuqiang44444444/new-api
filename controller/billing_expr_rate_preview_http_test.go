package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const exchangeRatePreviewExpression = `tier("standard", (p * 12 + c * 36 + cr * 1 + cc * 15) / usd_exchange_rate())`

func TestBillingPreviewHTTPExchangeRateAndNormalizedTokens(t *testing.T) {
	prior := operation_setting.USDExchangeRate
	t.Cleanup(func() { require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(prior))) })

	for _, rate := range []float64{7, 10} {
		require.NoError(t, operation_setting.SetUSDExchangeRate(fmt.Sprint(rate)))
		result := billingRatePreviewHTTP(t, exchangeRatePreviewExpression, map[string]any{
			"usage_semantic":   "normalized",
			"normalized_usage": map[string]any{"p": 1000000, "c": 100000, "cr": 200000, "cc": 50000, "len": 1250000},
			// Request context cannot replace the authoritative system rate.
			"body": map[string]any{"usd_exchange_rate": 1},
		})
		require.Empty(t, result.Error)
		require.NotNil(t, result.Evaluation)
		assert.InDelta(t, 16.55/rate, result.Evaluation.RawCostUSD, 1e-12)
		assert.Equal(t, common.QuotaRound(16.55/rate*common.QuotaPerUnit), result.Evaluation.Quota)
		assert.Equal(t, 1000000.0, result.Evaluation.Normalized.P, "cache must not be subtracted twice")
		assert.Equal(t, 1250000.0, result.Evaluation.Normalized.Len)
		assert.Equal(t, "standard", result.Evaluation.MatchedTier)
		require.NotNil(t, result.Evaluation.UsdExchangeRate)
		assert.Equal(t, rate, result.Evaluation.UsdExchangeRate.Rate)
		assert.Equal(t, billingexpr.UsdExchangeRateSourceKey, result.Evaluation.UsdExchangeRate.SourceKey)
		assert.False(t, result.Evaluation.UsdExchangeRate.FrozenAt.IsZero())
	}

	zero := billingRatePreviewHTTP(t, exchangeRatePreviewExpression, map[string]any{
		"usage_semantic": "normalized", "normalized_usage": map[string]any{},
	})
	require.Empty(t, zero.Error)
	require.NotNil(t, zero.Evaluation)
	assert.Zero(t, zero.Evaluation.RawCostUSD)

	require.Error(t, operation_setting.SetUSDExchangeRate("0"))
	invalid := billingRatePreviewHTTP(t, exchangeRatePreviewExpression, map[string]any{})
	assert.NotEmpty(t, invalid.Error)
	assert.Nil(t, invalid.Evaluation, "invalid rate must not become a zero-price success")
	independent := billingRatePreviewHTTP(t, `tier("base", p * 2)`, map[string]any{"prompt_tokens": 1000000})
	require.Empty(t, independent.Error)
	require.NotNil(t, independent.Evaluation)
	assert.Equal(t, 2.0, independent.Evaluation.RawCostUSD)
}

func TestBillingPreviewHTTPRejectsInvalidNormalizedSamples(t *testing.T) {
	for _, sample := range []map[string]any{
		{"usage_semantic": "normalized"},
		{"usage_semantic": "normalized", "normalized_usage": map[string]any{"p": nil}},
		{"usage_semantic": "normalized", "normalized_usage": map[string]any{"p": -1}},
		{"usage_semantic": "normalized", "normalized_usage": map[string]any{"len": 2147483648}},
		{"usage_semantic": "normalized", "normalized_usage": map[string]any{"secret": 1}},
		{"usage_semantic": "normalized", "normalized_usage": map[string]any{}, "prompt_tokens": 1},
		{"usage_semantic": "openai", "normalized_usage": map[string]any{"p": 1}},
		{"image_count": 0},
		{"image_count": 129},
	} {
		result := billingRatePreviewHTTP(t, `tier("base", p * 2)`, sample)
		assert.NotEmpty(t, result.Error, "%v", sample)
		assert.Nil(t, result.Evaluation, "%v", sample)
	}
}

func TestBillingPreviewHTTPPreservesLengthImageCacheAndFixedRequestUnits(t *testing.T) {
	result := billingRatePreviewHTTP(t, `len > 100 ? tier("large", img_cr * 2) : tier("small", p)`, map[string]any{
		"usage_semantic": "normalized", "normalized_usage": map[string]any{"p": 1, "len": 101, "img_cr": 1000000},
	})
	require.Empty(t, result.Error)
	require.NotNil(t, result.Evaluation)
	assert.Equal(t, "large", result.Evaluation.MatchedTier)
	assert.Equal(t, 2.0, result.Evaluation.RawCostUSD)
	assert.Equal(t, 1000000.0, result.Evaluation.Normalized.ImgCR)

	result = billingRatePreviewHTTP(t, `tier("image", fixed(0.04)) * image_count`, map[string]any{"image_count": 3, "body": map[string]any{"n": 9}})
	require.Empty(t, result.Error)
	require.NotNil(t, result.Evaluation)
	assert.Equal(t, billingexpr.BillingUnitRequest, result.Evaluation.BillingUnit)
	assert.InDelta(t, 0.12, result.Evaluation.RawCostUSD, 1e-12)
}

func billingRatePreviewHTTP(t *testing.T, expression string, sample map[string]any) service.BillingExprPreviewItemResult {
	t.Helper()
	body, err := common.Marshal(map[string]any{"items": []map[string]any{{"key": "preview", "expression": expression, "sample": sample}}})
	require.NoError(t, err)
	router := gin.New()
	router.POST("/preview", PreviewBillingExpression)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/preview", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, response.Code)
	var result struct {
		Success bool                                   `json:"success"`
		Data    []service.BillingExprPreviewItemResult `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.Len(t, result.Data, 1)
	return result.Data[0]
}
