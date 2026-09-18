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
		InstallHTTPExchange(c)
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

// record accepts authenticated requests whose final status is 4xx/5xx (api_error
// events) plus 2xx/3xx requests that carry an explicit stream-error observation
// (stream_error events, kept at their real committed status). The outlet split
// happens at submission: WARN log lines stay 4xx-only across all modules, while
// relay/asset events go to the persister. Events that can reach no outlet are
// never enqueued, so the accepted counter keeps its accounting invariant.
func (s *Sink) record(c *gin.Context, startedAt time.Time, report *requestReport) {
	defer isolateDiagnosticPanic()
	status := c.Writer.Status()
	if report == nil || status < http.StatusOK || status >= 600 {
		return
	}
	report.mu.Lock()
	marker, passed := report.auth, report.authPassed
	stream := report.stream
	report.mu.Unlock()
	if !passed {
		return
	}
	if status >= 400 {
		// API 调用错误：提交时决定出口（WARN 全模块仅 4xx；持久化仅 relay/asset）。
		// 两个出口都不可达的事件不入队，accepted 计数恒可对账。
		logLine := status < 500
		persist := persistedModules[marker.Module]
		if !logLine && !persist {
			return
		}
		s.submit(buildClientErrorEvent(c, startedAt, status, marker, report))
		return
	}
	// HTTP 已以 2xx/3xx 提交：仅显式标记的流式异常产生事件，且仅持久化模块，
	// 不为流式异常伪造 4xx/5xx，也不改变 WARN 4xx 统计口径。
	if !persistedModules[marker.Module] || !stream.set {
		return
	}
	s.submit(buildStreamErrorEvent(c, startedAt, status, marker, stream, report))
}

func buildClientErrorEvent(c *gin.Context, startedAt time.Time, status int, marker authPassMarker, report *requestReport) Event {
	elapsedMs := time.Since(startedAt).Milliseconds()
	requestID := SanitizeLogValue(c.GetString(common.RequestIdKey), 64)
	message :=
		"event=" + eventName +
			" status=" + strconv.Itoa(status) +
			" method=" + SanitizeLogValue(c.Request.Method, 32) +
			" route=" + marker.Route +
			" module=" + marker.Module +
			" request_id=" + requestID +
			" user_id=" + strconv.Itoa(marker.UserID) +
			" identity=" + marker.Source
	if marker.UserID <= 0 {
		message += " identity_context=" + identityContextMissing
	}
	message += " elapsed_ms=" + strconv.FormatInt(elapsedMs, 10)

	report.mu.Lock()
	stage, reason, publicCode, model, channelID, protocol := report.stage, report.reason, report.publicCode, report.model, report.channelID, report.protocol
	stream := report.stream
	detail := copiesDetail(report.detail)
	report.mu.Unlock()
	if stream.set {
		// 最终已是 4xx/5xx 的请求把流式诊断合并进同一条事件，不重复记录。
		if detail == nil {
			detail = map[string]string{}
		}
		detail["stream_reason"] = stream.reason
		detail["stream_end_reason"] = stream.endReason
		detail["stream_severity"] = stream.severity
		if stream.errorCount > 0 {
			detail["stream_error_count"] = strconv.Itoa(stream.errorCount)
		}
	}

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
	return Event{
		At:                time.Now(),
		RequestID:         requestID,
		Message:           message,
		EventType:         EventAPIError,
		Module:            marker.Module,
		Method:            SanitizeLogValue(c.Request.Method, 32),
		Route:             marker.Route,
		Status:            status,
		UserID:            marker.UserID,
		Username:          SanitizeLogValue(c.GetString("username"), 64),
		TokenName:         SanitizeLogValue(c.GetString("token_name"), 64),
		UpstreamRequestID: SanitizeLogValue(c.GetString(common.UpstreamRequestIdKey), 128),
		Stage:             stage,
		Reason:            reason,
		PublicCode:        publicCode,
		Model:             model,
		ChannelID:         channelID,
		Protocol:          protocol,
		ElapsedMs:         elapsedMs,
		Detail:            detail,
		HTTPExchange:      SnapshotHTTPExchange(c.Request.Context()),
	}
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

// buildStreamErrorEvent 固化一条「HTTP 已 2xx 提交后的流式异常」事件：保留真实
// 提交状态，不伪造 4xx/5xx；身份、模块与路由沿用正式放行点冻结的载体事实。
func buildStreamErrorEvent(c *gin.Context, startedAt time.Time, status int, marker authPassMarker, stream streamDiag, report *requestReport) Event {
	requestID := SanitizeLogValue(c.GetString(common.RequestIdKey), 64)
	elapsedMs := time.Since(startedAt).Milliseconds()
	message := "event=api_stream_error" +
		" status=" + strconv.Itoa(status) +
		" module=" + marker.Module +
		" request_id=" + requestID +
		" reason=" + stream.reason +
		" severity=" + stream.severity +
		" elapsed_ms=" + strconv.FormatInt(elapsedMs, 10)
	detail := map[string]string{
		"stream_reason":     stream.reason,
		"stream_end_reason": stream.endReason,
		"stream_severity":   stream.severity,
	}
	if stream.errorCount > 0 {
		detail["stream_error_count"] = strconv.Itoa(stream.errorCount)
	}
	report.mu.Lock()
	model := report.model
	channelID := report.channelID
	report.mu.Unlock()
	if model != "" {
		message += " model=" + model
	}
	if channelID > 0 {
		message += " channel_id=" + strconv.Itoa(channelID)
	}
	return Event{
		At:                time.Now(),
		RequestID:         requestID,
		Message:           message,
		EventType:         EventStreamError,
		Module:            marker.Module,
		Method:            SanitizeLogValue(c.Request.Method, 32),
		Route:             marker.Route,
		Status:            status,
		UserID:            marker.UserID,
		Username:          SanitizeLogValue(c.GetString("username"), 64),
		TokenName:         SanitizeLogValue(c.GetString("token_name"), 64),
		UpstreamRequestID: SanitizeLogValue(c.GetString(common.UpstreamRequestIdKey), 128),
		Stage:             "stream",
		Reason:            stream.reason,
		Model:             model,
		ChannelID:         channelID,
		ElapsedMs:         elapsedMs,
		Detail:            detail,
		HTTPExchange:      SnapshotHTTPExchange(c.Request.Context()),
	}
}
