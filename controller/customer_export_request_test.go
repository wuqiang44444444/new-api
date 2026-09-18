package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportRequestPreservesExplicitZeroAndScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"job_type":"statement_details","start_timestamp":1000,"end_timestamp":2001,"token_id":0,"channel_id":3,"token_name":"key","group":"vip","request_id":"req","upstream_request_id":"up","username":"customer","model_name":"model","billing_mode":"per_second"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	got, ok := parseCustomerExportRequest(ctx)
	require.True(t, ok)
	require.NotNil(t, got.TokenId)
	assert.Equal(t, 0, *got.TokenId)
	require.NotNil(t, got.ChannelId)
	assert.Equal(t, 3, *got.ChannelId)
	assert.Equal(t, "key", got.TokenName)
	assert.Equal(t, "vip", got.Group)
	assert.Equal(t, "req", got.RequestId)
	assert.Equal(t, "up", got.UpstreamRequestId)
	assert.Equal(t, "customer", got.Username)
	assert.Equal(t, "model", got.ModelName)
	assert.Equal(t, "per_second", got.BillingMode)
	assert.EqualValues(t, 2001, got.EndTimestamp)
	ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"job_type":"usage_logs"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	absent, ok := parseCustomerExportRequest(ctx)
	require.True(t, ok)
	assert.Nil(t, absent.TokenId)
}
