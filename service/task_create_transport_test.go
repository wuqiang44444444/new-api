package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The probe must be observable through the real wiring: attach on a gin
// context, dial a closed port, settle from the gin context (not the rebinding
// local request). Regression for the first review finding: reading the stale
// c.Request.Context() made the classification dead code.
func TestSettleTaskCreateTransportOutcomeReleasesProvenUnsentCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must never reach a handler")
	}))
	server.Close() // port closed: dial refused, nothing sent

	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{}`))
	common.SetContextKey(ginCtx, constant.ContextKeyTaskCreateAttemptID, 1)
	info := &relaycommon.RelayInfo{OriginModelName: "video-model", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 7}}

	client := &http.Client{Transport: http.DefaultTransport}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"model":"video-model"}`))
	require.NoError(t, err)
	req = AttachTaskCreateTransportProbe(ginCtx, req, client)
	require.NotNil(t, client.Transport)

	_, err = client.Do(req)
	require.Error(t, err)
	SettleTaskCreateTransportOutcome(ginCtx, info, err)

	assert.Equal(t, relaycommon.TaskCreateTerminalRejection, relaycommon.GetTaskCreateDisposition(ginCtx))
	assert.False(t, common.GetContextKeyBool(ginCtx, constant.ContextKeyTaskCreateOutcomeUnknown))
}

// A reachable server means bytes went out: the outcome must stay unknown so
// the hold is preserved for technical reconciliation.
func TestSettleTaskCreateTransportOutcomeKeepsUnknownAfterWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{}`))
	common.SetContextKey(ginCtx, constant.ContextKeyTaskCreateAttemptID, 2)

	client := &http.Client{Transport: http.DefaultTransport}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{"model":"video-model"}`))
	require.NoError(t, err)
	req = AttachTaskCreateTransportProbe(ginCtx, req, client)
	require.NotNil(t, client.Transport)

	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	state := taskCreateTransportProbeStateFromGin(ginCtx)
	require.NotNil(t, state)
	assert.True(t, state.provenSent())
	assert.False(t, state.provenUnsent())
}
