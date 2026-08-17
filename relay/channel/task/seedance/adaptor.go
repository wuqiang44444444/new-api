// Package seedance owns the Seedance Link task channel boundary.
package seedance

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
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
	if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); taskErr != nil {
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
	if a.protocol == dto.VideoUpstreamProtocolFunCloudSeedance &&
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
	path, err := videoCreatePath(a.profile, a.createPath)
	if err != nil {
		return "", err
	}
	return joinVideoUpstreamURL(a.baseURL, path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if a.profile == dto.VideoUpstreamProfileThirdPartyFunCloudSeedance {
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
	if data, handled, err := buildFunCloudVideoCreateRequest(c, info, a.profile); handled {
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	if data, handled, err := buildFeicaiVideoCreateRequest(c, info, a.profile); handled {
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	body, typed, err := a.modelArkContractPayload(c)
	if err != nil {
		return nil, errors.Wrap(err, "convert Seedance request payload failed")
	}
	if !typed {
		return nil, relaycommon.NewVideoContractError("invalid_video_contract", "Seedance requires the ModelArk V3 request contract")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	if _, defaultGenerateAudio, ok := providerBillingDefaults(a.protocol, body.Model); ok && defaultGenerateAudio && body.GenerateAudio == nil {
		value := dto.BoolValue(true)
		body.GenerateAudio = &value
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	switch a.protocol {
	case dto.VideoUpstreamProtocolMoxingMediaTaskV1:
		data, err = thirdparty.MoxingMediaCreateRequest(data)
	case dto.VideoUpstreamProtocolMoxingModelArkV1:
		// The Moxing ModelArk protocol consumes the typed payload directly.
	default:
		data, err = convertVideoCreateRequest(a.profile, data)
	}
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *taskdto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	responseBody, err = normalizeVideoCreateResponse(a.profile, responseBody)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "normalize_response_body_failed", http.StatusBadGateway)
	}
	var providerResponse responsePayload
	if err := common.Unmarshal(responseBody, &providerResponse); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrap(err, "decode Seedance create response"), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(providerResponse.ID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.CreatedAt = time.Now().Unix()
	video.Model = info.OriginModelName
	c.JSON(http.StatusOK, video)
	return providerResponse.ID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	profile, err := videoProfileFromFetchBody(body)
	if err != nil {
		return nil, err
	}
	adapterVersion, err := videoAdapterVersionFromFetchBody(body, a.ChannelType, profile)
	if err != nil {
		return nil, err
	}
	path, err := videoTaskPath(profile, videoQueryTemplateFromFetchBody(body), taskID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, joinVideoUpstreamURL(baseURL, path), nil)
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
	if err != nil || resp == nil || profile.IsOfficial() || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp, err
	}
	responseBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read upstream task response: %w", err)
	}
	responseBody, err = normalizeVideoTaskResponse(
		profile,
		adapterVersion,
		responseBody,
		taskID,
		baseURL,
		body,
	)
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	resp.ContentLength = int64(len(responseBody))
	return resp, nil
}

func (*TaskAdaptor) GetModelList() []string { return ModelList }

func (*TaskAdaptor) GetChannelName() string { return ChannelName }

func (*TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
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
	if providerTask.Status == "failed" {
		video.Error = &dto.OpenAIVideoError{Message: providerTask.Error.Message, Code: providerTask.Error.Code}
	}
	return common.Marshal(video)
}
