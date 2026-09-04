package asyncimage

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
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	client *http.Client
}

type imagePayload struct {
	Prompt       string `json:"prompt"`
	GenType      string `json:"genType,omitempty"`
	AspectRatio  string `json:"aspectRatio,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"outputFormat,omitempty"`
}

type apiEnvelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string   `json:"taskId"`
		Status string   `json:"status"`
		Result []string `json:"result"`
	} `json:"data"`
}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || strings.TrimSpace(info.ChannelBaseUrl) == "" {
		return "", errors.New("channel base URL is required")
	}
	modelName := strings.TrimSpace(info.UpstreamModelName)
	if modelName == "" {
		return "", errors.New("upstream model is required")
	}
	return strings.TrimRight(info.ChannelBaseUrl, "/") + "/api/v2/open/aigc/" + url.PathEscape(modelName), nil
}

func (a *Adaptor) SetupRequestHeader(_ *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil || strings.TrimSpace(info.ApiKey) == "" {
		return errors.New("API key is required")
	}
	req.Set("Authorization", "Bearer "+info.ApiKey)
	req.Set("Content-Type", "application/json")
	req.Set("Accept", "application/json")
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("async image channel only supports images generations")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("async image channel does not support Claude requests")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("async image channel does not support Gemini requests")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("async image channel does not support rerank requests")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("async image channel does not support embedding requests")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("async image channel does not support audio requests")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("async image channel does not support responses requests")
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil || info.RelayMode != relayconstant.RelayModeImagesGenerations {
		return nil, badRequest("async image channel only supports /v1/images/generations")
	}
	if request.N != nil && *request.N != 1 {
		return nil, badRequest("n must be exactly 1 for async image channels")
	}
	if request.ResponseFormat != "" && request.ResponseFormat != "url" {
		return nil, badRequest("response_format must be url")
	}
	if err := rejectUnsupportedImageFields(request); err != nil {
		return nil, err
	}

	modelName := strings.TrimSpace(request.Model)
	if modelName == "" {
		return nil, badRequest("model is required")
	}
	if !containsModel(modelName) {
		return nil, badRequest(fmt.Sprintf("unsupported async image model %q", modelName))
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, badRequest("prompt is required")
	}
	if err := validatePrompt(modelName, prompt); err != nil {
		return nil, err
	}

	extra, err := parseExtraFields(request.ExtraFields)
	if err != nil {
		return nil, err
	}
	// Until FunCloud input-image pricing is verified, the published contract is
	// text-to-image only. Keeping this rejection here prevents a successful
	// reference-image request from bypassing the fixed pre-consume price.
	if _, hasReferenceImages := extra["reference_images"]; hasReferenceImages {
		return nil, badRequest("reference_images are not available until input-image pricing is configured")
	}
	aspectRatio, err := parseStringField(extra, "aspect_ratio")
	if err != nil {
		return nil, err
	}
	resolution, err := parseStringField(extra, "resolution")
	if err != nil {
		return nil, err
	}
	quality, err := parseStringField(extra, "quality")
	if err != nil {
		return nil, err
	}
	extraOutputFormat, err := parseStringField(extra, "output_format")
	if err != nil {
		return nil, err
	}
	outputFormat, err := parseTopLevelString(request.OutputFormat, "output_format")
	if err != nil {
		return nil, err
	}
	if outputFormat != "" && extraOutputFormat != "" && outputFormat != extraOutputFormat {
		return nil, badRequest("output_format and extra_fields.output_format must match")
	}
	if outputFormat == "" {
		outputFormat = extraOutputFormat
	}
	for key := range extra {
		if key != "reference_images" && key != "aspect_ratio" && key != "resolution" && key != "quality" && key != "output_format" {
			return nil, badRequest(fmt.Sprintf("extra_fields.%s is not supported", key))
		}
	}

	payload := imagePayload{Prompt: prompt, AspectRatio: aspectRatio}
	if err := validateModelFields(modelName, &payload, request.Size, request.Quality, resolution, quality, outputFormat, aspectRatio); err != nil {
		return nil, err
	}
	if info.UpstreamModelName == "" {
		info.UpstreamModelName = modelName
	}
	return payload, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info == nil {
		return nil, upstreamError("missing relay info")
	}
	requestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, upstreamError("failed to build create request URL")
	}
	reqCtx := context.Background()
	if c != nil && c.Request != nil {
		reqCtx = c.Request.Context()
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, requestURL, requestBody)
	if err != nil {
		return nil, upstreamError("failed to build create request")
	}
	// ContentLength comes from the replayable upstream body when available.
	if sized, ok := requestBody.(interface{ Size() int64 }); ok {
		req.ContentLength = sized.Size()
	}
	if err := a.SetupRequestHeader(c, &req.Header, info); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeChannelHeaderOverrideInvalid, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	for key, value := range overrides {
		applyHeaderOverride(req, key, value)
	}
	client, err := a.httpClient(info)
	if err != nil {
		return nil, upstreamError("failed to initialize upstream client")
	}
	client = channel.ImageRelayHTTPClient(client, info.StartTime)
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || (c != nil && c.Request != nil && errors.Is(c.Request.Context().Err(), context.Canceled)) {
			return nil, channel.ImageRelayClientCanceledError()
		}
		if errors.Is(err, context.DeadlineExceeded) || (c != nil && c.Request != nil && errors.Is(c.Request.Context().Err(), context.DeadlineExceeded)) {
			return nil, timeoutError(err)
		}
		return nil, upstreamError("create request failed")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// Keep the response body for DoResponse so FunCloud's application
		// envelope (including code=10002) can be mapped to the public error.
		return resp, nil
	}
	return resp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info == nil {
		return nil, upstreamError("missing relay info")
	}
	if resp == nil || resp.Body == nil {
		return nil, upstreamError("empty create response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, providerHTTPError(resp, "create request")
	}
	defer service.CloseResponseBodyGracefully(resp)
	initial, err := decodeEnvelope(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, channel.ImageRelayClientCanceledError()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, timeoutError(err)
		}
		return nil, upstreamError("invalid create response")
	}
	if apiErr := envelopeError(initial); apiErr != nil {
		return nil, apiErr
	}
	if strings.EqualFold(initial.Data.Status, "success") {
		return a.writeImageResponse(c, initial.Data.Result)
	}
	if !strings.EqualFold(initial.Data.Status, "processing") && !strings.EqualFold(initial.Data.Status, "queued") {
		return nil, upstreamError("unknown create task status")
	}
	if strings.TrimSpace(initial.Data.TaskID) == "" {
		return nil, upstreamError("create response did not include task ID")
	}
	ctx, cancel := responseContext(c, info)
	defer cancel()
	client, err := a.httpClient(info)
	if err != nil {
		return nil, upstreamError("failed to initialize polling client")
	}
	pollURL := strings.TrimRight(info.ChannelBaseUrl, "/") + "/api/v2/open/aigc/" + url.PathEscape(initial.Data.TaskID)
	firstPoll := true
	for {
		if !firstPoll {
			if err := waitForNextPoll(ctx, info); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil, channel.ImageRelayClientCanceledError()
				}
				return nil, timeoutError(err)
			}
		}
		firstPoll = false
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
		if err != nil {
			return nil, upstreamError("failed to build polling request")
		}
		request.Header.Set("Authorization", "Bearer "+info.ApiKey)
		request.Header.Set("Accept", "application/json")
		overrides, overrideErr := channel.ResolveHeaderOverride(info, c)
		if overrideErr != nil {
			return nil, types.NewErrorWithStatusCode(overrideErr, types.ErrorCodeChannelHeaderOverrideInvalid, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		for key, value := range overrides {
			applyHeaderOverride(request, key, value)
		}
		pollResp, err := client.Do(request)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, channel.ImageRelayClientCanceledError()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, timeoutError(err)
			}
			return nil, upstreamError("polling request failed")
		}
		if pollResp.StatusCode < http.StatusOK || pollResp.StatusCode >= http.StatusMultipleChoices {
			return nil, providerHTTPError(pollResp, "polling request")
		}
		result, decodeErr := func() (apiEnvelope, error) {
			defer service.CloseResponseBodyGracefully(pollResp)
			return decodeEnvelope(pollResp.Body)
		}()
		if decodeErr != nil {
			if errors.Is(decodeErr, context.Canceled) {
				return nil, channel.ImageRelayClientCanceledError()
			}
			if errors.Is(decodeErr, context.DeadlineExceeded) {
				return nil, timeoutError(decodeErr)
			}
			return nil, upstreamError("invalid polling response")
		}
		if apiErr := envelopeError(result); apiErr != nil {
			return nil, apiErr
		}
		switch strings.ToLower(strings.TrimSpace(result.Data.Status)) {
		case "processing", "queued":
			continue
		case "success":
			return a.writeImageResponse(c, result.Data.Result)
		default:
			return nil, upstreamError("unknown polling task status")
		}
	}
}

