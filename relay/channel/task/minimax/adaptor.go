package minimax

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
)

// TaskAdaptorForPlatform returns the typed adaptor when the platform names
// the MiniMax Link channel type. The typed channel is code-registered and is
// never served by the js task-plugin registry.
func TaskAdaptorForPlatform(platform constant.TaskPlatform) channel.TaskAdaptor {
	if platform != Platform {
		return nil
	}
	return &TaskAdaptor{}
}

// TaskAdaptor is the host adaptor of the MiniMax Link typed video channel.
// The JD Cloud conversion lives in the versioned minimax-link artifact; this
// adaptor owns typed admission, frozen execution, lifecycle capabilities,
// billing probe facts and content source resolution.
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType  int
	apiKey       string
	baseURL      string
	protocol     dto.VideoUpstreamProtocol
	createPath   string
	pluginCreate *minimaxCreateConversion
}

// Init reads the frozen-at-request channel identity. The JD paths are
// code-registered; they are written back onto the channel meta so the task
// snapshot freezes them.
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
	a.protocol = info.ChannelOtherSettings.VideoUpstreamProtocol
	a.createPath = jdCloudCreatePath
	info.ChannelOtherSettings.VideoUpstreamProfile = ""
	info.ChannelOtherSettings.VideoUpstreamCreatePath = a.createPath
	info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate = jdCloudQueryPathTemplate
}

func (*TaskAdaptor) GetModelList() []string {
	return []string{"MiniMax-H3"}
}

func (*TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ValidateRequestAndSetAction validates the northbound structure and the
// declared open set before anything downstream can move funds or call the
// provider. Explicit out-of-scope values fail
// here with 400 invalid_request: no attempt, no hold, no provider call.
func (a *TaskAdaptor) ValidateRequestAndSetAction(requestContext *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	contract, ok := relaycommon.GetVideoContractRequest(requestContext)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return service.TaskErrorWrapperLocal(
			errors.New("MiniMax Link channels require the ModelArk V3 request contract"),
			"invalid_video_contract",
			http.StatusBadRequest,
		)
	}
	if a.protocol != VideoUpstreamProtocolJdCloudTaskV1 {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("unsupported MiniMax Link video upstream protocol %q", a.protocol),
			"invalid_video_protocol",
			http.StatusBadRequest,
		)
	}
	if taskErr := relaycommon.ValidateBasicTaskRequest(requestContext, info, constant.TaskActionTextToVideo); taskErr != nil {
		return taskErr
	}
	if billing_setting.GetBillingMode(info.OriginModelName) != billing_setting.BillingModeTieredExpr {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("customer model %s requires tiered_expr billing", info.OriginModelName),
			"model_price_error",
			http.StatusBadRequest,
		)
	}
	info.Action = constant.TaskActionTextToVideo
	return nil
}

// ValidateMappedRequest runs the artifact conversion once, before pricing and
// before the durable attempt. Declared-scope violations surface as HTTP 400
// invalid_request here; nothing has attempted, held funds or called JD.
func (a *TaskAdaptor) ValidateMappedRequest(requestContext *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	if taskErr := service.PrepareVideoReferenceAudio(requestContext, info); taskErr != nil {
		return taskErr
	}
	if _, err := a.ensureCreateConversion(requestContext, info); err != nil {
		if contractErr, ok := relaycommon.AsVideoContractError(err); ok {
			return service.TaskErrorWrapperLocal(contractErr, contractErr.Code, http.StatusBadRequest)
		}
		return service.TaskErrorWrapperLocal(err, "minimax_plugin_unavailable", http.StatusServiceUnavailable)
	}
	return nil
}

