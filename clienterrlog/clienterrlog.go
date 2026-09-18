// Package clienterrlog records one diagnostic event per API request that passed
// the formal authentication entrance and finally returned a 4xx/5xx to the caller.
//
// 边界（docs/80-dev/2026-09-16-系统4xx日志缺失问题分析与修复方案.md、
// docs/80-dev/2026-09-17-全量错误日志独立菜单分析与方案.md）：
//   - `MarkAuthPassed` 只表示本请求已通过正式鉴权入口并放行下游，不参与权限判定，
//     不持久化、不缓存，不构成另一套认证事实源。
//   - 健康容量内每个「已鉴权 + 最终 4xx」请求写一条 WARN；故障时业务优先，丢失可观察。
//   - relay/asset 模块的 API 调用最终 4xx/5xx 额外交由注册的 persister 持久化到
//     错误事件表（仅本模块；dashboard/web 等页面流量不持久化）。
//   - 业务失败处通过 `Attach` 提供白名单诊断（阶段/原因码/受控上下文）；未分类的
//     事件以 reason=unclassified 记录，不因分类缺失而漏记。
//   - 普通文本日志不包含请求/响应正文；管理员错误详情另存有界脱敏 JSON 快照。
//     查询参数、Cookie、凭据、源 URL、签名参数和媒体二进制不进入快照。
package clienterrlog

import (
	"context"
	"strings"
	"sync"
	"unicode"

	"github.com/gin-gonic/gin"
)

const eventName = "authenticated_api_client_error"

// Matches middleware.RouteTagKey without introducing an import cycle.
const routeTagKey = "route_tag"

// 已成功放行的正式鉴权入口来源。
const (
	AuthSourceAPIToken         = "api_token"
	AuthSourceAPITokenReadOnly = "api_token_readonly"
	AuthSourceSession          = "session"
	AuthSourcePAT              = "personal_access_token"
	AuthSourceArtifactAccess   = "task_artifact_access"
)

// 身份字段异常时的受控标注；不因此丢弃已鉴权请求。
const identityContextMissing = "missing"

type authPassMarker struct {
	Source string
	UserID int
	Route  string
	Module string
}

// MarkAuthPassed freezes the formal entrance's identity and route in the shared
// request carrier, including when authentication runs in a nested Gin engine.
func MarkAuthPassed(c *gin.Context, source string) {
	defer isolateDiagnosticPanic()
	target, ok := c.Request.Context().Value(contextKey{}).(*requestReport)
	if !ok {
		return
	}
	switch source {
	case AuthSourceAPIToken, AuthSourceAPITokenReadOnly, AuthSourceSession, AuthSourcePAT, AuthSourceArtifactAccess:
	default:
		source = "unknown"
	}
	marker := authPassMarker{Source: source, UserID: c.GetInt("id"), Route: routeTemplateField(c), Module: moduleField(c)}
	target.mu.Lock()
	defer target.mu.Unlock()
	target.auth = marker
	target.authPassed = true
}

// Report 是业务失败处提供的一次白名单诊断。空字段被忽略；同一次请求内先到的
// Stage/Reason/PublicCode/Model/ChannelID 优先（最深失败点先记录），Detail 仅补充缺失键。
type Report struct {
	Stage      string
	Reason     string
	PublicCode string
	Model      string
	ChannelID  int
	Protocol   string
	Detail     map[string]string
}

type contextKey struct{}

type streamDiag struct {
	set        bool
	reason     string
	endReason  string
	severity   string
	errorCount int
}

type requestReport struct {
	mu         sync.Mutex
	auth       authPassMarker
	authPassed bool
	stage      string
	reason     string
	publicCode string
	model      string
	channelID  int
	protocol   string
	detail     map[string]string
	stream     streamDiag
}

// Attach 把诊断合并进当前请求的统一事件；ctx 未携带记录载体时为安全 no-op。
func Attach(ctx context.Context, report Report) {
	defer isolateDiagnosticPanic()
	if ctx == nil {
		return
	}
	target, ok := ctx.Value(contextKey{}).(*requestReport)
	if !ok {
		return
	}
	target.merge(report)
}

