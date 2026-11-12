package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaymode "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const channelAlertSelectionKey = "channel_alert_selection"
const channelAlertHTTPKey = "channel_alert_http"

type channelAlertHTTP struct {
	status    int
	transport bool
	inactive  bool
}

func SetChannelAlertSelection(c *gin.Context, channel *model.Channel, slot int) {
	if !channelAlerts.enabled.Load() {
		return
	}
	if !channel.ChannelInfo.IsMultiKey {
		slot = -1
	}
	c.Set(channelAlertSelectionKey, model.ChannelAlertTaskScope{Revision: model.ChannelAlertRevision(channel), Slot: slot})
}

func BeginChannelAlertAttempt(c *gin.Context) {
	if channelAlerts.enabled.Load() {
		c.Set(channelAlertHTTPKey, channelAlertHTTP{})
	}
}

func RecordChannelAlertHTTP(c *gin.Context, response *http.Response, err error) {
	if !channelAlerts.enabled.Load() || c == nil {
		return
	}
	outcome := channelAlertHTTP{transport: err != nil && response == nil && !errors.Is(err, context.Canceled)}
	if response != nil {
		outcome.status = response.StatusCode
	}
	c.Set(channelAlertHTTPKey, outcome)
}

func MarkChannelAlertCredentialInactive(c *gin.Context, status int) {
	if channelAlerts.enabled.Load() {
		c.Set(channelAlertHTTPKey, channelAlertHTTP{status: status, inactive: true})
	}
}

func ChannelAlertSelection(c *gin.Context) *model.ChannelAlertTaskScope {
	if !channelAlerts.enabled.Load() {
		return nil
	}
	raw, ok := c.Get(channelAlertSelectionKey)
	if !ok {
		return nil
	}
	snapshot, ok := raw.(model.ChannelAlertTaskScope)
	if !ok {
		return nil
	}
	return &snapshot
}

func channelAlertOperation(mode int) string {
	switch mode {
	case relaymode.RelayModeChatCompletions:
		return "chat"
	case relaymode.RelayModeResponses:
		return "responses"
	case relaymode.RelayModeResponsesCompact:
		return "responses_compact"
	case relaymode.RelayModeImagesGenerations:
		return "image_generation"
	case relaymode.RelayModeImagesEdits:
		return "image_edit"
	case relaymode.RelayModeEmbeddings:
		return "embeddings"
	case relaymode.RelayModeAudioSpeech:
		return "speech"
	case relaymode.RelayModeAudioTranscription:
		return "transcription"
	case relaymode.RelayModeAudioTranslation:
		return "audio_translation"
	case relaymode.RelayModeRealtime:
		return "realtime"
	case relaymode.RelayModeGemini:
		return "gemini"
	case relaymode.RelayModeCompletions:
		return "completion"
	case relaymode.RelayModeModerations:
		return "moderation"
	case relaymode.RelayModeRerank:
		return "rerank"
	case relaymode.RelayModeAlphaSearch:
		return "search"
	}
	return "relay"
}

func classifyChannelAlertHTTP(status int, transport bool) channelAlertKind {
	switch {
	case transport || status == http.StatusRequestTimeout:
		return channelAlertNetwork
	case status == http.StatusTooManyRequests:
		return channelAlertLimit
	case status >= 500 && status <= 599:
		return channelAlertService
	default:
		return channelAlertSuccess
	}
}

func recordChannelAlertAttempt(c *gin.Context, info *relaycommon.RelayInfo, operation, id string, success bool, code types.ErrorCode) {
	snapshot := ChannelAlertSelection(c)
	if snapshot == nil || info.ChannelMeta == nil || !success && c.Request.Context().Err() != nil {
		return
	}
	scope := channelAlertScope{Channel: info.ChannelId, Revision: snapshot.Revision, Model: info.OriginModelName, Operation: operation, Slot: snapshot.Slot}
	raw, _ := c.Get(channelAlertHTTPKey)
	httpResult, _ := raw.(channelAlertHTTP)
	if raw == nil {
		httpResult.status = clienterrlog.UpstreamResponseStatus(c.Request.Context())
	}
	kind := classifyChannelAlertHTTP(httpResult.status, httpResult.transport)
	if httpResult.inactive || httpResult.status == http.StatusUnauthorized && code == types.ErrorCode("invalid_api_key") {
		kind = channelAlertKey
	} else if httpResult.status == 0 && !httpResult.transport {
		switch code {
		case types.ErrorCodeChannelParamOverrideInvalid, types.ErrorCodeChannelHeaderOverrideInvalid, types.ErrorCodeChannelModelMappedError, types.ErrorCodeChannelInvalidKey:
			kind = channelAlertConfig
		}
	}
	if kind == channelAlertSuccess && !success && info.StreamStatus != nil && httpResult.status >= 200 && httpResult.status < 300 {
		stream := info.StreamStatus.OutcomeSnapshot()
		if stream.ErrorStatus >= 400 {
			httpResult.status = stream.ErrorStatus
			kind = classifyChannelAlertHTTP(stream.ErrorStatus, false)
		} else if stream.EndReason == relaycommon.StreamEndReasonTimeout || stream.EndReason == relaycommon.StreamEndReasonScannerErr {
			kind = channelAlertNetwork
		}
	}
	if success && httpResult.status > 0 || kind != channelAlertSuccess {
		recordChannelAlert(scope, id, kind, httpResult.status)
	}
}

