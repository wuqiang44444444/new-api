package clienterrlog

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func withReport(ctx context.Context, report *requestReport) context.Context {
	return context.WithValue(ctx, contextKey{}, report)
}

// Recorder 是注册在 RequestId 之后的统一记录出口：请求结束时检查
// 「正式鉴权成功标记 + 最终状态为 4xx」，满足则写一条 WARN 事件。
// 未标记成功的请求不产生事件；不重新执行鉴权，不改变请求处理顺序。
func Recorder() gin.HandlerFunc { return defaultSink.Middleware() }

func (s *Sink) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		report := installReport(c)
		c.Next() // Business panics belong exclusively to the existing Recovery.
		s.record(c, startedAt, report)
	}
}

func installReport(c *gin.Context) (report *requestReport) {
	defer isolateDiagnosticPanic()
	report = &requestReport{}
	c.Request = c.Request.WithContext(withReport(c.Request.Context(), report))
	return report
}

func (s *Sink) record(c *gin.Context, startedAt time.Time, report *requestReport) {
	defer isolateDiagnosticPanic()
	status := c.Writer.Status()
	if report == nil || status < http.StatusBadRequest || status >= 500 {
		return
	}
	report.mu.Lock()
	marker, passed := report.auth, report.authPassed
	report.mu.Unlock()
	if !passed {
		return
	}
	s.submit(buildClientErrorEvent(c, startedAt, status, marker, report))
}

func buildClientErrorEvent(c *gin.Context, startedAt time.Time, status int, marker authPassMarker, report *requestReport) Event {
	message :=
		"event=" + eventName +
			" status=" + strconv.Itoa(status) +
			" method=" + SanitizeLogValue(c.Request.Method, 32) +
			" route=" + marker.Route +
			" module=" + marker.Module +
			" request_id=" + SanitizeLogValue(c.GetString(common.RequestIdKey), 64) +
			" user_id=" + strconv.Itoa(marker.UserID) +
			" identity=" + marker.Source
	if marker.UserID <= 0 {
		message += " identity_context=" + identityContextMissing
	}
	message += " elapsed_ms=" + strconv.FormatInt(time.Since(startedAt).Milliseconds(), 10)

	report.mu.Lock()
	stage, reason, publicCode, model, channelID, protocol := report.stage, report.reason, report.publicCode, report.model, report.channelID, report.protocol
	detail := copiesDetail(report.detail)
	report.mu.Unlock()

	if stage == "" {
		stage = "unclassified"
	}
	message += " stage=" + stage
	if reason == "" {
		reason = "unclassified"
	}
	message += " reason=" + reason
	if publicCode != "" {
		message += " public_code=" + publicCode
	}
	if model != "" {
		message += " model=" + model
	}
	if channelID > 0 {
		message += " channel_id=" + strconv.Itoa(channelID)
	}
	if protocol != "" {
		message += " protocol=" + protocol
	}
	for _, key := range sortedDetailKeys(detail) {
		message += " detail." + key + "=" + detail[key]
	}
	return Event{At: time.Now(), RequestID: SanitizeLogValue(c.GetString(common.RequestIdKey), 64), Message: message}
}

// routeTemplateField 输出 gin 路由模板；无模板（未匹配路由）时用受控分类，不输出原始 URL。
func routeTemplateField(c *gin.Context) string {
	if template := SanitizeLogValue(c.FullPath(), 256); template != "" {
		return template
	}
	return "unmatched"
}

// moduleField 输出路由上的 RouteTag 受控模块分类（relay/asset/api/old_api/web），缺失记 unset。
func moduleField(c *gin.Context) string {
	switch tag := c.GetString(routeTagKey); tag {
	case "relay", "asset", "api", "old_api", "web":
		return tag
	default:
		return "unset"
	}
}

func sortedDetailKeys(detail map[string]string) []string {
	keys := make([]string, 0, len(detail))
	for key := range detail {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
