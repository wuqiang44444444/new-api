package controller

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedImagePriceUsesExistingBillingPreview(t *testing.T) {
	server := gin.New()
	server.POST("/image-prices", ImageSizePricing)
	server.POST("/preview", PreviewBillingExpression)

	prices := map[string]float64{
		"1K_low": 0.01667, "1K_medium": 0.05484, "1K_high": 0.18364,
		"2K_low": 0.02070, "2K_medium": 0.09119, "2K_high": 0.32907,
		"4K_low": 0.02848, "4K_medium": 0.16106, "4K_high": 0.60859,
	}
	payload, err := common.Marshal(map[string]any{"profile": "medium_three", "prices": prices})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest("POST", "/image-prices", bytes.NewReader(payload)))
	require.Equal(t, 200, recorder.Code)
	var generated struct {
		Success bool
		Data    struct{ Expression string }
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &generated))
	require.True(t, generated.Success)
	require.NotEmpty(t, generated.Data.Expression)
	reopened := billingexpr.ReadImageSizePrices(generated.Data.Expression)
	require.NotNil(t, reopened)
	assert.Equal(t, prices, reopened.Prices)

	two, one := 2, 1
	payload, err = common.Marshal(map[string]any{"items": []service.BillingExprPreviewItem{
		{Key: "reserve", Expression: generated.Data.Expression, Sample: &service.BillingExprPreviewSample{
			Body: map[string]any{"size": "3840x2160", "quality": "high"}, ImageCount: &two}},
		{Key: "actual", Expression: generated.Data.Expression, Sample: &service.BillingExprPreviewSample{
			Body: map[string]any{"size": "2160x3840", "quality": "high"}, ImageCount: &one}},
	}})
	require.NoError(t, err)
	recorder = httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest("POST", "/preview", bytes.NewReader(payload)))
	require.Equal(t, 200, recorder.Code)
	var preview struct {
		Success bool
		Data    []service.BillingExprPreviewItemResult
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &preview))
	require.True(t, preview.Success)
	require.Len(t, preview.Data, 2)
	for _, item := range preview.Data {
		require.Empty(t, item.Error)
		require.NotNil(t, item.Evaluation)
		assert.Equal(t, billingexpr.BillingUnitRequest, item.Evaluation.BillingUnit)
		assert.Equal(t, "4K_high", item.Evaluation.MatchedTier)
	}
	assert.InDelta(t, 1.21718, preview.Data[0].Evaluation.RawCostUSD, 1e-9)
	assert.InDelta(t, 0.60859, preview.Data[1].Evaluation.RawCostUSD, 1e-9)
}

func TestImagePriceSaveEndpointsReturnNonBlockingFieldWarning(t *testing.T) {
	modelManagementDB(t, "sqlite", "")
	const name = "image-save-advisory"
	const expr = `(param("input_image_count") == nil ? tier("a",fixed(1)) : tier("b",fixed(2))) * image_count`
	snapshot, err := model.GetModelPricingSnapshot([]string{name})
	require.NoError(t, err)
	server := gin.New()
	server.PATCH("/model_pricing", UpdateModelPricingConfig)
	server.PUT("/option", UpdateOption)
	options, err := common.Marshal(map[string]string{name: expr})
	require.NoError(t, err)
	changes := []model.ModelPricingChange{{
		ModelName: name, ExpectedVersion: snapshot.Entries[0].Version,
		Pricing: model.PricingValues{
			"billing_setting.billing_mode": "tiered_expr",
			"billing_setting.billing_expr": expr,
		},
	}}
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"PATCH", "/model_pricing", gin.H{"changes": changes}},
		{"PUT", "/option", gin.H{"key": "billing_setting.billing_expr", "value": string(options)}},
	} {
		raw, err := common.Marshal(tc.body)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, bytes.NewReader(raw)))
		var response struct {
			Success  bool
			Message  string
			Warnings map[string][]billingexpr.ImagePriceWarning `json:"pricing_warnings"`
			Data     struct {
				Warnings map[string][]billingexpr.ImagePriceWarning `json:"pricing_warnings"`
			}
		}
		require.Equal(t, 200, w.Code)
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success, response.Message)
		warnings := response.Warnings
		if tc.method == "PATCH" {
			warnings = response.Data.Warnings
		}
		assert.Equal(t, []billingexpr.ImagePriceWarning{{Code: "unavailable_parameter", Field: "input_image_count"}}, warnings[name])
	}
}

func TestLegacyGridWithEqualSizePricesRemainsImportable(t *testing.T) {
	server := gin.New()
	server.POST("/image-prices", ImageSizePricing)
	chain := `tier("1K_medium", fixed(0.2))`
	for _, tier := range []string{"4K", "2K", "1K"} {
		size := fmt.Sprintf(`param("size") == %q`, tier)
		if tier == "1K" {
			size = "(" + size + ` || param("size") == "1024x1024")`
		}
		quality := fmt.Sprintf(`param("quality") == "low" ? tier("%[1]s_low", fixed(0.1)) : param("quality") == "high" ? tier("%[1]s_high", fixed(0.3)) : tier("%[1]s_medium", fixed(0.2))`, tier)
		chain = fmt.Sprintf("(%s ? (%s) : %s)", size, quality, chain)
	}
	for _, migrate := range []bool{false, true} {
		request, err := common.Marshal(gin.H{"expression": chain + " * image_count", "migrate": migrate})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, httptest.NewRequest("POST", "/image-prices", bytes.NewReader(request)))
		var response struct {
			Success bool
			Data    struct {
				Importable bool
				Table      *billingexpr.ImageSizePriceTable
				Warnings   []billingexpr.ImagePriceWarning
			}
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success)
		assert.True(t, response.Data.Importable)
		assert.Empty(t, response.Data.Warnings)
		if !migrate {
			assert.Nil(t, response.Data.Table)
			continue
		}
		require.NotNil(t, response.Data.Table)
		for _, size := range []string{"1K", "2K", "4K"} {
			assert.Equal(t, 0.1, response.Data.Table.Prices[size+"_low"])
			assert.Equal(t, 0.2, response.Data.Table.Prices[size+"_medium"])
			assert.Equal(t, 0.3, response.Data.Table.Prices[size+"_high"])
		}
	}
}
