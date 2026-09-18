package clienterrlog_test

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// persistCapture 捕获 persister 收到的结构化事件；用完必须恢复全局钩子。
type persistCapture struct {
	mu     sync.Mutex
	events []clienterrlog.Event
}

func (p *persistCapture) register(t *testing.T) {
	t.Helper()
	clienterrlog.SetEventPersister(func(e clienterrlog.Event) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.events = append(p.events, e)
		return nil
	})
	t.Cleanup(func() { clienterrlog.SetEventPersister(nil) })
}

func (p *persistCapture) snapshot() []clienterrlog.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]clienterrlog.Event(nil), p.events...)
}

// relay/asset 的最终 5xx：持久化且不再写 WARN 行；结构化字段完整。
func TestRecorderPersistsRelayApi5xxWithoutWarnLine(t *testing.T) {
	buffer := logtest.New(t)
	capture := &persistCapture{}
	capture.register(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "relay") },
		func(c *gin.Context) {
			c.Set("id", 7)
			c.Set("username", "customer_a")
			c.Set("token_name", "key-prod-1")
			clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
		},
		func(c *gin.Context) {
			c.Set(common.UpstreamRequestIdKey, "upstream-req-9")
			c.Status(http.StatusBadGateway)
		},
	)

	response := postAsset(engine)

	require.Equal(t, http.StatusBadGateway, response.Code)
	// 5xx 无 WARN 交付，不能用 buffer.String()（它会等待写端交付）；用计数器断言。
	assert.Eventually(t, func() bool { return len(capture.snapshot()) == 1 }, 2*time.Second, 10*time.Millisecond)
	events := capture.snapshot()
	require.Len(t, events, 1)
	event := events[0]
	assert.Equal(t, "relay", event.Module)
	assert.Equal(t, http.StatusBadGateway, event.Status)
	assert.Equal(t, 7, event.UserID)
	assert.Equal(t, "customer_a", event.Username)
	assert.Equal(t, "key-prod-1", event.TokenName)
	assert.Equal(t, "upstream-req-9", event.UpstreamRequestID)
	assert.Equal(t, "req-fixed-1", event.RequestID)
	assert.Equal(t, "unclassified", event.Stage)
}

// relay 的最终 4xx：WARN 行与持久化各一份，结构化字段与日志行一致。
func TestRecorderPersistsRelay4xxAlongsideWarnLine(t *testing.T) {
	buffer := logtest.New(t)
	capture := &persistCapture{}
	capture.register(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "relay") },
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
		func(c *gin.Context) { c.Status(http.StatusBadRequest) },
	)

	postAsset(engine)

	log := buffer.String()
	require.Equal(t, 1, strings.Count(log, "authenticated_api_client_error"), log)
	events := capture.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, "relay", events[0].Module)
	assert.Equal(t, 400, events[0].Status)
}

// dashboard（api 模块）4xx：仅 WARN 行，不持久化。
func TestRecorderDashboardApi4xxNotPersisted(t *testing.T) {
	buffer := logtest.New(t)
	capture := &persistCapture{}
	capture.register(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "api") },
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceSession) },
		func(c *gin.Context) { c.Status(http.StatusBadRequest) },
	)

	postAsset(engine)

	require.Equal(t, 1, strings.Count(buffer.String(), "authenticated_api_client_error"), buffer.String())
	assert.Empty(t, capture.snapshot())
}

// dashboard（api 模块）5xx：两个出口都不可达，事件完全不入队，accepted 可对账。
func TestRecorderDashboardApi5xxNeverEnqueued(t *testing.T) {
	buffer := logtest.New(t)
	capture := &persistCapture{}
	capture.register(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "api") },
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceSession) },
		func(c *gin.Context) { c.Status(http.StatusInternalServerError) },
	)

	postAsset(engine)

	assert.Empty(t, buffer.String())
	assert.Empty(t, capture.snapshot())
	health := buffer.Health()
	assert.Zero(t, health.Accepted)
}

// persister 失败被独立计数，不影响 WARN 出口与请求响应。
func TestRecorderPersisterFailureCountedSeparately(t *testing.T) {
	buffer := logtest.New(t)
	clienterrlog.SetEventPersister(func(e clienterrlog.Event) error {
		return errors.New("db down")
	})
	t.Cleanup(func() { clienterrlog.SetEventPersister(nil) })
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "relay") },
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
		func(c *gin.Context) { c.Status(http.StatusBadGateway) },
	)

	response := postAsset(engine)

	require.Equal(t, http.StatusBadGateway, response.Code)
	health := buffer.Health()
	assert.Equal(t, uint64(1), health.Accepted, "5xx 事件仍入队（持久化出口）")
	assert.Eventually(t, func() bool {
		return buffer.Health().PersistFailed == 1
	}, 2*time.Second, 10*time.Millisecond)
}

// asset 模块最终 5xx：持久化（素材 API 属于 API 调用流量）。
func TestRecorderPersistsAsset5xx(t *testing.T) {
	buffer := logtest.New(t)
	capture := &persistCapture{}
	capture.register(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "asset") },
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
		func(c *gin.Context) { c.Status(http.StatusInternalServerError) },
	)

	postAsset(engine)

	assert.Eventually(t, func() bool { return len(capture.snapshot()) == 1 }, 2*time.Second, 10*time.Millisecond)
	events := capture.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, "asset", events[0].Module)
	assert.Equal(t, 500, events[0].Status)
}