func (a *Adaptor) GetModelList() []string { return ModelList }

func (a *Adaptor) GetChannelName() string { return ChannelName }

func (a *Adaptor) httpClient(info *relaycommon.RelayInfo) (*http.Client, error) {
	if a != nil && a.client != nil {
		return a.client, nil
	}
	return service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
}

func (a *Adaptor) writeImageResponse(c *gin.Context, urls []string) (any, *types.NewAPIError) {
	if len(urls) != 1 {
		return nil, upstreamError("provider result must contain exactly one HTTP(S) URL")
	}
	imageURL := strings.TrimSpace(urls[0])
	if !isHTTPURL(imageURL) {
		return nil, upstreamError("provider result must contain exactly one HTTP(S) URL")
	}
	response := dto.ImageResponse{Created: time.Now().Unix(), Data: []dto.ImageData{{Url: imageURL}}}
	if c == nil {
		return nil, upstreamError("missing response context")
	}
	c.JSON(http.StatusOK, response)
	return &dto.Usage{}, nil
}

func responseContext(c *gin.Context, info *relaycommon.RelayInfo) (context.Context, context.CancelFunc) {
	parent := context.Background()
	if c != nil && c.Request != nil {
		parent = c.Request.Context()
	}
	startTime := time.Time{}
	if info != nil {
		startTime = info.StartTime
	}
	return channel.ImageRelayTimeoutContext(parent, startTime)
}