// PeekReport 返回当前请求已合并的诊断，用于测试与进程内检查。
func PeekReport(ctx context.Context) (Report, bool) {
	if ctx == nil {
		return Report{}, false
	}
	target, ok := ctx.Value(contextKey{}).(*requestReport)
	if !ok {
		return Report{}, false
	}
	target.mu.Lock()
	defer target.mu.Unlock()
	return Report{
		Stage:      target.stage,
		Reason:     target.reason,
		PublicCode: target.publicCode,
		Model:      target.model,
		ChannelID:  target.channelID,
		Protocol:   target.protocol,
		Detail:     copiesDetail(target.detail),
	}, true
}

// StreamErrorReport 携带一次受控流式异常观察。Reason 必须是固定原因码，
// 不保存原始响应内容；正常完成的请求不得产生该报告。
type StreamErrorReport struct {
	Reason     string
	EndReason  string
	Severity   string
	ErrorCount int
}

// AttachStreamError 把流式异常观察合并进当前请求的统一事件载体；同请求先到的
// 观察优先。ctx 未携带载体时为安全 no-op，自身 panic 被隔离。
func AttachStreamError(ctx context.Context, report StreamErrorReport) {
	defer isolateDiagnosticPanic()
	if ctx == nil {
		return
	}
	target, ok := ctx.Value(contextKey{}).(*requestReport)
	if !ok {
		return
	}
	target.mu.Lock()
	defer target.mu.Unlock()
	if target.stream.set {
		return
	}
	target.stream = streamDiag{
		set:        true,
		reason:     SanitizeLogValue(report.Reason, 64),
		endReason:  SanitizeLogValue(report.EndReason, 32),
		severity:   SanitizeLogValue(report.Severity, 16),
		errorCount: report.ErrorCount,
	}
}

func (r *requestReport) merge(report Report) {
	if reportEmpty(report) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stage == "" {
		r.stage = SanitizeLogValue(report.Stage, 64)
	}
	if r.reason == "" {
		r.reason = SanitizeLogValue(report.Reason, 64)
	}
	if r.publicCode == "" {
		r.publicCode = SanitizeLogValue(report.PublicCode, 64)
	}
	if r.model == "" {
		r.model = SanitizeLogValue(report.Model, 128)
	}
	if r.protocol == "" {
		r.protocol = SanitizeLogValue(report.Protocol, 64)
	}
	if r.channelID <= 0 && report.ChannelID > 0 {
		r.channelID = report.ChannelID
	}
	// Iterate the fixed whitelist, never arbitrary input keys or an unbounded map.
	for _, key := range []string{"asset_kind", "media_type", "group_kind", "operation", "field", "source_status", "source_content_type", "required_min_ttl_seconds", "decoded_width", "decoded_height"} {
		if _, exists := r.detail[key]; exists {
			continue
		}
		value := SanitizeLogValue(report.Detail[key], 128)
		if value == "" {
			continue
		}
		if r.detail == nil {
			r.detail = map[string]string{}
		}
		r.detail[key] = value
	}

}

func reportEmpty(report Report) bool {
	return report.Stage == "" && report.Reason == "" && report.PublicCode == "" &&
		report.Model == "" && report.Protocol == "" && report.ChannelID <= 0 && len(report.Detail) == 0
}

func copiesDetail(detail map[string]string) map[string]string {
	if len(detail) == 0 {
		return nil
	}
	copied := make(map[string]string, len(detail))
	for key, value := range detail {
		copied[key] = value
	}
	return copied
}

// SanitizeLogValue 去掉控制字符、把空白折叠为下划线并限制长度，保证事件恒为单行
// 值的语义白名单由调用方负责；长度和控制字符处理不等于秘密脱敏。
func SanitizeLogValue(value string, maxRunes int) string {
	if len(value) > maxRunes*4 {
		value = value[:maxRunes*4]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsControl(character) {
			continue
		}
		if unicode.IsSpace(character) {
			builder.WriteRune('_')
			continue
		}
		builder.WriteRune(character)
		if builder.Len() >= maxRunes*4 {
			break
		}
	}
	result := []rune(strings.TrimSpace(builder.String()))
	if len(result) > maxRunes {
		result = result[:maxRunes]
	}
	return string(result)
}
