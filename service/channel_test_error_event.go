package service

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// 渠道测试失败事件的受控分类：只基于结构化错误码与本文件已知的固定消息前缀，
// 不做原始错误文本推断；无法分类时保持 unclassified。
const (
	channelTestModeManual  = "manual"
	channelTestModeAuto    = "auto"
	channelTestProtocolKey = "channel_test_protocol"

	ChannelTestReasonResponseTimeExceeded = "response_time_exceeded"
	ChannelTestStageResponseThreshold     = "threshold"
)

// RecordManualChannelTestFailureEvent 在手工测试出口登记一次失败事件：操作人是
// 当前管理员（不冒充客户调用），status 保存管理接口实际返回的 200。非阻塞提交，
// 不改变测试结果或响应。
func RecordManualChannelTestFailureEvent(c *gin.Context, channel *model.Channel, testedModel, endpointType string, isStream bool, testContext *gin.Context, localErr error, apiError *types.NewAPIError, upstreamStatus int, elapsedMs int64) {
	if channel == nil {
		return
	}
	stage, reason, publicCode := ClassifyChannelTestFailure(localErr, apiError, "")
	operatorID := 0
	operatorName := ""
	if c != nil {
		operatorID = c.GetInt("id")
		operatorName = c.GetString("username")
	}
	submitChannelTestFailureEvent(testContext, clienterrlog.BackendEvent{
		EventType:         clienterrlog.EventChannelTest,
		Module:            model.ErrorEventModuleRelay,
		Stage:             stage,
		Reason:            reason,
		PublicCode:        publicCode,
		Model:             testedModel,
		ChannelID:         channel.Id,
		UserID:            operatorID,
		Username:          operatorName,
		RequestID:         common.NewRequestId(),
		UpstreamRequestID: upstreamRequestIDFromTestContext(testContext),
		Status:            200,
		ElapsedMs:         elapsedMs,
		Detail:            channelTestEventDetail(channelTestModeManual, endpointType, isStream, upstreamStatus, nil),
	})
}

// ChannelAutoCheckResult is a per-channel observation in a system task run.
// Detail contains only controlled codes, fixed summary keys and model identifiers.
type ChannelAutoCheckResult struct {
	ChannelID int               `json:"channel_id"`
	Model     string            `json:"model"`
	CheckedAt int64             `json:"checked_at"`
	Detail    map[string]string `json:"detail"`
}

// A manual batch passes no checks and retains its existing event contract.
func RecordAutoChannelTestFailureEvent(channel *model.Channel, testedModel string, testContext *gin.Context, localErr error, finalAPIError *types.NewAPIError, upstreamStatus int, elapsedMs int64, thresholdExceeded bool, isStream bool, checks []ChannelAutoCheckResult) {
	if channel == nil {
		return
	}
	PrepareChannelTestAudit(testContext)
	observation := ChannelStatusAuditForRequest(testContext, finalAPIError)
	stage, reason, publicCode := ClassifyChannelTestFailure(localErr, finalAPIError, "")
	extra := map[string]string{}
	if len(checks) > 0 {
		testedModel = checks[0].Model
		for key, value := range checks[0].Detail {
			extra[key] = value
		}
	}
	if thresholdExceeded {
		stage, reason = ChannelTestStageResponseThreshold, ChannelTestReasonResponseTimeExceeded
		extra["threshold_kind"] = "response_time"
	}
	submitChannelTestFailureEvent(testContext, clienterrlog.BackendEvent{
		EventType: clienterrlog.EventChannelTest, Module: model.ErrorEventModuleRelay,
		Stage: stage, Reason: reason, PublicCode: publicCode, Model: testedModel,
		ChannelID: channel.Id, RequestID: observation.RequestID,
		UpstreamRequestID: upstreamRequestIDFromTestContext(testContext), ElapsedMs: elapsedMs,
		Detail: channelTestEventDetail(channelTestModeAuto, "", isStream, upstreamStatus, extra),
	})
}

// ClassifyChannelTestFailure 按结构化错误码分阶段与原因；前缀匹配只针对本仓库
// 固定错误文案（不支持类型、AzureBatch 连接、Seedance 探针）。
func ClassifyChannelTestFailure(localErr error, apiError *types.NewAPIError, reasonOverride string) (string, string, string) {
	if reasonOverride != "" {
		return "", reasonOverride, ""
	}
	if apiError != nil {
		code := string(apiError.GetErrorCode())
		switch {
		case apiError.GetErrorCode() == types.ErrorCodeChannelResponseTimeExceeded:
			return ChannelTestStageResponseThreshold, ChannelTestReasonResponseTimeExceeded, code
		case strings.Contains(code, "model_mapped_error"),
			strings.Contains(code, "param_override_invalid"),
			strings.Contains(code, "model_price_error"),
			strings.Contains(code, "invalid_api_type"):
			return "config", "test_config_error", code
		case strings.Contains(code, "convert_request_failed"), strings.Contains(code, "json_marshal_failed"):
			return "request_build", "test_request_build_failed", code
		case strings.Contains(code, "do_request_failed"):
			return "upstream_call", "test_upstream_unreachable", code
		default:
			return "upstream_response", "test_upstream_rejected", code
		}
	}
	if localErr != nil {
		message := localErr.Error()
		switch {
		case strings.Contains(message, "channel test is not supported"):
			return "capability", "test_unsupported_channel_type", ""
		case strings.Contains(message, "seedance link asset probe failed"):
			return "probe", "asset_probe_failed", ""
		case strings.Contains(message, "azure batch connection failed"):
			return "config", "test_azure_batch_connection_failed", ""
		}
	}
	return "unclassified", "unclassified", ""
}

func channelTestEventDetail(mode, endpointType string, isStream bool, upstreamStatus int, extra map[string]string) map[string]string {
	detail := map[string]string{"test_mode": mode}
	if endpointType != "" {
		detail["endpoint_type"] = endpointType
	}
	if isStream {
		detail["probe_stream"] = "true"
	}
	if upstreamStatus > 0 {
		detail["upstream_status"] = strconv.Itoa(upstreamStatus)
	}
	for key, value := range extra {
		if _, exists := detail[key]; !exists && value != "" {
			detail[key] = value
		}
	}
	return detail
}

func upstreamRequestIDFromTestContext(testContext *gin.Context) string {
	if testContext == nil {
		return ""
	}
	return testContext.GetString(common.UpstreamRequestIdKey)
}

func submitChannelTestFailureEvent(c *gin.Context, event clienterrlog.BackendEvent) {
	if status := ChannelTestUpstreamCostStatus(c); status != "" {
		if event.Detail == nil {
			event.Detail = make(map[string]string)
		}
		event.Detail[ChannelTestUpstreamCostKey] = status
	}
	if c != nil && c.Request != nil {
		event.Method = c.Request.Method
		event.Route = c.Request.URL.Path
		event.Protocol = c.GetString(channelTestProtocolKey)
		event.HTTPExchange = clienterrlog.SnapshotHTTPExchange(c.Request.Context())
	}
	clienterrlog.SubmitBackendEvent(event)
}

func ObserveChannelTestRequest(c *gin.Context, request any, protocol string) {
	c.Set(channelTestProtocolKey, protocol)
	if body, err := common.Marshal(request); err == nil {
		clienterrlog.ObserveHTTPBody(c.Request.Context(), "request", body, 0)
	}
}
