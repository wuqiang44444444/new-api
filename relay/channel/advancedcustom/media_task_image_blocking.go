package advancedcustom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	mediaimageprotocol "github.com/QuantumNous/new-api/relay/mediaimage"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	mediaTaskImageCapability       = "image_generation"
	mediaTaskImageQueryPath        = "/v1/media/tasks/{task_id}"
	mediaTaskImageDefaultTimeout   = 5 * time.Minute
	mediaTaskImageMaxResponseBytes = 1 << 20
	mediaTaskImageMaxPromptRunes   = 3000
	mediaTaskImageMaxInputImages   = 14
	mediaTaskImageMaxGeminiImages  = 10
)

type mediaTaskImageRequest struct {
	Model           string               `json:"model"`
	Capability      string               `json:"capability"`
	Prompt          string               `json:"prompt"`
	N               *uint                `json:"n,omitempty"`
	Size            string               `json:"size"`
	ResponseFormat  string               `json:"response_format,omitempty"`
	Image           json.RawMessage      `json:"image,omitempty"`
	ReferenceImages []string             `json:"reference_images,omitempty"`
	AspectRatio     string               `json:"aspect_ratio,omitempty"`
	Extra           *mediaTaskImageExtra `json:"extra,omitempty"`
}

type mediaTaskImageExtra struct {
	SequentialImageGeneration        string                               `json:"sequential_image_generation,omitempty"`
	SequentialImageGenerationOptions *mediaTaskImageSequentialImageOption `json:"sequential_image_generation_options,omitempty"`
	Watermark                        *bool                                `json:"watermark,omitempty"`
}

type mediaTaskImageSequentialImageOption struct {
	MaxImages *uint `json:"max_images,omitempty"`
}

