package clienterrlog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(common.RequestIdKey, "req-fixed-1")
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), common.RequestIdKey, "req-fixed-1"))
		c.Next()
	}
}

func newRecorderEngine(buffer *logtest.Capture, handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(fixedRequestID(), buffer.Middleware())
	engine.POST("/v1/assets", handlers...)
	return engine
}

func postAsset(engine *gin.Engine) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/assets", nil)
	engine.ServeHTTP(recorder, request)
	return recorder
}

// 已鉴权 + 最终 4xx：恰好一条事件，字段齐全。
func TestRecorderWritesExactlyOneEventForAuthenticated4xx(t *testing.T) {
	buffer := logtest.New(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { c.Set("route_tag", "asset") },
		func(c *gin.Context) { c.Set("id", 7); clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
		func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "x"})
		},
	)

	response := postAsset(engine)

	require.Equal(t, http.StatusBadRequest, response.Code)
	log := buffer.String()
	assert.Equal(t, 1, strings.Count(log, "authenticated_api_client_error"), log)
	assert.Contains(t, log, "event="+"authenticated_api_client_error")
	assert.Contains(t, log, "status=400")
	assert.Contains(t, log, "method=POST")
	assert.Contains(t, log, "route=/v1/assets")
	assert.Contains(t, log, "module=asset")
	assert.Contains(t, log, "request_id=req-fixed-1")
	assert.Contains(t, log, "user_id=7")
	assert.Contains(t, log, "identity=api_token")
	assert.Contains(t, log, "reason=unclassified")
	assert.NotContains(t, log, "identity_context=")
}

// 未通过正式鉴权的请求不产生事件，即使上下文已有用户 ID。
func TestRecorderSkipsRequestsThatNeverPassedAuth(t *testing.T) {
	buffer := logtest.New(t)
	engine := newRecorderEngine(buffer, func(c *gin.Context) {
		c.Set("id", 7) // 失败请求也可能有 user_id，不得据此记录
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "x"})
	})

	response := postAsset(engine)

	require.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Empty(t, buffer.String())
}

// 完整 4xx 区间判断：399/500 不记录，400/401/403/404/422/429/499 每请求一条。
func TestRecorderCoversExact4xxRange(t *testing.T) {
	for _, tc := range []struct {
		status int
		events int
	}{
		{200, 0}, {204, 0}, {302, 0}, {399, 0}, {400, 1}, {401, 1}, {403, 1}, {404, 1}, {422, 1}, {429, 1}, {499, 1}, {500, 0}, {502, 0},
	} {
		buffer := logtest.New(t)
		engine := newRecorderEngine(buffer,
			func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
			func(c *gin.Context) { c.Status(tc.status) },
		)
		response := postAsset(engine)
		require.Equal(t, tc.status, response.Code, "status %d", tc.status)
		assert.Equal(t, tc.events, strings.Count(buffer.String(), "authenticated_api_client_error"), "status %d: %s", tc.status, buffer.String())
	}
}

// clienterrlog.Attach 合并语义：先到者优先，Detail 只补缺失键；clienterrlog.PeekReport 可读回合并结果。
func TestAttachMergesFirstWriterWins(t *testing.T) {
	buffer := logtest.New(t)
	var merged clienterrlog.Report
	var hasReport bool
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
		func(c *gin.Context) {
			clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{Stage: "source_fetch", Reason: "timeout"})
			clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
				Stage: "content_validation", Reason: "invalid_image",
				PublicCode: "invalid_request", Model: "customer-model", ChannelID: 5,
				Detail: map[string]string{"asset_kind": "general", "media_type": "image"},
			})
			merged, hasReport = clienterrlog.PeekReport(c.Request.Context())
			c.Status(http.StatusBadRequest)
		},
	)

	postAsset(engine)

	log := buffer.String()
	require.Equal(t, 1, strings.Count(log, "authenticated_api_client_error"), log)
	assert.Contains(t, log, "stage=source_fetch")
	assert.Contains(t, log, "reason=timeout")
	assert.Contains(t, log, "public_code=invalid_request")
	assert.Contains(t, log, "model=customer-model")
	assert.Contains(t, log, "channel_id=5")
	assert.Contains(t, log, "detail.asset_kind=general")
	assert.Contains(t, log, "detail.media_type=image")
	assert.NotContains(t, log, "invalid_image")
	require.True(t, hasReport)
	assert.Equal(t, "source_fetch", merged.Stage)
	assert.Equal(t, "timeout", merged.Reason)
	assert.Equal(t, "invalid_request", merged.PublicCode)
	assert.Equal(t, 5, merged.ChannelID)
}

// clienterrlog.PeekReport 在无记录载体（未注册 Recorder）的 ctx 上安全返回 false。
func TestPeekReportWithoutCarrierIsNoop(t *testing.T) {
	report, ok := clienterrlog.PeekReport(context.Background())
	assert.False(t, ok)
	assert.Empty(t, report.Stage)
	clienterrlog.Attach(context.Background(), clienterrlog.Report{Stage: "x"}) // 不得 panic
}

// 敏感与异常文本：控制字符被剔除、超长被截断，事件恒为单行。
func TestRecorderSanitizesValuesAndKeepsSingleLine(t *testing.T) {
	buffer := logtest.New(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
		func(c *gin.Context) {
			clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
				Model:  "bad\nmodel\twith ctrl\x00chars and   spaces",
				Detail: map[string]string{"source_content_type": strings.Repeat("x", 500) + "\nline2"},
			})
			c.Status(http.StatusUnprocessableEntity)
		},
	)

	postAsset(engine)

	log := buffer.String()
	require.Equal(t, 1, strings.Count(log, "\n"), "event must stay single line: %q", log)
	assert.Contains(t, log, "model=badmodelwith_ctrlchars_and___spaces")
	assert.NotContains(t, log, "line2")
	assert.True(t, len(log) < 1024, "log line should be bounded")
}

// 身份字段缺失：保留事件并明确标注，不丢弃已鉴权请求。
func TestRecorderKeepsEventWhenIdentityMissing(t *testing.T) {
	buffer := logtest.New(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceSession) },
		func(c *gin.Context) { c.Status(http.StatusBadRequest) },
	)

	postAsset(engine)

	log := buffer.String()
	assert.Equal(t, 1, strings.Count(log, "authenticated_api_client_error"), log)
	assert.Contains(t, log, "user_id=0")
	assert.Contains(t, log, "identity_context=missing")
}

// 范围内全量：连续重复 4xx 不采样、不去重。
func TestRecorderDoesNotSampleRepeatedErrors(t *testing.T) {
	buffer := logtest.New(t)
	engine := newRecorderEngine(buffer,
		func(c *gin.Context) { clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken) },
		func(c *gin.Context) { c.Status(http.StatusBadRequest) },
	)

	for i := 0; i < 3; i++ {
		postAsset(engine)
	}

	assert.Equal(t, 3, strings.Count(buffer.String(), "authenticated_api_client_error"), buffer.String())
}

// 未匹配路由不携带标记：不产生事件。
func TestRecorderSkipsUnmatchedRoutes(t *testing.T) {
	buffer := logtest.New(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(fixedRequestID(), buffer.Middleware())

	request := httptest.NewRequest(http.MethodPost, "/no/such/route", nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	assert.Empty(t, buffer.String())
}