func waitForNextPoll(ctx context.Context, info *relaycommon.RelayInfo) error {
	delay := 5 * time.Second
	if info != nil && !info.StartTime.IsZero() && time.Since(info.StartTime) < 30*time.Second {
		delay = 3 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func decodeEnvelope(reader io.Reader) (apiEnvelope, error) {
	var envelope apiEnvelope
	if err := common.DecodeJson(reader, &envelope); err != nil {
		return apiEnvelope{}, err
	}
	return envelope, nil
}

func envelopeError(envelope apiEnvelope) *types.NewAPIError {
	if envelope.Code == 0 {
		return nil
	}
	if envelope.Code == 10005 || envelope.Code == 40001 {
		common.SysError(fmt.Sprintf("async image provider credential or balance error: code=%d", envelope.Code))
	}
	status := http.StatusBadGateway
	if envelope.Code == 10002 {
		status = http.StatusBadRequest
	}
	message := fmt.Sprintf("async image provider error (%d)", envelope.Code)
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponse, status, types.ErrOptionWithSkipRetry())
}

func providerHTTPError(resp *http.Response, operation string) *types.NewAPIError {
	if resp == nil {
		return upstreamError(operation + " returned an empty response")
	}
	defer service.CloseResponseBodyGracefully(resp)
	if resp.Body != nil {
		if envelope, err := decodeEnvelope(resp.Body); err == nil {
			if apiErr := envelopeError(envelope); apiErr != nil {
				return apiErr
			}
		}
	}
	return upstreamError(fmt.Sprintf("%s returned HTTP %d", operation, resp.StatusCode))
}

func applyHeaderOverride(req *http.Request, key, value string) {
	if req == nil {
		return
	}
	if strings.EqualFold(key, "Host") {
		req.Host = value
		return
	}
	req.Header.Set(key, value)
}

func badRequest(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

func upstreamError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}

func timeoutError(err error) *types.NewAPIError {
	if err == nil {
		err = context.DeadlineExceeded
	}
	return types.NewErrorWithStatusCode(fmt.Errorf("async image provider timed out: %w", err), types.ErrorCodeChannelResponseTimeExceeded, http.StatusGatewayTimeout, types.ErrOptionWithSkipRetry())
}

func isHTTPURL(raw string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
}

func containsModel(modelName string) bool {
	for _, candidate := range ModelList {
		if candidate == modelName {
			return true
		}
	}
	return false
}

func validatePrompt(modelName, prompt string) error {
	length := utf8.RuneCountInString(prompt)
	if strings.HasPrefix(modelName, "seedream-") {
		if length < 3 || length > 3000 {
			return badRequest("seedream prompt length must be between 3 and 3000 characters")
		}
		return nil
	}
	if length > 20000 {
		return badRequest("nano banana prompt length must not exceed 20000 characters")
	}
	return nil
}

func rejectUnsupportedImageFields(request dto.ImageRequest) error {
	if len(request.Style) > 0 || len(request.User) > 0 || len(request.Background) > 0 || len(request.Moderation) > 0 ||
		len(request.OutputCompression) > 0 || len(request.PartialImages) > 0 || len(request.Images) > 0 || len(request.Mask) > 0 ||
		len(request.InputFidelity) > 0 || request.Watermark != nil || len(request.WatermarkEnabled) > 0 || len(request.UserId) > 0 || len(request.Image) > 0 ||
		request.Stream != nil || len(request.Extra) > 0 {
		return badRequest("request contains unsupported image fields")
	}
	return nil
}

func parseExtraFields(raw []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, badRequest("extra_fields must be a JSON object")
	}
	return fields, nil
}