func ObserveChannelRelayResult(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) {
	if !channelAlerts.enabled.Load() {
		return
	}
	stream := info.StreamStatus
	success := apiErr == nil && !info.PerformanceBusinessRejection && (stream == nil || stream.IsNormalEnd() && !stream.HasErrors() && (stream.ResponseOutcome() == "" || stream.ResponseOutcome() == "completed"))
	recordChannelAlertAttempt(c, info, channelAlertOperation(info.RelayMode), c.GetString(common.RequestIdKey), success, apiErr.GetErrorCode())
}

func ChannelAlertSocketObservationID() string {
	if channelAlerts.enabled.Load() {
		return common.NewRequestId()
	}
	return ""
}

func ObserveChannelResponsesSocket(c *gin.Context, info *relaycommon.RelayInfo, id string, completed bool) {
	if channelAlerts.enabled.Load() && info != nil {
		recordChannelAlertAttempt(c, info, "responses_websocket", id, completed, "")
	}
}

func ObserveChannelResponsesSocketTerminal(c *gin.Context, info *relaycommon.RelayInfo, id string, completed bool) {
	if !channelAlerts.enabled.Load() || info == nil || !completed {
		return
	}
	c.Set(channelAlertHTTPKey, channelAlertHTTP{status: http.StatusOK})
	ObserveChannelResponsesSocket(c, info, id, true)
}

func ObserveChannelTaskSubmission(c *gin.Context, info *relaycommon.RelayInfo, taskErr *dto.TaskError, accepted bool) {
	if !channelAlerts.enabled.Load() || taskErr != nil && taskErr.LocalError {
		return
	}
	var code types.ErrorCode
	if taskErr != nil {
		code = types.ErrorCode(taskErr.Code)
	}
	recordChannelAlertAttempt(c, info, channelAlertSubmitOperation(c, info), c.GetString(common.RequestIdKey), accepted, code)
}

func ObserveChannelProbe(ctx context.Context, channel *model.Channel, modelName, operation string, status int, attempted, responded, success bool, slot int) {
	if !channelAlerts.enabled.Load() || ctx.Err() != nil || (!attempted && !responded) {
		return
	}
	kind := classifyChannelAlertHTTP(status, attempted && !responded)
	if success || kind != channelAlertSuccess {
		recordChannelAlert(channelAlertScope{Channel: channel.Id, Revision: model.ChannelAlertRevision(channel), Model: modelName, Operation: operation, Slot: slot},
			common.NewRequestId(), kind, status)
	}
}

func ObserveFrozenChannelResult(ctx context.Context, task *model.Task, operation, observationID string, status int, transport, success bool) {
	if !channelAlerts.enabled.Load() || task == nil || task.PrivateData.ChannelAlert == nil || !success && ctx.Err() != nil {
		return
	}
	kind := classifyChannelAlertHTTP(status, transport)
	if !success && kind == channelAlertSuccess {
		return
	}
	snapshot := task.PrivateData.ChannelAlert
	if observationID == "" {
		observationID = common.NewRequestId()
	}
	recordChannelAlert(channelAlertScope{Channel: task.ChannelId, Revision: snapshot.Revision, Model: task.Properties.OriginModelName,
		Operation: operation, Slot: snapshot.Slot}, observationID, kind, status)
}

func ObserveChannelVideoPoll(ctx context.Context, task *model.Task, status int, err error) {
	var temporary *MiniMaxTemporaryQueryError
	var network net.Error
	transport := errors.As(err, &network) || errors.Is(err, io.ErrUnexpectedEOF)
	if errors.As(err, &temporary) {
		status = temporary.StatusCode
		transport = status == 0
	}
	if errors.Is(err, relaycommon.ErrUpstreamObservationUnavailable) || errors.Is(err, context.Canceled) {
		return
	}
	ObserveFrozenChannelResult(ctx, task, "video_query", "", status, transport, false)
}

func imageTaskAlertOperation(task *model.Task) string {
	if task.Action == model.TaskActionImageEdit {
		return "image_edit"
	}
	return "image_generation"
}

