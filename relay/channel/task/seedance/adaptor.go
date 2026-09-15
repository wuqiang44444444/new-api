// Package seedance owns the Seedance Link task channel boundary.
package seedance

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type ContentItem struct {
	Type     string    `json:"type,omitempty"`
	Text     string    `json:"text,omitempty"`
	ImageURL *MediaURL `json:"image_url,omitempty"`
	VideoURL *MediaURL `json:"video_url,omitempty"`
	AudioURL *MediaURL `json:"audio_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type MediaURL struct {
	URL string `json:"url,omitempty"`
}

type requestPayload struct {
	Model                 string         `json:"model"`
	Content               []ContentItem  `json:"content,omitempty"`
	CallbackURL           string         `json:"callback_url,omitempty"`
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`
	ServiceTier           string         `json:"service_tier,omitempty"`
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"`
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`
	Draft                 *dto.BoolValue `json:"draft,omitempty"`
	Tools                 []struct {
		Type string `json:"type,omitempty"`
	} `json:"tools,omitempty"`
	SafetyIdentifier string         `json:"safety_identifier,omitempty"`
	Priority         *dto.IntValue  `json:"priority,omitempty"`
	Resolution       string         `json:"resolution,omitempty"`
	Ratio            string         `json:"ratio,omitempty"`
	OutputFormat     *string        `json:"output_format,omitempty"`
	Duration         *dto.IntValue  `json:"duration,omitempty"`
	Frames           *dto.IntValue  `json:"frames,omitempty"`
	Seed             *dto.IntValue  `json:"seed,omitempty"`
	CameraFixed      *dto.BoolValue `json:"camera_fixed,omitempty"`
	Watermark        *dto.BoolValue `json:"watermark,omitempty"`
}

type responsePayload struct {
	ID string `json:"id"`
}

type responseTask struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Seed            int    `json:"seed"`
	Resolution      string `json:"resolution"`
	Duration        int    `json:"duration"`
	Ratio           string `json:"ratio"`
	FramesPerSecond int    `json:"framespersecond"`
	ServiceTier     string `json:"service_tier"`
	Tools           []struct {
		Type string `json:"type"`
	} `json:"tools"`
	Usage *struct {
		CompletionTokens *int `json:"completion_tokens"`
		TotalTokens      *int `json:"total_tokens"`
		ToolUsage        struct {
			WebSearch int `json:"web_search"`
		} `json:"tool_usage"`
	} `json:"usage"`
	// usage_source/usage_evidence 由第三方归一层写入，描述计费用量的形成字段与全部采集证据。
	UsageSource   string         `json:"usage_source,omitempty"`
	UsageEvidence map[string]int `json:"usage_evidence,omitempty"`
	Error         struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	ProviderBillingEvidence *relaycommon.ProviderBillingEvidence `json:"_provider_billing_evidence,omitempty"`
	CreatedAt               int64                                `json:"created_at"`
	UpdatedAt               int64                                `json:"updated_at"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
	protocol    dto.VideoUpstreamProtocol
	profile     dto.VideoUpstreamProfile
	createPath  string
	// pluginCreate caches the extension-plugin conversion for the current
	// submission: the billing probe and the request body derive from one
	// deterministic buildCreate result.
	pluginCreate *seedancePluginCreateConversion
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	protocol := info.ChannelOtherSettings.VideoUpstreamProtocol
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
	a.protocol = protocol
	a.profile = protocol.TransportProfile()
	a.createPath, info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate = protocol.TransportPaths(info.UpstreamModelName)
	info.ChannelOtherSettings.VideoUpstreamProfile = a.profile
	info.ChannelOtherSettings.VideoUpstreamCreatePath = a.createPath
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return service.TaskErrorWrapperLocal(
			stderrors.New("Seedance channels require the ModelArk V3 request contract"),
			"invalid_video_contract",
			http.StatusBadRequest,
		)
	}
	if err := dto.ValidateVideoUpstreamProtocol(a.protocol); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_video_protocol", http.StatusBadRequest)
	}
	if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionImageToVideo); taskErr != nil {
		return taskErr
	}
	payload, typed, err := a.modelArkContractPayload(c)
	if err != nil || !typed {
		if err == nil {
			err = stderrors.New("Seedance requires the ModelArk V3 request contract")
		}
		return service.TaskErrorWrapperLocal(err, "invalid_video_contract", http.StatusBadRequest)
	}
	info.Action = modelArkTaskAction(payload)
	if taskErr := service.ValidateFunCloudHostedVideoMedia(c, info); taskErr != nil {
		return taskErr
	}
	if (a.protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 || a.protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 ||
		a.protocol == dto.VideoUpstreamProtocolModelArkV3CMCC) &&
		billing_setting.GetBillingMode(info.OriginModelName) != billing_setting.BillingModeTieredExpr {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("customer model %s requires tiered_expr billing", info.OriginModelName),
			"model_price_error",
			http.StatusBadRequest,
		)
	}
	return applyVideoServiceTierPolicy(c, info, a.profile)
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	if a.pluginCreate != nil && a.createPath != "" {
		return joinVideoUpstreamURL(a.baseURL, a.createPath), nil
	}
	path, err := videoCreatePath(a.profile, a.createPath)
	if err != nil {
		return "", err
	}
	return joinVideoUpstreamURL(a.baseURL, path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return a.applyCMCCRequestHeaders(c, req)
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if a.protocol == dto.VideoUpstreamProtocolModelArkV3CMCC {
		return nil
	}
	if a.profile == dto.VideoUpstreamProfileThirdPartySynlinkVideoV1 || a.profile == dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3 {
		return nil
	}
	if a.profile == dto.VideoUpstreamProfileThirdPartyRelay &&
		providerModelFromRelayInfo(info, info.OriginModelName) == modelSeedance20 {
		return nil
	}
	payload, typed, err := a.modelArkContractPayload(c)
	if err != nil || !typed {
		return nil
	}
	hasVideo := false
	for _, item := range payload.Content {
		if item.Type == "video_url" && item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
			hasVideo = true
			break
		}
	}
	ratio, ok := GetVideoInputRatio(providerModelFromRelayInfo(info, info.OriginModelName), strings.TrimSpace(payload.Resolution), hasVideo)
	if !ok || ratio == 1.0 {
		return nil
	}
	return map[string]float64{"video_input": ratio}
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if data, handled, err := a.buildSeedancePluginCreateRequestBody(c, info, a.profile); handled {
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	return nil, fmt.Errorf("video protocol is retired or not registered for new requests")
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// ParseResponse 只解析上游创建响应，不写客户端响应；展示由控制器负责。
func (a *TaskAdaptor) ParseResponse(c *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (*channel.TaskSubmitResponse, *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	responseBody, err = normalizeSeedanceVideoCreateResponse(c, a, responseBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "normalize_response_body_failed", http.StatusBadGateway)
	}
	var providerResponse responsePayload
	if err := common.Unmarshal(responseBody, &providerResponse); err != nil {
		return nil, service.TaskErrorWrapper(errors.Wrap(err, "decode Seedance create response"), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(providerResponse.ID) == "" {
		return nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	// Seedance 是纯异步创建：没有立即完成的 TaskInfo，也没有跨轮次插件状态。
	return &channel.TaskSubmitResponse{
		UpstreamTaskID: providerResponse.ID,
		TaskData:       responseBody,
		ClientResponse: json.RawMessage(responseBody),
	}, nil
}

// FetchTask 从任务快照读取查询输入。baseUrl 由调用方传入；上游任务 ID、协议
// profile、adapter 版本与查询路径模板都只读创建时冻结的 PrivateData 快照，
// 缺失时失败关闭，不回退到当前渠道配置。
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
	profile, err := videoUpstreamProfileFromTask(task)
	if err != nil {
		return nil, err
	}
	adapterVersion, err := videoAdapterVersionFromTask(task, a.ChannelType, profile)
	if err != nil {
		return nil, err
	}
	path, err := seedanceTaskPath(task, profile, taskID, baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinVideoUpstreamURL(baseURL, path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	resp, err := client.Do(req)
	service.AttachRawTaskPollingEvidence(task, req, resp, err)
	// Synlink has no verified terminal contract for non-2xx queries. Do not let a proxy's
	// 404/410 become a definitive missing-task result in the shared poller.
	if err == nil && resp != nil && profile == dto.VideoUpstreamProfileThirdPartySynlinkVideoV1 && (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) {
		_, _ = io.Copy(io.Discard, resp.Body) // Preserve the existing protected response evidence tee.
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "Upstream task query returned HTTP 404; the result is unconfirmed"}
		}
		return nil, &relaycommon.UpstreamContractViolation{Reason: fmt.Sprintf("unverified Synlink query HTTP status %d", resp.StatusCode)}
	}
	if err != nil || resp == nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp, err
	}
	responseBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read upstream task response: %w", err)
	}
	if profile.IsOfficial() && seedancePluginTaskSnapshot(task) == nil {
		responseBody, err = normalizeOfficialTaskUsage(responseBody, taskID)
	} else {
		responseBody, err = normalizeSeedanceVideoTaskResponse(
			ctx,
			task,
			profile,
			adapterVersion,
			responseBody,
			taskID,
			baseURL,
			frozenVideoBillingContext(task),
		)
	}
	if err != nil {
		return nil, err
	}
	if profile == dto.VideoUpstreamProfileThirdPartySynlinkVideoV1 {
		responseBody, err = attachSynlinkLastFrame(task, responseBody)
		if err != nil {
			return nil, err
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	resp.ContentLength = int64(len(responseBody))
	return resp, nil
}

func (*TaskAdaptor) GetModelList() []string { return ModelList }

func (*TaskAdaptor) GetChannelName() string { return ChannelName }

// ParseTaskResult 按统一轮询合同解析 Provider 任务响应；task 与 resp 由接口
// 约定保留，当前协议解析不需要它们。
func (*TaskAdaptor) ParseTaskResult(_ *model.Task, _ *http.Response, respBody []byte) (*relaycommon.TaskInfo, error) {
	providerTask := responseTask{}
	if err := common.Unmarshal(respBody, &providerTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal Seedance task result failed")
	}
	result := relaycommon.TaskInfo{Code: 0}
	switch providerTask.Status {
	case "pending", "queued":
		result.Status = model.TaskStatusQueued
		result.Progress = "10%"
	case "processing", "running":
		result.Status = model.TaskStatusInProgress
		result.Progress = "50%"
	case "succeeded":
		result.Status = model.TaskStatusSuccess
		result.Progress = "100%"
		result.Url = providerTask.Content.VideoURL
		if providerTask.Usage != nil {
			if providerTask.Usage.CompletionTokens != nil {
				result.CompletionTokens = *providerTask.Usage.CompletionTokens
				result.UsageReported = true
				result.CompletionTokensReported = true
			}
			if providerTask.Usage.TotalTokens != nil {
				result.TotalTokens = *providerTask.Usage.TotalTokens
				result.UsageReported = true
			}
		}
		result.UsageSource = providerTask.UsageSource
		result.UsageEvidence = providerTask.UsageEvidence
		result.ProviderBillingEvidence = providerTask.ProviderBillingEvidence
	case "failed":
		result.Status = model.TaskStatusFailure
		result.Progress = "100%"
		result.Reason = providerTask.Error.Message
	case "cancelled":
		result.Status = model.TaskStatusCancelled
		result.Progress = "100%"
	case "expired":
		result.Status = model.TaskStatusExpired
		result.Progress = "100%"
	default:
		return nil, fmt.Errorf("unknown video task status %q", providerTask.Status)
	}
	return &result, nil
}

func (*TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var providerTask responseTask
	if err := common.Unmarshal(originTask.Data, &providerTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal Seedance task data failed")
	}
	video := dto.NewOpenAIVideo()
	video.ID = originTask.TaskID
	video.TaskID = originTask.TaskID
	video.Status = originTask.Status.ToVideoStatus()
	video.SetProgressStr(originTask.Progress)
	video.SetMetadata("url", providerTask.Content.VideoURL)
	video.CreatedAt = originTask.CreatedAt
	video.CompletedAt = originTask.UpdatedAt
	video.Model = originTask.Properties.OriginModelName
	if originTask.Status == model.TaskStatusFailure {
		failure := originTask.PublicVideoFailure()
		video.Error = &dto.OpenAIVideoError{Message: failure.Message, Code: failure.Code}
	}
	return common.Marshal(video)
}

func (*TaskAdaptor) CapturesRawPollingEvidence() {}
