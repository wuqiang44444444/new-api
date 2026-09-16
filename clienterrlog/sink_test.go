package clienterrlog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func awaitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "operation did not complete while output remained blocked")
	}
}

func TestBlockedSinkKeepsRequestsBoundedAndSnapshotsIndependent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	events := make(chan Event, 2)
	sink := NewSink(ctx, 1, func(e Event) error {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		events <- e
		return nil
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sink.Middleware())
	var calls atomic.Int32
	details := map[string]string{"asset_kind": "general"}
	router.GET("/private/:id", func(c *gin.Context) {
		calls.Add(1)
		c.Set(common.RequestIdKey, "request-safe")
		c.Set("id", 7)
		MarkAuthPassed(c, AuthSourceAPIToken)
		Attach(c.Request.Context(), Report{Model: "customer-model", Detail: details})
		c.Set("id", 99) // Later context changes cannot reattribute the authenticated caller.
		c.JSON(422, gin.H{"error": "unchanged"})
	})
	request := func() <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			r := httptest.NewRecorder()
			router.ServeHTTP(r, httptest.NewRequest("GET", "/private/opaque-id?secret=value", nil))
			assert.Equal(t, 422, r.Code)
			assert.JSONEq(t, `{"error":"unchanged"}`, r.Body.String())
		}()
		return done
	}
	first := request()
	awaitSignal(t, entered)
	awaitSignal(t, first)
	awaitSignal(t, request()) // One queued event.
	awaitSignal(t, request()) // Queue full: drop, never retry or write synchronously.
	health := sink.Health()
	assert.EqualValues(t, 3, calls.Load())
	assert.EqualValues(t, 2, health.Accepted)
	assert.EqualValues(t, 1, health.Dropped)
	assert.Equal(t, 1, health.Queued)
	assert.NotZero(t, health.WriteStartedAt)
	details["asset_kind"] = "mutated-after-request"
	once.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		select {
		case e := <-events:
			assert.Contains(t, e.Message, "route=/private/:id")
			assert.Contains(t, e.Message, "user_id=7")
			assert.Contains(t, e.Message, "detail.asset_kind=general")
			assert.NotContains(t, e.Message, "opaque-id")
			assert.NotContains(t, e.Message, "secret")
			assert.NotContains(t, e.Message, "mutated")
		case <-time.After(5 * time.Second):
			require.FailNow(t, "snapshot not delivered")
		}
	}
}

func TestSinkWriteFailuresAreCountedAndWorkerContinues(t *testing.T) {
	for _, mode := range []string{"error", "panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			second := make(chan struct{})
			gate := make(chan struct{})
			defer close(gate)
			var calls int
			sink := NewSink(ctx, 2, func(e Event) error {
				calls++
				if calls == 1 {
					if mode == "panic" {
						panic("private diagnostic")
					}
					return errors.New("private diagnostic")
				}
				close(second)
				<-gate
				return nil
			})
			sink.submit(Event{Message: "first"})
			sink.submit(Event{Message: "second"})
			awaitSignal(t, second)
			assert.EqualValues(t, 1, sink.Health().Failed)
			assert.True(t, sink.Health().Available)
			assert.NotZero(t, sink.Health().WriteStartedAt)
		})
	}
}

type panickingDiagnosticContext struct{ context.Context }

func (panickingDiagnosticContext) Value(any) any { panic("diagnostic lookup failed") }

func TestDiagnosticFaultAndUnavailableSinkDoNotChangeResponse(t *testing.T) {
	sink := NewSink(context.Background(), 1, nil)
	for _, brokenCarrier := range []bool{false, true} {
		before := diagnosticFailures.Load()
		engine := gin.New()
		engine.Use(sink.Middleware())
		engine.GET("/api/test", func(c *gin.Context) {
			c.Set("id", 7)
			MarkAuthPassed(c, AuthSourceSession)
			if brokenCarrier {
				Attach(panickingDiagnosticContext{context.Background()}, Report{Reason: "test"})
			}
			c.String(400, "original response")
		})
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest("GET", "/api/test", nil))
		assert.Equal(t, 400, response.Code)
		assert.Equal(t, "original response", response.Body.String())
		if brokenCarrier {
			assert.Equal(t, before+1, diagnosticFailures.Load())
		}
	}
	assert.False(t, sink.Health().Available)
	assert.EqualValues(t, 2, sink.Health().Dropped)
}

func TestRecorderDoesNotSwallowBusinessPanicOrTurnSSEInto4xx(t *testing.T) {
	for _, mode := range []string{"panic", "sse"} {
		t.Run(mode, func(t *testing.T) {
			sink := NewSink(context.Background(), 1, nil)
			engine := gin.New()
			recovered := false
			engine.Use(func(c *gin.Context) {
				defer func() {
					if recover() != nil {
						recovered = true
						c.Status(500)
					}
				}()
				c.Next()
			}, sink.Middleware())
			engine.GET("/api/test", func(c *gin.Context) {
				MarkAuthPassed(c, AuthSourceSession)
				if mode == "panic" {
					panic("business")
				}
				c.Status(200)
				c.Writer.Flush()
				c.Status(400)
			})
			r := httptest.NewRecorder()
			engine.ServeHTTP(r, httptest.NewRequest("GET", "/api/test", nil))
			if mode == "panic" {
				assert.True(t, recovered)
				assert.Equal(t, 500, r.Code)
			} else {
				assert.Equal(t, 200, r.Code)
			}
			assert.Zero(t, sink.Health().Dropped)
			assert.Zero(t, sink.Health().Accepted)
		})
	}
}

func TestReportWhitelistBoundsAndSnapshotSurvivesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	event := make(chan Event, 1)
	sink := NewSink(ctx, 1, func(e Event) error { event <- e; return nil })
	engine := gin.New()
	engine.Use(sink.Middleware())
	engine.GET("/api/test", func(c *gin.Context) {
		c.Set("id", 7)
		MarkAuthPassed(c, AuthSourceSession)
		Attach(c.Request.Context(), Report{Model: strings.Repeat("x", 10000), Detail: map[string]string{"url": "https://secret.example/?sig=private", "cookie": "private", "asset_kind": "general"}})
		c.String(400, "unchanged")
	})
	requestCtx, requestCancel := context.WithCancel(context.Background())
	requestCancel()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil).WithContext(requestCtx)
	engine.ServeHTTP(httptest.NewRecorder(), req)
	select {
	case e := <-event:
		assert.NotContains(t, e.Message, "private")
		assert.NotContains(t, e.Message, "cookie")
		assert.Less(t, len(e.Message), 1024)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "request cancellation lost event")
	}
}
