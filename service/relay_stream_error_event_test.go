package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 流式状态分类：致命结束原因、客户端断开、协议失败 token、软错误、正常结束。
func TestClassifyRelayStreamStatus(t *testing.T) {
	normal := relaycommon.NewStreamStatus()
	normal.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	clientGone := relaycommon.NewStreamStatus()
	clientGone.SetEndReason(relaycommon.StreamEndReasonClientGone, errors.New("context canceled"))
	softStop := relaycommon.NewStreamStatus()
	softStop.SetEndReason(relaycommon.StreamEndReasonHandlerStop, errors.New("write failed once"))
	softOnly := relaycommon.NewStreamStatus()
	softOnly.RecordError("transient decode issue")
	protocolFailed := relaycommon.NewStreamStatus()
	protocolFailed.RecordError("response_failed")
	protocolIncomplete := relaycommon.NewStreamStatus()
	protocolIncomplete.RecordError("response_incomplete")
	protocolCancelled := relaycommon.NewStreamStatus()
	protocolCancelled.RecordError("response_cancelled")

	for _, tc := range []struct {
		name     string
		status   *relaycommon.StreamStatus
		reason   string
		severity string
	}{
		{"normal done", normal, "", ""},
		{"client gone", clientGone, "client_disconnected", "client"},
		{"timeout", mustEnd(relaycommon.StreamEndReasonTimeout), "stream_timeout", "fatal"},
		{"scanner error", mustEnd(relaycommon.StreamEndReasonScannerErr), "stream_scanner_error", "fatal"},
		{"panic", mustEnd(relaycommon.StreamEndReasonPanic), "stream_handler_panic", "fatal"},
		{"handler stop with error", softStop, "stream_stopped_with_error", "soft"},
		{"soft errors only", softOnly, "stream_soft_errors", "soft"},
		{"protocol failed", protocolFailed, "upstream_response_failed", "fatal"},
		{"protocol incomplete", protocolIncomplete, "upstream_response_incomplete", "fatal"},
		{"protocol cancelled", protocolCancelled, "upstream_response_cancelled", "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, severity := classifyRelayStreamStatus(tc.status)
			assert.Equal(t, tc.reason, reason)
			assert.Equal(t, tc.severity, severity)
		})
	}
}

func mustEnd(reason relaycommon.StreamEndReason) *relaycommon.StreamStatus {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(reason, errors.New("boom"))
	return status
}

// 渠道测试失败分类：结构化错误码、固定消息前缀与未分类兜底。
func TestClassifyChannelTestFailure(t *testing.T) {
	configErr := types.NewError(fmt.Errorf("bad mapping"), types.ErrorCodeChannelModelMappedError)
	doReqErr := types.NewError(fmt.Errorf("dial tcp: timeout"), types.ErrorCodeDoRequestFailed)
	badRespErr := types.NewError(fmt.Errorf("upstream 503"), types.ErrorCodeBadResponse)

	for _, tc := range []struct {
		name       string
		localErr   error
		apiError   *types.NewAPIError
		wantStage  string
		wantReason string
	}{
		{"config error", nil, configErr, "config", "test_config_error"},
		{"upstream unreachable", nil, doReqErr, "upstream_call", "test_upstream_unreachable"},
		{"upstream rejected", nil, badRespErr, "upstream_response", "test_upstream_rejected"},
		{"unsupported type", errors.New("OpenAI channel test is not supported"), nil, "capability", "test_unsupported_channel_type"},
		{"seedance probe", errors.New("seedance link asset probe failed: boom"), nil, "probe", "asset_probe_failed"},
		{"azure batch", errors.New("azure batch connection failed: boom"), nil, "config", "test_azure_batch_connection_failed"},
		{"unclassified", errors.New("something else"), nil, "unclassified", "unclassified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage, reason, _ := ClassifyChannelTestFailure(tc.localErr, tc.apiError, "")
			assert.Equal(t, tc.wantStage, stage)
			assert.Equal(t, tc.wantReason, reason)
		})
	}
}

// 观察器端到端：异常流式状态经请求载体进入 stream_error 事件；载体的流式
// 诊断保存在专用字段而非 Detail。
func TestObserveRelayStreamErrorMergesIntoCarrier(t *testing.T) {
	buffer := logtest.New(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(buffer.Middleware())
	engine.POST("/v1/stream", func(c *gin.Context) {
		c.Set("route_tag", "relay")
		clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
		relayInfo := &relaycommon.RelayInfo{IsStream: true, StreamStatus: relaycommon.NewStreamStatus()}
		relayInfo.StreamStatus.SetEndReason(relaycommon.StreamEndReasonTimeout, errors.New("deadline"))
		ObserveRelayStreamError(c, relayInfo)
		c.Status(http.StatusOK)
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/stream", nil))

	assert.EqualValues(t, 1, buffer.Health().Accepted, "异常流式状态产生一条 stream_error 事件")
	assert.Zero(t, buffer.Health().Written, "stream_error 不写 WARN 行")
}
