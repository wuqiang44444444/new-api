package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 统一出口与业务出口各写一次：公共 JSON 响应保持原样，同时每请求恰好一条
// 带阶段/原因/公共错误码的诊断事件；事件不包含请求体或 URL 细节。
func TestAssetErrorExitsKeepPublicResponseAndEmitOneEvent(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		expectStatus int
		expectCode   string
		expectStage  string
		expectReason string
	}{
		{"unsupported type", service.ErrUnsupportedAssetType, http.StatusUnprocessableEntity, "unsupported_asset_type", "capability_check", "unsupported_asset_type"},
		{"model missing", service.ErrAssetModelNotFound, http.StatusNotFound, "model_not_found", "model_resolution", "model_not_found"},
		{"default group missing", service.ErrDefaultAssetGroupNotConfigured, http.StatusConflict, "default_asset_group_not_configured", "group_resolution", "default_asset_group_not_configured"},
		{"reserved group name", service.ErrReservedAssetGroupName, http.StatusBadRequest, "reserved_asset_group_name", "request_validation", "reserved_asset_group_name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buffer := logtest.New(t)
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Set(common.RequestIdKey, "req-"+tc.expectCode)
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), common.RequestIdKey, "req-"+tc.expectCode))
				c.Next()
			}, buffer.Middleware())
			engine.POST("/v1/assets", func(c *gin.Context) {
				c.Set("id", 9)
				clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
				writeAssetServiceError(c, tc.err)
			})

			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/assets", nil))

			require.Equal(t, tc.expectStatus, response.Code)
			var body struct {
				Error struct {
					Code      string `json:"code"`
					RequestID string `json:"request_id"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, tc.expectCode, body.Error.Code)
			assert.Equal(t, "req-"+tc.expectCode, body.Error.RequestID)

			log := buffer.String()
			assert.Equal(t, 1, strings.Count(log, "event=authenticated_api_client_error"), log)
			assert.Contains(t, log, "status="+strconv.Itoa(tc.expectStatus))
			assert.Contains(t, log, "stage="+tc.expectStage)
			assert.Contains(t, log, "reason="+tc.expectReason)
			assert.Contains(t, log, "public_code="+tc.expectCode)
			assert.Contains(t, log, "user_id=9")
			assert.NotContains(t, log, "Bearer")
		})
	}
}
