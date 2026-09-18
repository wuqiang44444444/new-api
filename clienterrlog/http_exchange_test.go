package clienterrlog

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPExchangePreservesTrafficAndRedactsSnapshots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := `{"model":"test","prompt":"draw a cat","max_tokens":0,"stream":false,"id":9007199254740993,"nested":{"apiKey":"fixture-key","refreshToken":"fixture-token","image_url":"https://example.test/private?sig=fixture-signature"},"message":"bad api_key=fixture-inline"}`
	responseBody := `{"error":{"code":"invalid_size","message":"size must be 1024x1024"}}`
	var snapshot *HTTPExchange
	var event Event
	router := gin.New()
	router.Use(func(c *gin.Context) {
		installReport(c)
		InstallHTTPExchange(c)
		c.Next()
		snapshot = SnapshotHTTPExchange(c.Request.Context())
		event = buildBackendEvent(BackendEvent{EventType: EventChannelTest, HTTPExchange: snapshot})
	})
	router.POST("/test", func(c *gin.Context) {
		data, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, requestBody, string(data))
		upstream := &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(responseBody))}
		WrapUpstreamResponse(c.Request.Context(), upstream)
		EnsureUpstreamResponseCapture(c.Request.Context(), upstream)
		got, err := io.ReadAll(upstream.Body)
		require.NoError(t, err)
		assert.Equal(t, responseBody, string(got))
		c.Status(400)
		_, err = c.Writer.WriteString(responseBody)
		require.NoError(t, err)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(requestBody)))
	assert.Equal(t, 400, response.Code)
	assert.Equal(t, responseBody, response.Body.String())
	require.NotNil(t, snapshot)
	assert.Equal(t, "captured", snapshot.Request.State)
	for _, secret := range []string{"fixture-key", "fixture-token", "fixture-signature", "fixture-inline", "https://"} {
		assert.NotContains(t, snapshot.Request.Body, secret)
	}
	assert.Contains(t, snapshot.Request.Body, `"max_tokens": 0`)
	assert.Contains(t, snapshot.Request.Body, `"stream": false`)
	assert.Contains(t, snapshot.Request.Body, `9007199254740993`)
	assert.Contains(t, snapshot.Request.Body, `draw a cat`)
	assert.JSONEq(t, responseBody, snapshot.UpstreamResponse.Body)
	assert.Equal(t, 400, snapshot.UpstreamResponse.Status)
	assert.Equal(t, 400, snapshot.Response.Status)
	assert.Equal(t, snapshot, event.HTTPExchange)
	assert.NotContains(t, event.Message, "draw a cat")
}

func TestHTTPExchangeUnavailableBodiesFailClosed(t *testing.T) {
	for _, tt := range []struct{ name, input, state string }{
		{"large", `{"prompt":"` + strings.Repeat("x", diagnosticBodyLimit) + `"}`, "too_large"},
		{"malformed", `{"api_key":"fixture-secret"`, "unsupported"},
		{"html", `<html>fixture-secret</html>`, "unsupported"},
		{"empty", "", "empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(tt.input))
			InstallHTTPExchange(c)
			data, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			assert.Equal(t, tt.input, string(data))
			snapshot := SnapshotHTTPExchange(c.Request.Context())
			assert.Equal(t, tt.state, snapshot.Request.State)
			assert.Empty(t, snapshot.Request.Body)
		})
	}
	assert.Nil(t, SnapshotHTTPExchange(context.Background()))
}

func TestHTTPExchangeLatestUpstreamResponseAndNoHeaders(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	c.Request.Header.Set("Authorization", "Bearer fixture-secret")
	InstallHTTPExchange(c)
	for _, status := range []int{429, 503} {
		response := &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"error":"try later"}`))}
		WrapUpstreamResponse(c.Request.Context(), response)
		_, err := io.Copy(io.Discard, response.Body)
		require.NoError(t, err)
	}
	snapshot := SnapshotHTTPExchange(c.Request.Context())
	assert.Equal(t, 503, snapshot.UpstreamResponse.Status)
	assert.JSONEq(t, `{"error":"try later"}`, snapshot.UpstreamResponse.Body)
	encoded, err := common.Marshal(snapshot)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "fixture-secret")
}

func TestHTTPExchangeDiscardsReadFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	c.Request.Body = io.NopCloser(iotest.ErrReader(errors.New("fixture read failure")))
	InstallHTTPExchange(c)
	_, err := io.ReadAll(c.Request.Body)
	require.EqualError(t, err, "fixture read failure")
	snapshot := SnapshotHTTPExchange(c.Request.Context())
	assert.Equal(t, "read_failed", snapshot.Request.State)
	assert.Empty(t, snapshot.Request.Body)
}
