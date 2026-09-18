package service

import (
	"strings"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// 受控流式诊断标记：由适配器经 sr.Error 登记的固定 token；响应内容本身不参与匹配。
const (
	streamTokenResponseFailed     = "response_failed"
	streamTokenResponseIncomplete = "response_incomplete"
	streamTokenResponseCancelled  = "response_cancelled"
)

// ObserveRelayStreamError 在 Relay 退出时观察最终尝试的流式状态（仅观察）：
// 正常完成且无错误的请求不产生标记；显式异常经请求载体合并进请求级事件。
// StreamScannerHandler 返回前已 join 全部流式 goroutine，此处读取安全；
// 非扫描器路径可能保留前一尝试实例，属文档已列的观察限制。
func ObserveRelayStreamError(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if c == nil || c.Request == nil || relayInfo == nil || !relayInfo.IsStream {
		return
	}
	streamStatus := relayInfo.StreamStatus
	if streamStatus == nil {
		return
	}
	reason, severity := classifyRelayStreamStatus(streamStatus)
	if reason == "" {
		return
	}
	// 补充客户模型与最终渠道（relay 错误 Attach 仅在最终失败时触发）。
	clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
		Model:     c.GetString("original_model"),
		ChannelID: common.GetContextKeyInt(c, constant.ContextKeyChannelId),
	})
	clienterrlog.AttachStreamError(c.Request.Context(), clienterrlog.StreamErrorReport{
		Reason:     reason,
		EndReason:  string(streamStatus.EndReason),
		Severity:   severity,
		ErrorCount: streamStatus.TotalErrorCount(),
	})
}

// classifyRelayStreamStatus 区分致命失败、可恢复软错误与客户端断开；
// 返回空表示无需记录。带错误的 handler_stop 不被当成最终生成失败。
func classifyRelayStreamStatus(streamStatus *relaycommon.StreamStatus) (string, string) {
	switch streamStatus.EndReason {
	case relaycommon.StreamEndReasonTimeout:
		return "stream_timeout", "fatal"
	case relaycommon.StreamEndReasonScannerErr:
		return "stream_scanner_error", "fatal"
	case relaycommon.StreamEndReasonPanic:
		return "stream_handler_panic", "fatal"
	case relaycommon.StreamEndReasonPingFail:
		return "stream_ping_failed", "fatal"
	case relaycommon.StreamEndReasonClientGone:
		return "client_disconnected", "client"
	}
	if token := protocolFailureReason(streamStatus); token != "" {
		if token == "upstream_response_cancelled" {
			return token, "cancelled"
		}
		return token, "fatal"
	}
	if streamStatus.EndError != nil {
		return "stream_stopped_with_error", "soft"
	}
	if streamStatus.HasErrors() {
		return "stream_soft_errors", "soft"
	}
	return "", ""
}

// protocolFailureReason 识别适配器登记的协议失败 token（固定受控标记）。
func protocolFailureReason(streamStatus *relaycommon.StreamStatus) string {
	for i := range streamStatus.Errors {
		message := streamStatus.Errors[i].Message
		switch {
		case strings.Contains(message, streamTokenResponseFailed):
			return "upstream_response_failed"
		case strings.Contains(message, streamTokenResponseIncomplete):
			return "upstream_response_incomplete"
		case strings.Contains(message, streamTokenResponseCancelled):
			return "upstream_response_cancelled"
		}
	}
	return ""
}