func parseStringField(fields map[string]json.RawMessage, key string) (string, error) {
	raw, ok := fields[key]
	if !ok {
		return "", nil
	}
	return parseTopLevelString(raw, "extra_fields."+key)
}

func parseTopLevelString(raw json.RawMessage, fieldName string) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", nil
	}
	if string(trimmed) == "null" {
		return "", badRequest(fieldName + " must be a string")
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", badRequest(fieldName + " must be a string")
	}
	return strings.TrimSpace(value), nil
}

func validateModelFields(modelName string, payload *imagePayload, size, topQuality, resolution, extraQuality, outputFormat, aspectRatio string) error {
	publishedSize, supported := constant.FunCloudImagePublishedSize(modelName)
	if !supported {
		return badRequest(fmt.Sprintf("unsupported async image model %q", modelName))
	}
	if aspectRatio != "" {
		allowed := allAspectRatios
		if modelName == seedream5Lite {
			allowed = seedreamLiteAspectRatios
		} else if modelName == seedream5Pro {
			allowed = seedreamProAspectRatios
		}
		if _, ok := allowed[aspectRatio]; !ok {
			return badRequest("unsupported aspect_ratio for model")
		}
	}
	if modelName == nanoBanana2Lite {
		if size != "" || topQuality != "" || resolution != "" || extraQuality != "" || outputFormat != "" {
			return badRequest("nano-banana-2-lite does not support resolution, quality, or output format")
		}
		return nil
	}
	if modelName == nanoBanana2 {
		// The configured customer price is for the 1K variant. Do not expose
		// higher provider variants until their billing multipliers are wired.
		if topQuality != "" || extraQuality != "" {
			return badRequest("nano-banana-2 does not support quality")
		}
		if resolution == "" {
			resolution = size
		}
		if resolution == "" {
			resolution = publishedSize
		}
		if resolution != publishedSize {
			return badRequest("nano-banana-2 is currently published at fixed 1K resolution")
		}
		if size != "" && resolution != "" && size != resolution {
			return badRequest("size and extra_fields.resolution must match")
		}
		if outputFormat != "" && outputFormat != "jpg" && outputFormat != "png" {
			return badRequest("output_format must be jpg or png")
		}
		payload.Resolution, payload.OutputFormat = resolution, outputFormat
		return nil
	}

	payload.GenType = "t2i"
	quality := strings.TrimSpace(topQuality)
	if quality == "" {
		quality = strings.TrimSpace(extraQuality)
	}
	if topQuality != "" && extraQuality != "" && topQuality != extraQuality {
		return badRequest("quality and extra_fields.quality must match")
	}
	if quality == "" {
		quality = "basic"
	}
	if quality != "basic" && quality != "high" {
		return badRequest("quality must be basic or high")
	}
	if size != "" && size != publishedSize {
		return badRequest("this Seedream model is currently published at a fixed base resolution")
	}
	if size == "" {
		size = publishedSize
	}
	if quality != "basic" {
		// Seedream higher quality/resolution variants have different provider
		// prices; rejecting them keeps pre-consume and provider cost aligned.
		return badRequest("this Seedream model is currently published at fixed basic quality")
	}
	if resolution != "" {
		return badRequest("seedream models do not support extra_fields.resolution")
	}
	if outputFormat != "" {
		return badRequest("seedream models do not support output_format")
	}
	payload.Quality = quality
	return nil
}
