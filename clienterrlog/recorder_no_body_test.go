package clienterrlog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrdinaryErrorRecorderKeepsFactsWithoutObservingOrPersistingBodies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	previous := loadEventPersister()
	t.Cleanup(func() { SetEventPersister(previous) })
	persisted := make(chan Event, 1)
	SetEventPersister(func(event Event) error { persisted <- event; return nil })
	sink := NewSink(ctx, 1, func(Event) error { return nil })
	engine := gin.New()
	engine.Use(sink.Middleware())
	raw := `{"api_key":"fixture-secret","unclosed":`
	request := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(raw))
	originalBody := request.Body
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.Set("route_tag", "relay")
		c.Set("id", 7)
		MarkAuthPassed(c, AuthSourceAPIToken)
		assert.Equal(t, originalBody, c.Request.Body, "recorder must not install a second body reader")
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, raw, string(body))
		assert.Nil(t, SnapshotHTTPExchange(c.Request.Context()))
		c.String(422, "unchanged error response")
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	assert.Equal(t, 422, response.Code)
	assert.Equal(t, "unchanged error response", response.Body.String())
	select {
	case event := <-persisted:
		assert.Nil(t, event.HTTPExchange)
		assert.Equal(t, 422, event.Status)
		assert.Equal(t, "relay", event.Module)
		assert.NotContains(t, event.Message, "fixture-secret")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "error facts were not persisted")
	}
}

func TestOrdinaryRelayStatusSurvivesWithoutBodyCaptureAndResetsBeforeRetry(t *testing.T) {
	engine := gin.New()
	engine.Use(Recorder())
	engine.GET("/v1/responses", func(c *gin.Context) {
		BeginNativeRelayAttempt(c)
		response := &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("opaque upstream response"))}
		original := response.Body
		WrapUpstreamResponse(c.Request.Context(), response)
		assert.Equal(t, original, response.Body)
		assert.Equal(t, 429, UpstreamResponseStatus(c.Request.Context()))
		assert.Nil(t, SnapshotHTTPExchange(c.Request.Context()))
		BeginNativeRelayAttempt(c)
		assert.Zero(t, UpstreamResponseStatus(c.Request.Context()))
		WrapUpstreamResponse(c.Request.Context(), &http.Response{StatusCode: 503})
		assert.Equal(t, 503, UpstreamResponseStatus(c.Request.Context()), "status does not require a body")
		request := httptest.NewRequest("POST", "https://provider.invalid", strings.NewReader("opaque request"))
		originalRequest := request.Body
		WrapUpstreamRequest(c.Request.Context(), request)
		assert.Equal(t, originalRequest, request.Body)
		assert.Zero(t, UpstreamResponseStatus(c.Request.Context()), "a new transport request clears stale status")
		c.Status(204)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/v1/responses", nil))
	assert.Equal(t, 204, response.Code)
}