func convertMediaTaskImageRequest(request dto.ImageRequest, originModel string) (*mediaTaskImageRequest, error) {
	upstreamModel := strings.TrimSpace(request.Model)
	if upstreamModel == "" {
		return nil, mediaTaskImageInvalidRequest(errors.New("model is required"))
	}
	if isFixedPriceMediaTaskImageModel(originModel) || isFixedPriceMediaTaskImageModel(upstreamModel) {
		return nil, mediaTaskImageInvalidRequest(errors.New("this image model variant is not supported because fixed per-size pricing is disabled"))
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, mediaTaskImageInvalidRequest(errors.New("prompt is required"))
	}
	publicCapability, registeredPublicModel := model.ResolveImageSKUCapability(originModel)
	if registeredPublicModel {
		publicRequest := request
		publicRequest.Model = publicCapability.PublicModel
		if err := publicCapability.ValidateRequest(&publicRequest); err != nil {
			return nil, mediaTaskImageInvalidRequest(err)
		}
	} else {
		if len([]rune(prompt)) > mediaTaskImageMaxPromptRunes {
			return nil, mediaTaskImageInvalidRequest(fmt.Errorf("prompt must not exceed %d characters", mediaTaskImageMaxPromptRunes))
		}
		if request.Stream != nil && *request.Stream {
			return nil, mediaTaskImageInvalidRequest(errors.New("streaming image responses are not supported by this image model"))
		}
		if err := rejectUnsupportedMediaTaskImageFields(request); err != nil {
			return nil, mediaTaskImageInvalidRequest(err)
		}
	}

	imageCount := uint(1)
	if request.N != nil {
		imageCount = *request.N
	}
	if !registeredPublicModel && (imageCount == 0 || imageCount > dto.MaxImageN) {
		return nil, mediaTaskImageInvalidRequest(fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN))
	}

	responseFormat := strings.TrimSpace(request.ResponseFormat)
	if responseFormat == "" {
		responseFormat = "url"
	}
	if !registeredPublicModel && responseFormat != "url" {
		return nil, mediaTaskImageInvalidRequest(errors.New("response_format must be url"))
	}

	extraFields, err := parseMediaTaskImageExtraFields(request.Extra)
	if err != nil {
		return nil, mediaTaskImageInvalidRequest(err)
	}
	if extraFields.capability != "" && extraFields.capability != mediaTaskImageCapability {
		return nil, mediaTaskImageInvalidRequest(errors.New("capability must be image_generation"))
	}

	output := &mediaTaskImageRequest{
		Model:           upstreamModel,
		Capability:      mediaTaskImageCapability,
		Prompt:          prompt,
		N:               &imageCount,
		Size:            strings.TrimSpace(request.Size),
		ResponseFormat:  responseFormat,
		ReferenceImages: extraFields.referenceImages,
		AspectRatio:     extraFields.aspectRatio,
		Extra:           extraFields.extra,
	}

	if len(request.Image) > 0 && len(request.Images) > 0 {
		return nil, mediaTaskImageInvalidRequest(errors.New("image and images cannot be used together"))
	}
	if len(request.Image) > 0 {
		output.Image = append(json.RawMessage(nil), request.Image...)
	}
	if len(request.Images) > 0 {
		output.Image = append(json.RawMessage(nil), request.Images...)
	}

	validationModel := mediaTaskImageValidationModel(originModel, output.Model)
	northboundReferenceField := "reference_images"
	if strings.EqualFold(validationModel, "gemini-3.1-flash-image-preview-usage") &&
		len(output.Image) > 0 && !bytes.Equal(bytes.TrimSpace(output.Image), []byte("null")) {
		if len(output.ReferenceImages) > 0 {
			return nil, mediaTaskImageInvalidRequest(errors.New("image and reference_images cannot be used together"))
		}

		var referenceImages []string
		var singleReferenceImage string
		if err := common.Unmarshal(output.Image, &singleReferenceImage); err == nil {
			referenceImages = []string{singleReferenceImage}
		} else if err := common.Unmarshal(output.Image, &referenceImages); err != nil {
			return nil, mediaTaskImageInvalidRequest(errors.New("image must be an HTTP(S) URL or an array of HTTP(S) URLs"))
		}
		if !registeredPublicModel && len(referenceImages) > mediaTaskImageMaxGeminiImages {
			return nil, mediaTaskImageInvalidRequest(fmt.Errorf("image must not contain more than %d images", mediaTaskImageMaxGeminiImages))
		}
		if !registeredPublicModel {
			if err := validateMediaTaskImageURLs(referenceImages, "image"); err != nil {
				return nil, mediaTaskImageInvalidRequest(err)
			}
		}
		output.ReferenceImages = referenceImages
		output.Image = nil
		northboundReferenceField = "image"
	}

	if request.Watermark != nil {
		if output.Extra == nil {
			output.Extra = &mediaTaskImageExtra{}
		}
		if output.Extra.Watermark != nil && *output.Extra.Watermark != *request.Watermark {
			return nil, mediaTaskImageInvalidRequest(errors.New("watermark conflicts with extra.watermark"))
		}
		output.Extra.Watermark = request.Watermark
	}

	if !registeredPublicModel {
		if err := validateMediaTaskImageModel(output, validationModel, northboundReferenceField); err != nil {
			return nil, mediaTaskImageInvalidRequest(err)
		}
	}
	if output.Extra != nil && output.Extra.SequentialImageGeneration == "auto" {
		maxImages := output.Extra.SequentialImageGenerationOptions.MaxImages
		if maxImages == nil || *maxImages == 0 || *maxImages > dto.MaxImageN {
			return nil, mediaTaskImageInvalidRequest(fmt.Errorf("extra.sequential_image_generation_options.max_images must be between 1 and %d", dto.MaxImageN))
		}
		if *maxImages > imageCount {
			return nil, mediaTaskImageInvalidRequest(errors.New("n must be greater than or equal to extra.sequential_image_generation_options.max_images"))
		}
	}

	return output, nil
}

type parsedMediaTaskImageExtraFields struct {
	capability      string
	aspectRatio     string
	referenceImages []string
	extra           *mediaTaskImageExtra
}

