package clienterrlog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPExchangePersistenceRunsAfterRequestAndNeverReachesWARN(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	previous := loadEventPersister()
	t.Cleanup(func() { SetEventPersister(previous) })
	warned := make(chan Event, 1)
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	type storedResult struct {
		payload []byte
		err     error
	}
	persisted := make(chan storedResult, 1)
	SetEventPersister(func(event Event) error {
		payload, err := common.Marshal(event.HTTPExchange)
		persisted <- storedResult{payload: payload, err: err}
		return err
	})
	sink := NewSink(ctx, 1, func(event Event) error {
		warned <- event
		<-release
		return nil
	})
	raw := `{"id":9007199254740993,"max_tokens":0,"stream":false,"nested":{"\u0061pi_key":"fixture-secret"}}`
	var requestContext context.Context
	engine := gin.New()
	engine.Use(sink.Middleware())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		c.Set("route_tag", "relay")
		MarkAuthPassed(c, AuthSourceAPIToken)
		requestContext = c.Request.Context()
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, raw, string(body))
		c.JSON(422, gin.H{"error": "unchanged"})
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(raw)))
	assert.Equal(t, 422, response.Code)
	assert.JSONEq(t, `{"error":"unchanged"}`, response.Body.String())
	select {
	case event := <-warned:
		assert.Nil(t, event.HTTPExchange, "WARN must never receive a pending or stored body")
		assert.NotContains(t, event.Message, "fixture-secret")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "WARN did not reach the blocked writer")
	}
	select {
	case <-persisted:
		require.FailNow(t, "persistence ran before the worker was released")
	default:
	}
	// A later observation must not mutate the snapshot waiting on the worker.
	ObserveHTTPBody(requestContext, "request", []byte("broken suffix"), 0)
	once.Do(func() { close(release) })
	select {
	case result := <-persisted:
		require.NoError(t, result.err)
		assert.NotContains(t, string(result.payload), "fixture-secret")
		var stored HTTPExchange
		require.NoError(t, common.Unmarshal(result.payload, &stored))
		assert.Equal(t, "captured", stored.Request.State)
		assert.Contains(t, stored.Request.Body, `"id":9007199254740993`)
		assert.Contains(t, stored.Request.Body, `"max_tokens":0`)
		assert.Contains(t, stored.Request.Body, `"stream":false`)
		assert.NotContains(t, stored.Request.Body, "\n")
		assert.Equal(t, 422, stored.Response.Status)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "worker did not persist the frozen snapshot")
	}
}

func TestHTTPExchangePendingBytesAreNeverPubliclyExposed(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
	InstallHTTPExchange(c)
	ObserveHTTPBody(c.Request.Context(), "request", []byte(`{"api_key":"fixture-secret","n":0}`), 0)
	snapshot := SnapshotHTTPExchange(c.Request.Context())
	require.NotNil(t, snapshot)
	assert.Empty(t, snapshot.Request.Body)
	// Both pointer and value serialization must go through the same protection.
	for _, value := range []any{snapshot, *snapshot} {
		encoded, err := common.Marshal(value)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "fixture-secret")
		var stored HTTPExchange
		require.NoError(t, common.Unmarshal(encoded, &stored))
		assert.Equal(t, "captured", stored.Request.State)
		assert.Contains(t, stored.Request.Body, `"n":0`)
	}
}
