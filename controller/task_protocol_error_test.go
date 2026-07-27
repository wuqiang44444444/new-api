package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondTaskProtocolErrorUsesProtocolEnvelopeAndHidesUpstreamMessage(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
	}{
		{name: "OpenAI", protocol: model.TaskClientProtocolOpenAIVideos},
		{name: "ModelArk", protocol: model.TaskClientProtocolModelArkV3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			common.SetContextKey(context, constant.ContextKeyTaskClientProtocol, test.protocol)
			context.Set(common.RequestIdKey, "request-123")

			respondTaskError(context, &dto.TaskError{
				Code:       "fail_to_fetch_task",
				Message:    `{"error":{"message":"secret upstream body"}}`,
				StatusCode: http.StatusBadGateway,
				Error:      errors.New("secret upstream body"),
			})

			assert.Equal(t, http.StatusBadGateway, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "secret upstream body")
			var body map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			_, ok := body["error"].(map[string]any)
			assert.True(t, ok)
		})
	}
}

func TestTaskProtocolErrorFieldsSeparatesClientAndProviderAuthentication(t *testing.T) {
	tests := []struct {
		name       string
		local      bool
		status     int
		wantStatus int
		wantCode   string
		wantType   string
	}{
		{
			name: "client authentication", local: true, status: http.StatusUnauthorized,
			wantStatus: http.StatusUnauthorized, wantCode: "authentication_error", wantType: "authentication_error",
		},
		{
			name: "client permission", local: true, status: http.StatusForbidden,
			wantStatus: http.StatusForbidden, wantCode: "permission_denied", wantType: "permission_error",
		},
		{
			name: "provider authentication", local: false, status: http.StatusUnauthorized,
			wantStatus: http.StatusBadGateway, wantCode: "upstream_auth_error", wantType: "server_error",
		},
		{
			name: "provider permission", local: false, status: http.StatusForbidden,
			wantStatus: http.StatusBadGateway, wantCode: "upstream_auth_error", wantType: "server_error",
		},
		{
			name: "not found", local: true, status: http.StatusNotFound,
			wantStatus: http.StatusNotFound, wantCode: "not_found", wantType: "invalid_request_error",
		},
		{
			name: "conflict", local: true, status: http.StatusConflict,
			wantStatus: http.StatusConflict, wantCode: "conflict", wantType: "invalid_request_error",
		},
		{
			name: "quota", local: true, status: http.StatusPaymentRequired,
			wantStatus: http.StatusPaymentRequired, wantCode: "insufficient_quota", wantType: "insufficient_quota",
		},
		{
			name: "rate limit", local: false, status: http.StatusTooManyRequests,
			wantStatus: http.StatusTooManyRequests, wantCode: "rate_limit_exceeded", wantType: "rate_limit_error",
		},
		{
			name: "local internal error", local: true, status: http.StatusInternalServerError,
			wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantType: "server_error",
		},
		{
			name: "upstream unavailable", local: false, status: http.StatusBadGateway,
			wantStatus: http.StatusBadGateway, wantCode: "upstream_unavailable", wantType: "server_error",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, code, errorType, message := taskProtocolErrorFields(&dto.TaskError{
				StatusCode: test.status,
				LocalError: test.local,
			})

			assert.Equal(t, test.wantStatus, status)
			assert.Equal(t, test.wantCode, code)
			assert.Equal(t, test.wantType, errorType)
			assert.NotEmpty(t, message)
		})
	}
}
