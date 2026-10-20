package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeRelayFinalErrorDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name                                               string
		upstreamStatus, clientStatus                       int
		transport, nextAttempt, explicit, selectionFailure bool
		wantStage, wantReason                              string
	}{
		{name: "provider rate limit", upstreamStatus: 429, clientStatus: 429, wantStage: "upstream_response", wantReason: "upstream_rate_limited"},
		{name: "mapped provider rate limit", upstreamStatus: 429, clientStatus: 503, wantStage: "upstream_response", wantReason: "upstream_rate_limited"},
		{name: "gateway rate limit", clientStatus: 429, wantStage: "relay", wantReason: "local_error"},
		{name: "retry transport failure", upstreamStatus: 429, clientStatus: 502, transport: true, nextAttempt: true, wantStage: "upstream_request", wantReason: "upstream_transport_error"},
		{name: "retry local failure", upstreamStatus: 429, clientStatus: 400, nextAttempt: true, wantStage: "relay", wantReason: "local_error"},
		{name: "retry has no channel", upstreamStatus: 429, clientStatus: 500, nextAttempt: true, selectionFailure: true, wantStage: "channel_selection", wantReason: "channel_unavailable"},
		{name: "explicit diagnostic preserved", upstreamStatus: 429, clientStatus: 429, explicit: true, wantStage: "model_access", wantReason: "token_model_forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := logtest.New(t)
			engine := gin.New()
			engine.Use(capture.Middleware())
			engine.POST("/relay", func(c *gin.Context) {
				clienterrlog.BeginNativeRelayAttempt(c)
				c.Set("channel_id", 149)
				c.Set("original_model", "customer-image")
				c.Set(common.UpstreamRequestIdKey, "attempt-one")
				if tc.upstreamStatus > 0 {
					clienterrlog.WrapUpstreamResponse(c.Request.Context(), &http.Response{StatusCode: tc.upstreamStatus, Body: io.NopCloser(strings.NewReader(`{"error":{}}`))})
				}
				if tc.nextAttempt {
					clienterrlog.BeginNativeRelayAttempt(c)
					if !tc.selectionFailure {
						c.Set("channel_id", 155)
					}
					assert.Empty(t, c.GetString(common.UpstreamRequestIdKey))
					assert.Zero(t, clienterrlog.UpstreamResponseStatus(c.Request.Context()))
					assert.Equal(t, "not_recorded", clienterrlog.SnapshotHTTPExchange(c.Request.Context()).UpstreamResponse.State)
				}
				if tc.explicit {
					clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{Stage: "model_access", Reason: "token_model_forbidden", ChannelID: 7, Model: "explicit-model"})
				}
				apiErr := types.NewErrorWithStatusCode(errors.New("synthetic local error"), types.ErrorCodeInvalidRequest, tc.clientStatus)
				if tc.upstreamStatus > 0 && !tc.nextAttempt {
					apiErr = types.NewOpenAIError(errors.New("synthetic provider error"), types.ErrorCodeBadResponseStatusCode, tc.clientStatus)
				}
				if tc.transport {
					apiErr = types.NewErrorWithStatusCode(errors.New("synthetic connection failure"), types.ErrorCodeDoRequestFailed, tc.clientStatus)
				}
				if tc.selectionFailure {
					apiErr = types.NewError(errors.New("no available retry channel"), types.ErrorCodeGetChannelFailed)
				}
				AttachNativeRelayErrorDiagnostics(c, apiErr)
				report, ok := clienterrlog.PeekReport(c.Request.Context())
				require.True(t, ok)
				assert.Equal(t, tc.wantStage, report.Stage)
				assert.Equal(t, tc.wantReason, report.Reason)
				if tc.selectionFailure {
					assert.Zero(t, report.ChannelID)
				}
				if tc.explicit {
					assert.Equal(t, 7, report.ChannelID)
					assert.Equal(t, "explicit-model", report.Model)
				}
				assert.Equal(t, tc.clientStatus, apiErr.StatusCode)
				c.Status(http.StatusOK) // No authenticated final error is emitted by this fixture.
			})
			engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/relay", nil))
		})
	}
}