// BuildTaskBillingProbe produces the host-owned billing facts for the frozen
// expression. Values come from the northbound contract and the pinned
// declaration defaults only; the artifact contributes no probe fields, so no
// plugin output can rewrite charged amounts.
func (a *TaskAdaptor) BuildTaskBillingProbe(requestContext *gin.Context, info *relaycommon.RelayInfo) (map[string]any, error) {
	if _, err := a.ensureCreateConversion(requestContext, info); err != nil {
		return nil, err
	}
	configuration, err := pinnedConfiguration(requestContext)
	if err != nil {
		return nil, err
	}
	contract, ok := relaycommon.GetVideoContractRequest(requestContext)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return nil, fmt.Errorf("the minimax-link extension requires a ModelArk V3 request contract")
	}
	upstreamModel := providerModelFromRelayInfo(info, contract.ModelArk.Model)
	metadata, err := declaredMetadata(configuration, upstreamModel)
	if err != nil {
		return nil, err
	}
	duration, err := effectiveRequestDuration(contract.ModelArk, metadata)
	if err != nil {
		return nil, err
	}
	hasVideoInput := false
	for _, item := range contract.ModelArk.Content {
		if item.Type == "video_url" {
			hasVideoInput = true
			break
		}
	}
	return map[string]any{
		"duration_seconds": duration,
		"resolution":       declaredProbeEnum(metadata, contract.ModelArk.Resolution, metadataResolutionEnums),
		"ratio":            declaredProbeEnum(metadata, contract.ModelArk.Ratio, metadataRatioEnums),
		"has_video_input":  hasVideoInput,
	}, nil
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return joinProviderURL(a.baseURL, a.createPath), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(requestContext *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	conversion, err := a.ensureCreateConversion(requestContext, info)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(conversion.body), nil
}

func (a *TaskAdaptor) DoRequest(requestContext *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, requestContext, info, requestBody)
}

// ParseResponse parses the upstream create response through the pinned
// artifact and validates the registered acceptance contract. Only a trusted
// upstream task ID with the registered acceptance state admits a platform
// Task; anything else is a plain error that the submit flow routes to the
// unknown/ambiguous disposition.
func (a *TaskAdaptor) ParseResponse(requestContext *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	plugin := a.pinnedCreatePlugin()
	if plugin == nil {
		return nil, service.TaskErrorWrapperLocal(errors.New("the minimax-link extension plugin is unavailable"), "minimax_plugin_unavailable", http.StatusServiceUnavailable)
	}
	result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
		requestContextFrom(requestContext), createAdmissionTimeout,
		hookRoot, []string{protocolName(), "parseCreateResponse"},
		map[string]any{"body": string(responseBody)},
	)
	if callErr != nil {
		return nil, service.TaskErrorWrapper(errors.New(hookErrorMessage(callErr)), "normalize_response_body_failed", http.StatusBadGateway)
	}
	providerID, err := decodeCreateResponse(result)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "invalid_response", http.StatusBadGateway)
	}
	normalized, err := common.Marshal(map[string]any{"id": providerID})
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "normalize_response_body_failed", http.StatusInternalServerError)
	}
	return &channel.TaskSubmitResponse{
		UpstreamTaskID: providerID,
		TaskData:       normalized,
		ClientResponse: normalized,
	}, nil
}

func (a *TaskAdaptor) pinnedCreatePlugin() *pluginruntime.LoadedPlugin {
	if a.pluginCreate == nil {
		return nil
	}
	return a.pluginCreate.plugin
}

// FetchTask queries the provider with the frozen connection only: the
// upstream task id, query path, base URL, key and proxy all come from the
// creation-time snapshot. Non-2xx responses become untrusted observations
// (reconciliation), never a missing task or a refund: a 404 cannot prove a
// generation failed.
func (a *TaskAdaptor) FetchTask(baseURL, key string, task *model.Task, proxy string) (*http.Response, error) {
	return a.FetchTaskWithContext(context.Background(), baseURL, key, task, proxy)
}

func (a *TaskAdaptor) FetchTaskWithContext(ctx context.Context, baseURL, key string, task *model.Task, proxy string) (*http.Response, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	taskID := task.GetUpstreamTaskID()
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	if strings.TrimSpace(task.PrivateData.Key) != "" {
		key = task.PrivateData.Key
	}
	baseURL = task.PrivateData.VideoUpstreamQueryBaseURL
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("frozen upstream query base URL is unavailable")
	}
	pathTemplate := task.PrivateData.VideoUpstreamQueryPathTemplate
	if strings.TrimSpace(pathTemplate) == "" {
		pathTemplate = jdCloudQueryPathTemplate
	}
	path := strings.Replace(pathTemplate, "{task_id}", url.PathEscape(taskID), 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinProviderURL(baseURL, path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(task.PrivateData.VideoUpstreamProxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(req)
	service.AttachRawTaskPollingEvidence(task, req, resp, err)
	if err != nil || resp == nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return nil, &relaycommon.UpstreamContractViolation{
				Reason: fmt.Sprintf("unverified MiniMax query HTTP status %d", resp.StatusCode),
			}
		}
		return nil, err
	}
	responseBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read upstream task response: %w", err)
	}
	observation, normalized, err := normalizeTaskObservation(ctx, task, responseBody, taskID)
	if err != nil {
		return nil, err
	}
	_ = observation
	resp.Body = io.NopCloser(bytes.NewReader(normalized))
	resp.ContentLength = int64(len(normalized))
	return resp, nil
}