func ObserveChannelSelectionFailure(c *gin.Context, channel *model.Channel, modelName string, apiErr *types.NewAPIError) {
	if !channelAlerts.enabled.Load() || apiErr.GetErrorCode() != types.ErrorCodeChannelNoAvailableKey || len(channel.GetKeys()) != 0 {
		return
	}
	operation := channelAlertOperation(relaymode.Path2RelayMode(c.Request.URL.Path))
	if strings.HasSuffix(c.Request.URL.Path, "/contents/generations/tasks") {
		operation = "video_submit"
	}
	recordChannelAlert(channelAlertScope{Channel: channel.Id, Revision: model.ChannelAlertRevision(channel), Model: modelName, Operation: operation, Slot: -1},
		c.GetString(common.RequestIdKey), channelAlertConfig, 0)
}

func channelAlertSubmitOperation(c *gin.Context, info *relaycommon.RelayInfo) string {
	if info.RelayMode == relaymode.RelayModeVideoSubmit || info.TaskRelayInfo != nil && model.IsLinkVideoTaskClientProtocol(info.TaskRelayInfo.ClientProtocol) ||
		strings.Contains(c.Request.URL.Path, "/video") {
		return "video_submit"
	}
	return "task_submit"
}

func ObserveChannelRelayProbe(ctx context.Context, channel *model.Channel, modelName string, relayMode, status int, attempted, responded, success bool, slot int) {
	ObserveChannelProbe(ctx, channel, modelName, channelAlertOperation(relayMode), status, attempted, responded, success, slot)
}

func ObserveTrustedChannelVideoResult(ctx context.Context, task *model.Task, status int, outcome model.TaskStatus) {
	if status >= 200 && status < 300 {
		ObserveFrozenChannelResult(ctx, task, "video_query", "", status, false, true)
		if outcome == model.TaskStatusSuccess {
			ObserveFrozenChannelResult(ctx, task, "video_generation", task.TaskID, status, false, true)
		}
	}
}

type imageChannelAlertKey struct{}

func WithImageChannelAlert(ctx context.Context, task *model.Task) context.Context {
	if !channelAlerts.enabled.Load() || task.PrivateData.ChannelAlert == nil {
		return ctx
	}
	return context.WithValue(ctx, imageChannelAlertKey{}, task)
}

// ObserveImageChannelTransport counts only the actual outbound request result.
func ObserveImageChannelTransport(ctx context.Context, operation string, response *http.Response, err error) {
	task, _ := ctx.Value(imageChannelAlertKey{}).(*model.Task)
	if task == nil {
		return
	}
	if operation == "" {
		operation = imageTaskAlertOperation(task)
	}
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	id := task.TaskID
	if operation == "image_query" {
		id = ""
	}
	ObserveFrozenChannelResult(ctx, task, operation, id, status, err != nil && !errors.Is(err, context.Canceled), false)
}

func ObserveImageChannelSuccess(ctx context.Context, operation string) {
	task, _ := ctx.Value(imageChannelAlertKey{}).(*model.Task)
	if task == nil {
		return
	}
	if operation == "" {
		operation = imageTaskAlertOperation(task)
	}
	ObserveFrozenChannelResult(ctx, task, operation, task.TaskID, http.StatusOK, false, true)
}

func observeChannelBatchPoll(ctx context.Context, tasks []*model.Task, status int, err error) {
	if !channelAlerts.enabled.Load() {
		return
	}
	var network net.Error
	transport := errors.As(err, &network) || errors.Is(err, io.ErrUnexpectedEOF)
	if classifyChannelAlertHTTP(status, transport) == channelAlertSuccess {
		return
	}
	id := common.NewRequestId()
	for _, task := range tasks {
		ObserveFrozenChannelResult(ctx, task, "task_query", id, status, transport, false)
	}
}

func ObserveInlineImageChannelAlert(c *gin.Context, info *relaycommon.RelayInfo, operation string, response *http.Response, err error, success bool) {
	if !channelAlerts.enabled.Load() || c == nil || !success && c.Request.Context().Err() != nil {
		return
	}
	snapshot := ChannelAlertSelection(c)
	if snapshot == nil || info.ChannelMeta == nil {
		return
	}
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	kind := classifyChannelAlertHTTP(status, err != nil && response == nil && !errors.Is(err, context.Canceled))
	if success || kind != channelAlertSuccess {
		recordChannelAlert(channelAlertScope{Channel: info.ChannelId, Revision: snapshot.Revision, Model: info.OriginModelName,
			Operation: operation, Slot: snapshot.Slot}, c.GetString(common.RequestIdKey), kind, status)
	}
}

// SDK callers expose status errors without returning an http.Response.
func RecordChannelAlertSDKResult(c *gin.Context, err error) {
	if !channelAlerts.enabled.Load() {
		return
	}
	if err == nil {
		c.Set(channelAlertHTTPKey, channelAlertHTTP{status: http.StatusOK})
		return
	}
	var response interface{ HTTPStatusCode() int }
	if errors.As(err, &response) {
		c.Set(channelAlertHTTPKey, channelAlertHTTP{status: response.HTTPStatusCode()})
		return
	}
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded) {
		c.Set(channelAlertHTTPKey, channelAlertHTTP{transport: true})
	}
}
