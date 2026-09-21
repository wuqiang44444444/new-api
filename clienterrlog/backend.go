package clienterrlog

import (
	"github.com/QuantumNous/new-api/common"
	"strconv"
	"time"
)

// 事件类型常量：唯一区分事件类型。module 继续描述流量域、stage 继续描述失败阶段，
// 两者不承担类型语义（docs/80-dev/2026-09-17-全量错误日志独立菜单分析与方案.md 扩展节）。
const (
	EventAPIError    = "api_error"
	EventChannelTest = "channel_test"
	EventStreamError = "stream_error"
	EventTaskFailure = "task_failure"
)

// ValidEventType 校验事件类型白名单；空值表示第一轮遗留生产者，按 api_error 归类。
func ValidEventType(eventType string) bool {
	switch eventType {
	case "", EventAPIError, EventChannelTest, EventStreamError, EventTaskFailure:
		return true
	}
	return false
}

// BackendEvent 是后台来源（渠道测试、任务失败）的受控事件快照。它不携带 gin
// Context、请求对象或凭据；全部字符串字段在 buildBackendEvent 中净化限长。
// Status 为 0 表示该事件没有客户 HTTP 状态；渠道测试的上游状态独立保存在 Detail。
type BackendEvent struct {
	EventType         string
	Module            string // 流量域（relay），不承担类型语义
	Method            string
	Route             string // 合成测试请求的纯路径，不含查询参数
	Protocol          string
	Stage             string
	Reason            string
	PublicCode        string
	Model             string
	ChannelID         int
	UserID            int
	Username          string
	TaskID            string
	RequestID         string // 请求级关联 ID 或测试关联 ID
	UpstreamRequestID string
	Status            int
	ElapsedMs         int64
	Detail            map[string]string
	HTTPExchange      *HTTPExchange
}

// SubmitBackendEvent 把后台事件提交到进程级默认 sink：非阻塞、满载丢弃可观察、
// 不为单条事件启动 goroutine。未知事件类型按诊断故障计数并丢弃，不猜测归类。
func SubmitBackendEvent(event BackendEvent) {
	if !ValidEventType(event.EventType) {
		diagnosticFailures.Add(1)
		return
	}
	defaultSink.Submit(buildBackendEvent(event))
}

func buildBackendEvent(event BackendEvent) Event {
	requestID := SanitizeLogValue(event.RequestID, 64)
	taskID := SanitizeLogValue(event.TaskID, 64)
	message := "event=" + event.EventType
	if requestID != "" {
		message += " request_id=" + requestID
	}
	if event.ChannelID > 0 {
		message += " channel_id=" + strconv.Itoa(event.ChannelID)
	}
	if taskID != "" {
		message += " task_id=" + taskID
	}
	if event.Reason != "" {
		message += " reason=" + SanitizeLogValue(event.Reason, 64)
	}
	return Event{
		At:                time.Now(),
		RequestID:         requestID,
		Message:           message,
		EventType:         event.EventType,
		Module:            SanitizeLogValue(event.Module, 16),
		Method:            SanitizeLogValue(event.Method, 16),
		Route:             SanitizeLogValue(event.Route, 255),
		Protocol:          SanitizeLogValue(event.Protocol, 64),
		Stage:             SanitizeLogValue(event.Stage, 64),
		Reason:            SanitizeLogValue(event.Reason, 64),
		PublicCode:        SanitizeLogValue(event.PublicCode, 64),
		Model:             SanitizeLogValue(event.Model, 128),
		ChannelID:         event.ChannelID,
		UserID:            event.UserID,
		Username:          SanitizeLogValue(event.Username, 64),
		TaskID:            taskID,
		UpstreamRequestID: SanitizeLogValue(event.UpstreamRequestID, 128),
		Status:            event.Status,
		ElapsedMs:         event.ElapsedMs,
		Detail:            sanitizedBackendDetail(event.Detail),
		HTTPExchange:      event.HTTPExchange,
	}
}

// sanitizedBackendDetail 由固定白名单限制键数，并限制每值长度；完整保留允许的诊断字段。
func sanitizedBackendDetail(detail map[string]string) map[string]string {
	if len(detail) == 0 {
		return nil
	}
	sanitized := make(map[string]string)
	for key, value := range detail {
		switch key {
		case "platform", "action", "create_upstream_request_id", "fail_reason", "test_mode", "endpoint_type", "probe_stream", "upstream_status", "threshold_kind",
			"check_scope", "config_check", "generation_evidence", "upstream_request", "config_reason", "config_entry", "config_summary", "readonly_check", "billing_model", "check_result", "check_reason", "check_code", "connection_result", "probe_media", "upstream_cost_status":
		default:
			continue
		}
		if key == "fail_reason" {
			value = common.PublicTaskErrorMessage(value)
		}
		cleanKey := SanitizeLogValue(key, 32)
		if cleanKey == "" {
			continue
		}
		value = SanitizeLogValue(value, 128)
		if value == "" {
			continue
		}
		sanitized[cleanKey] = value
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}