func parseMediaTaskImageExtraFields(fields map[string]json.RawMessage) (parsedMediaTaskImageExtraFields, error) {
	var parsed parsedMediaTaskImageExtraFields
	for key, value := range fields {
		switch key {
		case "capability":
			if err := common.Unmarshal(value, &parsed.capability); err != nil {
				return parsed, errors.New("capability must be a string")
			}
			parsed.capability = strings.TrimSpace(parsed.capability)
		case "aspect_ratio":
			if err := common.Unmarshal(value, &parsed.aspectRatio); err != nil {
				return parsed, errors.New("aspect_ratio must be a string")
			}
			parsed.aspectRatio = strings.TrimSpace(parsed.aspectRatio)
		case "reference_images":
			if err := common.Unmarshal(value, &parsed.referenceImages); err != nil {
				return parsed, errors.New("reference_images must be an array of strings")
			}
		case "extra":
			extra, err := parseMediaTaskImageExtra(value)
			if err != nil {
				return parsed, err
			}
			parsed.extra = extra
		case "callback_url":
			// Deliberately omitted. This gateway owns completion and settlement.
		default:
			return parsed, fmt.Errorf("unsupported image field %q", key)
		}
	}
	return parsed, nil
}

func parseMediaTaskImageExtra(raw json.RawMessage) (*mediaTaskImageExtra, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil {
		return nil, errors.New("extra must be an object")
	}
	for key := range fields {
		switch key {
		case "sequential_image_generation", "sequential_image_generation_options", "watermark":
		default:
			return nil, fmt.Errorf("unsupported image extra field %q", key)
		}
	}

	var extra mediaTaskImageExtra
	if err := common.Unmarshal(raw, &extra); err != nil {
		return nil, errors.New("invalid image extra fields")
	}
	switch extra.SequentialImageGeneration {
	case "", "disabled":
		if extra.SequentialImageGenerationOptions != nil {
			return nil, errors.New("sequential_image_generation_options requires sequential_image_generation=auto")
		}
	case "auto":
		if extra.SequentialImageGenerationOptions == nil || extra.SequentialImageGenerationOptions.MaxImages == nil {
			return nil, errors.New("sequential_image_generation=auto requires max_images")
		}
	default:
		return nil, errors.New("sequential_image_generation must be disabled or auto")
	}
	return &extra, nil
}

func rejectUnsupportedMediaTaskImageFields(request dto.ImageRequest) error {
	switch {
	case strings.TrimSpace(request.Quality) != "":
		return errors.New("quality is not supported by this image model")
	case len(request.Style) > 0:
		return errors.New("style is not supported by this image model")
	case len(request.ExtraFields) > 0:
		return errors.New("extra_fields is not supported by this image model")
	case len(request.Background) > 0:
		return errors.New("background is not supported by this image model")
	case len(request.Moderation) > 0:
		return errors.New("moderation is not supported by this image model")
	case len(request.OutputFormat) > 0:
		return errors.New("output_format is not supported by this image model")
	case len(request.OutputCompression) > 0:
		return errors.New("output_compression is not supported by this image model")
	case len(request.PartialImages) > 0:
		return errors.New("partial_images is not supported by this image model")
	case len(request.Mask) > 0:
		return errors.New("mask is not supported by this image model")
	case len(request.InputFidelity) > 0:
		return errors.New("input_fidelity is not supported by this image model")
	case len(request.WatermarkEnabled) > 0:
		return errors.New("watermark_enabled is not supported by this image model")
	case len(request.UserId) > 0:
		return errors.New("user_id is not supported by this image model")
	}
	return nil
}

func mediaTaskImageValidationModel(originModel, upstreamModel string) string {
	if isKnownMediaTaskImageModel(originModel) {
		return originModel
	}
	if isKnownMediaTaskImageModel(upstreamModel) {
		return upstreamModel
	}
	return upstreamModel
}

func isKnownMediaTaskImageModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gemini-3.1-flash-image-preview-usage",
		"doubao-seedream-4-5-251128",
		"seedream-5-0-260128":
		return true
	default:
		return false
	}
}

func isFixedPriceMediaTaskImageModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), "gemini-3.1-flash-image-preview")
}

func validateMediaTaskImageModel(request *mediaTaskImageRequest, validationModel string, referenceField string) error {
	if request == nil {
		return errors.New("media image request is missing")
	}
	if referenceField == "" {
		referenceField = "image"
	}
	model := strings.ToLower(strings.TrimSpace(validationModel))
	switch model {
	case "gemini-3.1-flash-image-preview-usage":
		if !stringInSet(request.Size, "1K", "2K", "4K") {
			return errors.New("size must be one of 1K, 2K or 4K")
		}
		if len(request.ReferenceImages) > mediaTaskImageMaxGeminiImages {
			return fmt.Errorf("%s must not contain more than %d images", referenceField, mediaTaskImageMaxGeminiImages)
		}
		if request.AspectRatio != "" && !stringInSet(request.AspectRatio, "1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3", "21:9") {
			return errors.New("aspect_ratio is not supported")
		}
		if len(request.Image) > 0 || request.Extra != nil {
			return errors.New("extra is not supported by this image model")
		}
		return validateMediaTaskImageURLs(request.ReferenceImages, referenceField)
	case "doubao-seedream-4-5-251128":
		if request.N == nil || *request.N == 0 || *request.N > 4 {
			return errors.New("n must be between 1 and 4 for this image model")
		}
		if !stringInSet(request.Size, "2048x2048", "2304x1728", "1728x2304", "2560x1440", "1440x2560", "2496x1664", "1664x2496", "3024x1296") {
			return errors.New("size is not supported by this image model")
		}
		if len(request.ReferenceImages) > 0 || request.AspectRatio != "" || request.Extra != nil {
			return errors.New("reference_images, aspect_ratio and extra are not supported by this image model")
		}
		return validateMediaTaskImageRawURLs(request.Image, 1, "image")
	case "seedream-5-0-260128":
		if !stringInSet(request.Size, "2K", "3K") {
			return errors.New("size must be 2K or 3K for this image model")
		}
		if len(request.ReferenceImages) > 0 || request.AspectRatio != "" {
			return errors.New("reference_images and aspect_ratio are not supported by this image model")
		}
		return validateMediaTaskImageRawURLs(request.Image, mediaTaskImageMaxInputImages, "image")
	default:
		if strings.TrimSpace(request.Size) == "" {
			return errors.New("size is required")
		}
		if len(request.ReferenceImages) > mediaTaskImageMaxInputImages {
			return fmt.Errorf("reference_images must not contain more than %d images", mediaTaskImageMaxInputImages)
		}
		if err := validateMediaTaskImageURLs(request.ReferenceImages, "reference_images"); err != nil {
			return err
		}
		return validateMediaTaskImageRawURLs(request.Image, mediaTaskImageMaxInputImages, "image")
	}
}

func validateMediaTaskImageRawURLs(raw json.RawMessage, maxCount int, field string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var single string
	if err := common.Unmarshal(raw, &single); err == nil {
		return validateMediaTaskImageURLs([]string{single}, field)
	}
	var multiple []string
	if err := common.Unmarshal(raw, &multiple); err != nil {
		return fmt.Errorf("%s must be an HTTP(S) URL or an array of HTTP(S) URLs", field)
	}
	if len(multiple) > maxCount {
		return fmt.Errorf("%s must not contain more than %d images", field, maxCount)
	}
	return validateMediaTaskImageURLs(multiple, field)
}

func validateMediaTaskImageURLs(values []string, field string) error {
	for _, value := range values {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%s must contain only HTTP(S) URLs", field)
		}
	}
	return nil
}

