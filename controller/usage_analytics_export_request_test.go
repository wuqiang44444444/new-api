package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageExportRequestPreservesExactModelAndCustomerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"period":"day","date":"2026-10-12","view":"customer","user_id":43,"model_name":"Customer/Model A","language":"zh"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	var body usageAnalyticsExportBody
	require.NoError(t, ctx.ShouldBindJSON(&body))
	require.NotNil(t, body.UserId)
	assert.Equal(t, 43, *body.UserId)
	assert.Equal(t, "Customer/Model A", body.ModelName)
	assert.Equal(t, "customer", body.View)
	assert.Equal(t, "zh", body.Language)
}

func TestUsageExportInvalidModelReturnsTranslatableValidationMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 7)
	raw, err := common.Marshal(map[string]string{"period": "day", "date": "2026-10-12", "model_name": strings.Repeat("x", 256)})
	require.NoError(t, err)
	ctx.Request = httptest.NewRequest("POST", "/api/usage/self/exports", strings.NewReader(string(raw)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	CreateUsageSelfExport(ctx)
	var response struct {
		Success    bool   `json:"success"`
		MessageKey string `json:"message_key"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "Model filters require a customer usage view and a model name of at most 255 bytes.", response.MessageKey)
}