// observationBody is the normalized observation the host stores and parses.
// Its fields mirror the shared typed-video observation contract; usage
// evidence fields are stripped from persistence by the shared redaction.
type observationBody struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	UsageSource   string         `json:"usage_source,omitempty"`
	UsageEvidence map[string]int `json:"usage_evidence,omitempty"`
}

// ParseTaskResult maps the validated observation onto the shared polling
// contract. A success observation carries no upstream URL on purpose: the
// platform content address is the projection, and fresh CDN URLs are
// resolved on demand by the frozen content source. An already-successful
// task is monotonic: only a consistent success observation is accepted, so
// one malformed later poll can never flip accepted facts or funds.
func (a *TaskAdaptor) ParseTaskResult(task *model.Task, _ *http.Response, respBody []byte) (*relaycommon.TaskInfo, error) {
	var observation observationBody
	if err := common.Unmarshal(respBody, &observation); err != nil {
		return nil, fmt.Errorf("unmarshal MiniMax task result failed: %w", err)
	}
	if task != nil && task.Status == model.TaskStatusSuccess && observation.Status != observationStatusSucceeded {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "successful task received a conflicting observation"}
	}
	result := relaycommon.TaskInfo{Code: 0}
	switch observation.Status {
	case observationStatusQueued:
		result.Status = model.TaskStatusQueued
		result.Progress = "10%"
	case observationStatusRunning:
		result.Status = model.TaskStatusInProgress
		result.Progress = "50%"
	case observationStatusSucceeded:
		result.Status = model.TaskStatusSuccess
		result.Progress = "100%"
		result.UsageSource = observation.UsageSource
		result.UsageEvidence = observation.UsageEvidence
	case observationStatusFailed:
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = observation.Error.Message
	case observationStatusCancelled:
		result.Status = model.TaskStatusCancelled
		result.Progress = "100%"
	default:
		return nil, fmt.Errorf("unknown video task status %q", observation.Status)
	}
	return &result, nil
}

// TaskLifecycleCapabilities keeps both lifecycle support bits closed for the
// first phase: no queued cancellation and no terminal deletion until their
// acceptance criteria pass. Content delivery stays supported.
func (*TaskAdaptor) TaskLifecycleCapabilities(_ *model.Task) channel.TaskLifecycleCapabilities {
	return channel.TaskLifecycleCapabilities{SupportsContent: true}
}

// ConvertToOpenAIVideo projects the frozen observation onto the OpenAI video
// shape used by the shared task surface.
func (*TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var observation observationBody
	if err := common.Unmarshal(originTask.Data, &observation); err != nil {
		return nil, fmt.Errorf("unmarshal MiniMax task data failed: %w", err)
	}
	video := dto.NewOpenAIVideo()
	video.ID = originTask.TaskID
	video.TaskID = originTask.TaskID
	video.Status = originTask.Status.ToVideoStatus()
	video.SetProgressStr(originTask.Progress)
	video.CreatedAt = originTask.CreatedAt
	video.CompletedAt = originTask.UpdatedAt
	video.Model = originTask.Properties.OriginModelName
	if originTask.Status == model.TaskStatusFailure && observation.Error != nil {
		video.Error = &dto.OpenAIVideoError{Message: observation.Error.Message, Code: observation.Error.Code}
	}
	return common.Marshal(video)
}

// CapturesRawPollingEvidence marks the adaptor as persisting its own polling
// evidence through the shared evidence tee.
func (*TaskAdaptor) CapturesRawPollingEvidence() {}
