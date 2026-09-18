package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoRequestCapturesDiagnosticExchangeWithoutChangingTraffic(t *testing.T) {
	service.InitHttpClient()
	const requestBody = `{"model":"provider-test","prompt":"draw a cat","api_key":"fixture-secret"}`
	const responseBody = `{"error":{"message":"unsupported size","code":"invalid_size"}}`
	received := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			received <- "read_failed"
			return
		}
		received <- string(body)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, responseBody)
	}))
	defer upstream.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	clienterrlog.InstallHTTPExchange(c)
	req, err := http.NewRequest("POST", upstream.URL, strings.NewReader(requestBody))
	require.NoError(t, err)
	contentLength := req.ContentLength
	require.NotNil(t, req.GetBody)
	resp, err := doRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, err)
	defer resp.Body.Close()
	// The normal upstream error handler must not append the body twice.
	apiErr := service.RelayErrorHandler(c.Request.Context(), resp, false)
	require.NotNil(t, apiErr)
	assert.Equal(t, requestBody, <-received)
	assert.Equal(t, contentLength, req.ContentLength)
	replay, err := req.GetBody()
	require.NoError(t, err)
	defer replay.Close()
	replayBody, err := io.ReadAll(replay)
	require.NoError(t, err)
	assert.Equal(t, requestBody, string(replayBody))
	snapshot := clienterrlog.SnapshotHTTPExchange(c.Request.Context())
	require.NotNil(t, snapshot)
	assert.Contains(t, snapshot.UpstreamRequest.Body, "draw a cat")
	assert.NotContains(t, snapshot.UpstreamRequest.Body, "fixture-secret")
	assert.Equal(t, http.StatusBadRequest, snapshot.UpstreamResponse.Status)
	assert.JSONEq(t, responseBody, snapshot.UpstreamResponse.Body)
}
