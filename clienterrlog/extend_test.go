package clienterrlog_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 200 后的显式流式异常：按 stream_error 类型持久化，保留真实 200，不写 WARN 行。
func TestStreamErrorEventAfter200(t *testing.T) {
	buffer := logtest.New(t)
	capture := &persistCapture{}
	capture.register(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "relay") },
		func(c *gin.Context) {
			clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{Model: "m1", ChannelID: 3})
			clienterrlog.AttachStreamError(c.Request.Context(), clienterrlog.StreamErrorReport{
				Reason: "stream_timeout", EndReason: "timeout", Severity: "fatal", ErrorCount: 2,
			})
			c.Status(http.StatusOK)
		},
	)

	response := postAsset(engine)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Eventually(t, func() bool { return len(capture.snapshot()) == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.Zero(t, buffer.Health().Written, "流式事件不得写 WARN 行")
	events := capture.snapshot()
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, clienterrlog.EventStreamError, event.EventType)
	assert.Equal(t, 200, event.Status)
	assert.Equal(t, "stream_timeout", event.Reason)
	assert.Equal(t, "stream", event.Stage)
	assert.Equal(t, "m1", event.Model)
	assert.Equal(t, 3, event.ChannelID)
	assert.Equal(t, "fatal", event.Detail["stream_severity"])
	assert.Equal(t, "2", event.Detail["stream_error_count"])
}

// 最终 4xx 且有流式诊断：合并进同一条 api_error 事件，不重复记录。
func TestStreamDiagnosticsMergeIntoApiError(t *testing.T) {
	buffer := logtest.New(t)
	capture := &persistCapture{}
	capture.register(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "relay") },
		func(c *gin.Context) {
			clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
			clienterrlog.AttachStreamError(c.Request.Context(), clienterrlog.StreamErrorReport{
				Reason: "stream_timeout", EndReason: "timeout", Severity: "fatal",
			})
			c.Status(http.StatusBadGateway)
		},
	)

	postAsset(engine)

	assert.Eventually(t, func() bool { return len(capture.snapshot()) == 1 }, 2*time.Second, 10*time.Millisecond)
	events := capture.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, clienterrlog.EventAPIError, events[0].EventType)
	assert.Equal(t, 502, events[0].Status)
	assert.Equal(t, "stream_timeout", events[0].Detail["stream_reason"])
	assert.Zero(t, buffer.Health().Written, "502 不写 WARN 行")
	assert.EqualValues(t, 1, buffer.Health().Accepted)
}

// WARN 写失败不再阻断持久化：两个出口相互独立。
func TestWarnFailureDoesNotBlockPersistence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	persisted := make(chan clienterrlog.Event, 1)
	clienterrlog.SetEventPersister(func(e clienterrlog.Event) error { persisted <- e; return nil })
	t.Cleanup(func() { clienterrlog.SetEventPersister(nil) })
	sink := clienterrlog.NewSink(ctx, 4, func(e clienterrlog.Event) error { return errors.New("log io down") })
	engine := gin.New()
	engine.Use(sink.Middleware())
	engine.POST("/v1/x", func(c *gin.Context) {
		c.Set("route_tag", "relay")
		clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
		c.Status(http.StatusBadRequest)
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/x", nil))

	select {
	case event := <-persisted:
		assert.Equal(t, clienterrlog.EventAPIError, event.EventType)
		assert.Equal(t, 400, event.Status)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "persistence blocked by WARN failure")
	}
	health := sink.Health()
	assert.EqualValues(t, 1, health.Failed)
	assert.EqualValues(t, 1, health.Persisted)
}

// 后台事件入口：任务失败事件带类型与公开任务 ID；未知类型按诊断故障计数丢弃。
func TestSubmitBackendEventContract(t *testing.T) {
	capture := &persistCapture{}
	capture.register(t)
	clienterrlog.SubmitBackendEvent(clienterrlog.BackendEvent{
		EventType: clienterrlog.EventTaskFailure,
		Module:    "relay",
		Stage:     "task_lifecycle",
		Reason:    "task_failed",
		ChannelID: 5,
		UserID:    9,
		TaskID:    "task_abc",
		Detail:    map[string]string{"platform": "video", "fail_reason": "upstream gone"},
	})
	assert.Eventually(t, func() bool { return len(capture.snapshot()) == 1 }, 2*time.Second, 10*time.Millisecond)
	events := capture.snapshot()
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, clienterrlog.EventTaskFailure, event.EventType)
	assert.Equal(t, "task_abc", event.TaskID)
	assert.Equal(t, 0, event.Status, "任务事件无客户 HTTP 状态")
	assert.Equal(t, "video", event.Detail["platform"])
	assert.Equal(t, "upstream_gone", event.Detail["fail_reason"], "净化把空白折叠为下划线")

	before := logtest.New(t).Health().DiagnosticFailures
	lengthBefore := len(capture.snapshot())
	clienterrlog.SubmitBackendEvent(clienterrlog.BackendEvent{EventType: "bogus"})
	assert.Equal(t, before+1, logtest.New(t).Health().DiagnosticFailures)
	assert.Len(t, capture.snapshot(), lengthBefore, "未知类型不得入队")
}