func stringInSet(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func mediaTaskImageInvalidRequest(err error) error {
	return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

func (a *Adaptor) doMediaTaskImageBlocking(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	ctx, cancel := mediaTaskImageExecutionContext(c)
	defer cancel()
	querySpec := a.mediaTaskImageQuerySpec(info, "validation", nil)

	if !info.IsChannelTest {
		if len(info.HeadersOverride) > 0 || info.UseRuntimeHeadersOverride {
			return nil, mediaTaskImageInvalidRequest(errors.New("persistent media image tasks require route auth instead of header overrides"))
		}
		if a.route.Auth != nil {
			authType := strings.TrimSpace(a.route.Auth.Type)
			if (authType == dto.AdvancedCustomAuthTypeHeader || authType == dto.AdvancedCustomAuthTypeQuery) &&
				!strings.Contains(a.route.Auth.Value, "{api_key}") {
				return nil, mediaTaskImageInvalidRequest(errors.New("persistent media image task auth must reference {api_key}"))
			}
		}
		if _, err := mediaimageprotocol.BuildQueryURL(querySpec); err != nil {
			return nil, mediaTaskImageNoRetry(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest)
		}
		recoverySpec := service.MediaImageTaskCreateSpec{
			Protocol:            querySpec.Protocol,
			QueryBaseURL:        info.ChannelBaseUrl,
			QueryPathTemplate:   querySpec.PathTemplate,
			Proxy:               info.ChannelSetting.Proxy,
			ResponseFormat:      "url",
			RequestedImageCount: mediaTaskImageRequestedCount(info),
		}
		if a.route.Auth != nil {
			recoverySpec.AuthType = a.route.Auth.Type
			recoverySpec.AuthName = a.route.Auth.Name
			recoverySpec.AuthValueTemplate = a.route.Auth.Value
		}
		if err := service.StageMediaImageTaskCreateAttemptRecovery(c, info, recoverySpec); err != nil {
			return nil, mediaTaskImageNoRetry(
				fmt.Errorf("persist media image recovery journal: %w", err),
				types.ErrorCodeUpdateDataError,
				http.StatusInternalServerError,
			)
		}
	}
	initialURL, err := a.routeURL(info)
	if err != nil {
		return nil, err
	}
	initialResponse, err := a.doMediaTaskImageHTTP(c, ctx, info, http.MethodPost, initialURL, requestBody, info.UpstreamRequestBodySize)
	if err != nil {
		if !info.IsChannelTest {
			info.SkipRequestRefund = true
			return nil, mediaTaskImageNoRetry(errors.New("upstream media image create outcome is unknown"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
		}
		return nil, err
	}
	if initialResponse.StatusCode == http.StatusOK || initialResponse.StatusCode == http.StatusAccepted {
		outcome, err := mediaimageprotocol.InspectCreateResponse(mediaimageprotocol.ProtocolMediaImageTaskV1, initialResponse)
		if err != nil {
			_ = closeMediaTaskImageResponse(initialResponse)
			return nil, mediaTaskImageNoRetry(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
		}
		switch outcome.Disposition {
		case mediaimageprotocol.CreatePassthrough:
			if err := validateMediaTaskImageSynchronousCount(initialResponse, mediaTaskImageRequestedCount(info)); err != nil {
				return nil, mediaTaskImageNoRetry(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
			}
			return initialResponse, nil
		case mediaimageprotocol.CreateCompleted:
			_ = closeMediaTaskImageResponse(initialResponse)
			return mediaTaskImageSuccessResponse(&outcome.Result)
		case mediaimageprotocol.CreateRejected:
			_ = closeMediaTaskImageResponse(initialResponse)
			return nil, mediaTaskImageNoRetry(errors.New(outcome.Failure), types.ErrorCodeBadResponse, http.StatusBadGateway)
		case mediaimageprotocol.CreateAccepted:
			_ = closeMediaTaskImageResponse(initialResponse)
			info.SkipRequestRefund = true
			trace := &relaycommon.UpstreamTaskTrace{TaskID: outcome.TaskID, CreateRequestID: outcome.RequestID}
			relaycommon.SetUpstreamTaskTrace(c, trace)
			if outcome.RequestID != "" {
				c.Set(common.UpstreamRequestIdKey, outcome.RequestID)
			}
			return a.finishAcceptedMediaTaskImage(c, ctx, info, trace, querySpec, outcome.TaskID, outcome.RequestID)
		}
	}
	return initialResponse, nil
}

func (a *Adaptor) finishAcceptedMediaTaskImage(
	c *gin.Context,
	ctx context.Context,
	info *relaycommon.RelayInfo,
	trace *relaycommon.UpstreamTaskTrace,
	querySpec mediaimageprotocol.QuerySpec,
	taskID string,
	createRequestID string,
) (*http.Response, error) {
	querySpec.TaskID = taskID
	if !info.IsChannelTest {
		imageCount := mediaTaskImageRequestedCount(info)
		spec := service.MediaImageTaskCreateSpec{
			UpstreamTaskID:      taskID,
			CreateRequestID:     createRequestID,
			Protocol:            querySpec.Protocol,
			QueryBaseURL:        info.ChannelBaseUrl,
			QueryPathTemplate:   querySpec.PathTemplate,
			Proxy:               info.ChannelSetting.Proxy,
			ResponseFormat:      "url",
			RequestedImageCount: imageCount,
		}
		if a.route.Auth != nil {
			spec.AuthType = a.route.Auth.Type
			spec.AuthName = a.route.Auth.Name
			spec.AuthValueTemplate = a.route.Auth.Value
		}
		task, persistErr := service.PersistMediaImageTask(c, info, spec)
		if persistErr != nil {
			localTaskID := ""
			if info.TaskRelayInfo != nil {
				localTaskID = info.TaskRelayInfo.PublicTaskID
			}
			logger.LogError(c, fmt.Sprintf(
				"media image task persistence failed after upstream acceptance: channel_id=%d local_task_id=%q upstream_task_id=%q create_request_id=%q idempotency_journal=%t error=%s",
				info.ChannelId,
				localTaskID,
				taskID,
				createRequestID,
				common.GetContextKeyInt(c, constant.ContextKeyTaskIdempotencyID) != 0,
				service.SanitizeTaskErrorText(persistErr),
			))
			return nil, mediaTaskImageNoRetry(errors.New("upstream image task was created but local persistence failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if mediaTaskImagePreferAsync(c) {
			return mediaTaskImageAcceptedResponse(), nil
		}
		waitStartedAt := time.Now()
		task, waitErr := service.WaitMediaImageTask(ctx, task.TaskID, false)
		if task != nil {
			info.PersistedImageTask = model.ProjectOpenAIImageTask(task)
			if task.PrivateData.MediaImage != nil {
				trace.LastPollRequestID = task.PrivateData.MediaImage.LastPollRequestID
				trace.PollAttempts = task.PrivateData.MediaImage.PollAttempts
				trace.PollElapsedMilliseconds = time.Since(waitStartedAt).Milliseconds()
				recordMediaTaskImagePollRequestID(c, trace, trace.LastPollRequestID)
			}
		}
		if waitErr != nil || task == nil || !task.Status.IsTerminal() {
			return mediaTaskImageAcceptedResponse(), nil
		}
		switch task.Status {
		case model.TaskStatusSuccess:
			result := &mediaimageprotocol.Result{URLs: append([]string(nil), task.PrivateData.MediaImage.ResultURLs...)}
			return mediaTaskImageSuccessResponse(result)
		default:
			return nil, mediaTaskImageNoRetry(errors.New(mediaimageprotocol.SanitizeFailure(task.FailReason)), types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
	}

	headers, err := a.mediaTaskImageQueryHeaders(c, info)
	if err != nil {
		return nil, mediaTaskImageNoRetry(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	querySpec.Headers = headers
	waitResult, err := mediaimageprotocol.Wait(ctx, func(request *http.Request) (*http.Response, error) {
		logger.LogDebug(c, "media image upstream request: method=%s url=%s", request.Method, relaycommon.SanitizeURLForLog(request.URL.String()))
		return channel.DoRequest(c, request, info)
	}, querySpec, mediaimageprotocol.WaitOptions{SkipSleep: info.ChannelOtherSettings.DisableTaskPollingSleep})
	trace.PollAttempts = waitResult.Attempts
	trace.PollElapsedMilliseconds = waitResult.Elapsed.Milliseconds()
	recordMediaTaskImagePollRequestID(c, trace, waitResult.Observation.RequestID)
	if err != nil {
		if ctx.Err() != nil {
			logger.LogWarn(c, fmt.Sprintf(
				"media image task timed out: channel_id=%d task_id=%q create_request_id=%q poll_request_id=%q poll_attempts=%d elapsed_ms=%d",
				info.ChannelId, trace.TaskID, trace.CreateRequestID, trace.LastPollRequestID, trace.PollAttempts, trace.PollElapsedMilliseconds,
			))
			return nil, mediaTaskImageTimeoutError()
		}
		return nil, mediaTaskImageNoRetry(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	}
	switch waitResult.Observation.State {
	case mediaimageprotocol.StateCompleted:
		return mediaTaskImageSuccessResponse(&waitResult.Observation.Result)
	case mediaimageprotocol.StateFailed:
		return nil, mediaTaskImageNoRetry(errors.New(waitResult.Observation.Failure), types.ErrorCodeBadResponse, http.StatusBadGateway)
	default:
		return nil, mediaTaskImageNoRetry(errors.New("upstream media image task returned an untrusted state"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
}

func mediaTaskImagePreferAsync(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	for _, token := range strings.Split(c.GetHeader("Prefer"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "respond-async") {
			return true
		}
	}
	return false
}

func mediaTaskImageAcceptedResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     make(http.Header),
		Body:       http.NoBody,
	}
}

func mediaTaskImageRequestedCount(info *relaycommon.RelayInfo) uint {
	if info != nil {
		if imageRequest, ok := info.Request.(*dto.ImageRequest); ok && imageRequest.N != nil {
			return *imageRequest.N
		}
	}
	return 1
}

func validateMediaTaskImageSynchronousCount(response *http.Response, requested uint) error {
	if response == nil || response.Body == nil || requested == 0 || requested > dto.MaxImageN {
		return errors.New("media image synchronous response validation is incomplete")
	}
	body, err := readMediaTaskImageResponse(response)
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))

	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	if err := common.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("upstream media image response is not valid JSON: %w", err)
	}
	if len(payload.Data) == 0 {
		return errors.New("upstream media image response is missing data")
	}
	var images []json.RawMessage
	if err := common.Unmarshal(payload.Data, &images); err != nil {
		return errors.New("upstream media image response data must be an array")
	}
	if len(images) == 0 {
		return errors.New("upstream media image response returned no images")
	}
	if len(images) > int(requested) {
		return fmt.Errorf("upstream media image response returned %d images for requested n=%d", len(images), requested)
	}
	return nil
}

func mediaTaskImageExecutionContext(c *gin.Context) (context.Context, context.CancelFunc) {
	parent := context.Background()
	if c != nil && c.Request != nil {
		parent = c.Request.Context()
	}
	timeout := mediaTaskImageDefaultTimeout
	if common.RelayTimeout > 0 {
		relayTimeout := time.Duration(common.RelayTimeout) * time.Second
		if relayTimeout < timeout {
			timeout = relayTimeout
		}
	}
	return context.WithTimeout(parent, timeout)
}

func (a *Adaptor) doMediaTaskImageHTTP(c *gin.Context, ctx context.Context, info *relaycommon.RelayInfo, method, target string, body io.Reader, contentLength int64) (*http.Response, error) {
	if body == nil {
		body = http.NoBody
	}
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("create media image upstream request: %w", err)
	}
	if contentLength > 0 {
		request.ContentLength = contentLength
	}
	headers := request.Header
	if err := a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, fmt.Errorf("setup media image upstream headers: %w", err)
	}
	if method == http.MethodGet {
		request.Header.Del("Content-Type")
		request.Header.Set("Accept", "application/json")
	}
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range overrides {
		request.Header.Set(key, value)
		if strings.EqualFold(key, "Host") {
			request.Host = value
		}
	}
	logger.LogDebug(c, "media image upstream request: method=%s url=%s", method, relaycommon.SanitizeURLForLog(target))
	return channel.DoRequest(c, request, info)
}

func (a *Adaptor) mediaTaskImageQuerySpec(info *relaycommon.RelayInfo, taskID string, headers http.Header) mediaimageprotocol.QuerySpec {
	spec := mediaimageprotocol.QuerySpec{
		Protocol:     mediaimageprotocol.ProtocolMediaImageTaskV1,
		BaseURL:      info.ChannelBaseUrl,
		PathTemplate: mediaTaskImageQueryPath,
		TaskID:       taskID,
		APIKey:       info.ApiKey,
		Headers:      headers,
	}
	if a.route.Auth != nil {
		spec.AuthType = a.route.Auth.Type
		spec.AuthName = a.route.Auth.Name
		spec.AuthValueTemplate = a.route.Auth.Value
	}
	return spec
}

func (a *Adaptor) mediaTaskImageQueryHeaders(c *gin.Context, info *relaycommon.RelayInfo) (http.Header, error) {
	headers := make(http.Header)
	if err := a.SetupRequestHeader(c, &headers, info); err != nil {
		return nil, fmt.Errorf("setup media image query headers: %w", err)
	}
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range overrides {
		headers.Set(key, value)
	}
	return headers, nil
}

func recordMediaTaskImagePollRequestID(c *gin.Context, trace *relaycommon.UpstreamTaskTrace, requestID string) {
	if trace == nil {
		return
	}
	requestID = mediaimageprotocol.SanitizeRequestID(requestID)
	if requestID != "" {
		trace.LastPollRequestID = requestID
	}
	if c == nil {
		return
	}
	if trace.LastPollRequestID != "" {
		c.Set(common.UpstreamRequestIdKey, trace.LastPollRequestID)
	} else if trace.CreateRequestID != "" {
		c.Set(common.UpstreamRequestIdKey, trace.CreateRequestID)
	}
}

func mediaTaskImageSuccessResponse(result *mediaimageprotocol.Result) (*http.Response, error) {
	if result == nil {
		return nil, mediaTaskImageNoRetry(errors.New("upstream succeeded response has no result"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	urls, err := mediaimageprotocol.NormalizeResultURLs(*result, dto.MaxImageN)
	if err != nil {
		return nil, mediaTaskImageNoRetry(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}

	response := dto.ImageResponse{
		Created: time.Now().Unix(),
		Data:    make([]dto.ImageData, 0, len(urls)),
	}
	for _, value := range urls {
		response.Data = append(response.Data, dto.ImageData{Url: value})
	}
	body, err := common.Marshal(response)
	if err != nil {
		return nil, mediaTaskImageNoRetry(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func readMediaTaskImageResponse(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errors.New("upstream media task response body is empty")
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, mediaTaskImageMaxResponseBytes+1))
	if err != nil {
		return nil, errors.New("read upstream media task response")
	}
	if len(body) > mediaTaskImageMaxResponseBytes {
		return nil, errors.New("upstream media task response is too large")
	}
	return body, nil
}

func closeMediaTaskImageResponse(response *http.Response) error {
	if response == nil || response.Body == nil {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	return response.Body.Close()
}

func mediaTaskImageNoRetry(err error, code types.ErrorCode, status int) error {
	return types.NewErrorWithStatusCode(err, code, status, types.ErrOptionWithSkipRetry())
}

func mediaTaskImageTimeoutError() error {
	return mediaTaskImageNoRetry(errors.New("upstream media task timed out"), types.ErrorCodeChannelResponseTimeExceeded, http.StatusGatewayTimeout)
}
