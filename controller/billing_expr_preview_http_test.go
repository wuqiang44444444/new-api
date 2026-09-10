package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingPreviewHTTPMoneyAndTime(t *testing.T) {
	router := gin.New()
	router.POST("/preview", PreviewBillingExpression)
	for _, sample := range []struct {
		at  string
		usd float64
	}{
		{"2026-09-07T08:59:59+08:00", 2}, {"2026-09-07T09:00:00+08:00", 4},
	} {
		payload := map[string]any{"items": []service.BillingExprPreviewItem{{
			Key: "estimator", Expression: `tier("base", p*2)*(hour("Asia/Shanghai")>=9?2:1)`,
			Sample: &service.BillingExprPreviewSample{PromptTokens: 1_000_000, PricingTime: sample.at},
		}}}
		body, err := common.Marshal(payload)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(string(body)))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool                                   `json:"success"`
			Data    []service.BillingExprPreviewItemResult `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success)
		require.Len(t, result.Data, 1)
		require.Empty(t, result.Data[0].Error)
		require.NotNil(t, result.Data[0].Evaluation)
		assert.Equal(t, sample.usd, result.Data[0].Evaluation.RawCostUSD)
		assert.Equal(t, common.QuotaRound(sample.usd*common.QuotaPerUnit), result.Data[0].Evaluation.Quota)
		require.Len(t, result.Data[0].Projection.Scenarios, 2)
	}
}
