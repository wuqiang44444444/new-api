package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynchronousRequestKeepsAcceptedBodyAfterClientCancellation(t *testing.T) {
	service.InitHttpClient()
	downstream, cancel := context.WithCancel(context.Background())
	defer cancel()
	writeBody := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-writeBody:
			_, _ = io.WriteString(w, `{"usage":{"input_tokens":40,"output_tokens":0}}`)
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	t.Cleanup(upstream.Close)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(downstream)
	transportContext, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	req, err := http.NewRequestWithContext(transportContext, http.MethodPost, upstream.URL, strings.NewReader("{}"))
	require.NoError(t, err)
	resp, err := doSynchronousRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()
	close(writeBody)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"usage":{"input_tokens":40,"output_tokens":0}}`, string(body))
}

type upstreamHeaderHandoffTransport struct {
	http.RoundTripper
	received func(*http.Response)
}

func (rt upstreamHeaderHandoffTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rt.RoundTripper.RoundTrip(req)
	if err == nil {
		rt.received(resp)
	}
	return resp, err
}

func TestSynchronousTextKeepsReceived200WhenCancellationRacesHandoff(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	base := client.Transport
	t.Cleanup(func() { client.Transport = base })
	downstream, cancel := context.WithCancel(context.Background())
	defer cancel()
	var receivedStatus int
	writeBody := make(chan struct{})
	client.Transport = upstreamHeaderHandoffTransport{RoundTripper: base, received: func(resp *http.Response) {
		receivedStatus = resp.StatusCode
		cancel()
	}}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-writeBody:
			_, _ = io.WriteString(w, "response")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(upstream.Close)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(downstream)
	transport, stop := context.WithTimeout(t.Context(), 3*time.Second)
	defer stop()
	req, err := http.NewRequestWithContext(transport, http.MethodPost, upstream.URL, nil)
	require.NoError(t, err)
	resp, err := doSynchronousRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.Equal(t, http.StatusOK, receivedStatus)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	close(writeBody)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "response", string(body))
}

func TestNativeImageKeepsExistingTransportWhenClientAlreadyDisconnected(t *testing.T) {
	service.InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil).WithContext(ctx)
	req, err := http.NewRequest(http.MethodPost, upstream.URL, nil)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	resp, err := doNativeAPIRequest(c, req, info)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
