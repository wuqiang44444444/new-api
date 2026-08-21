package moxingimage

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

const ChannelName = "Moxing Image"

type Adaptor struct {
	client *http.Client
}

type imagePayload struct {
	Capability     string `json:"capability"`
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	ResponseFormat string `json:"response_format"`
	Size           string `json:"size"`
}

type providerImage struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
}

type providerResponse struct {
	Data  []providerImage `json:"data"`
	Error json.RawMessage `json:"error"`
	Model string          `json:"model"`
}

type providerErrorBody struct {
	Code string `json:"code"`
	Type string `json:"type"`
}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || strings.TrimSpace(info.ChannelBaseUrl) == "" {
		return "", errors.New("channel base URL is required")
	}
	return strings.TrimRight(info.ChannelBaseUrl, "/") + "/v1/images/generations", nil
}

func (a *Adaptor) SetupRequestHeader(_ *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil || strings.TrimSpace(info.ApiKey) == "" {
		return errors.New("API key is required")
	}
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("Moxing image channel only supports images generations")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("Moxing image channel does not support Claude requests")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("Moxing image channel does not support Gemini requests")
}

func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("Moxing image channel does not support rerank requests")
}

func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("Moxing image channel does not support embedding requests")
}

func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("Moxing image channel does not support audio requests")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("Moxing image channel does not support responses requests")
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil || info.RelayMode != relayconstant.RelayModeImagesGenerations {
		return nil, badRequest("Moxing image channel only supports /v1/images/generations")
	}
	if request.N != nil && *request.N != 1 {
		return nil, badRequest("n must be exactly 1 for Moxing image channels")
	}
	if request.ResponseFormat != "" && request.ResponseFormat != "url" {
		return nil, badRequest("response_format must be url")
	}
	if err := rejectUnsupportedImageFields(request); err != nil {
		return nil, err
	}
	if err := rejectExtraFields(request.ExtraFields); err != nil {
		return nil, err
	}

	modelName := strings.TrimSpace(request.Model)
	fixedSize, supported := constant.MoxingImageFixedSize(modelName)
	if !supported {
		return nil, badRequest(fmt.Sprintf("unsupported Moxing image model %q", modelName))
	}
	if strings.TrimSpace(info.UpstreamModelName) != modelName {
		return nil, badRequest("mapped Moxing image model does not match the selected channel")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, badRequest("prompt is required")
	}
	if utf8.RuneCountInString(prompt) > 3000 {
		return nil, badRequest("prompt length must not exceed 3000 characters")
	}

	size := strings.TrimSpace(request.Size)
	if size == "" {
		size = fixedSize
	}
	if size != fixedSize {
		return nil, badRequest(fmt.Sprintf("Moxing image model %q is currently published at fixed %s resolution", modelName, fixedSize))
	}

	return imagePayload{
		Capability:     "image_generation",
		Model:          modelName,
		Prompt:         prompt,
		ResponseFormat: "url",
		Size:           size,
	}, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info == nil {
		return nil, upstreamError("missing relay info")
	}
	if common.RelayTimeout <= 0 {
		return nil, timeoutConfigurationError()
	}
	requestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, upstreamError("failed to build image request URL")
	}
	requestContext := context.Background()
	if c != nil && c.Request != nil {
		requestContext = c.Request.Context()
	}
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, requestURL, requestBody)
	if err != nil {
		return nil, upstreamError("failed to build image request")
	}
	if info.UpstreamRequestBodySize > 0 {
		request.ContentLength = info.UpstreamRequestBodySize
	}
	if err := a.SetupRequestHeader(c, &request.Header, info); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	overrides, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeChannelHeaderOverrideInvalid, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	for key, value := range overrides {
		applyHeaderOverride(request, key, value)
	}
	client, err := a.httpClient(info)
	if err != nil {
		return nil, upstreamError("failed to initialize image provider client")
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, timeoutError(err)
		}
		return nil, upstreamError("image request failed")
	}
	return response, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info == nil {
		return nil, upstreamError("missing relay info")
	}
	if response == nil || response.Body == nil {
		return nil, upstreamError("empty image response")
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, upstreamError("failed to read image response")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, providerHTTPError(response.StatusCode, body)
	}

	var provider providerResponse
	if err := common.Unmarshal(body, &provider); err != nil {
		return nil, upstreamError("invalid image response")
	}
	if hasProviderError(provider.Error) {
		return nil, providerApplicationError(provider.Error)
	}
	if modelName := strings.TrimSpace(provider.Model); modelName != "" && modelName != info.UpstreamModelName {
		return nil, upstreamError("image response model does not match the request")
	}
	if len(provider.Data) != 1 {
		return nil, upstreamError("image provider result must contain exactly one image")
	}
	imageURL := strings.TrimSpace(provider.Data[0].URL)
	if !isHTTPURL(imageURL) || strings.TrimSpace(provider.Data[0].B64JSON) != "" {
		return nil, upstreamError("image provider result must contain exactly one HTTP(S) URL")
	}
	if c == nil {
		return nil, upstreamError("missing response context")
	}
	c.JSON(http.StatusOK, dto.ImageResponse{
		Created: time.Now().Unix(),
		Data:    []dto.ImageData{{Url: imageURL}},
	})
	return &dto.Usage{}, nil
}

func (a *Adaptor) GetModelList() []string { return constant.MoxingImageProviderModels() }

func (a *Adaptor) GetChannelName() string { return ChannelName }

func (a *Adaptor) httpClient(info *relaycommon.RelayInfo) (*http.Client, error) {
	if a != nil && a.client != nil {
		return a.client, nil
	}
	return service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
}

func rejectUnsupportedImageFields(request dto.ImageRequest) error {
	if request.Quality != "" || len(request.Style) > 0 || len(request.User) > 0 || len(request.Background) > 0 ||
		len(request.Moderation) > 0 || len(request.OutputFormat) > 0 || len(request.OutputCompression) > 0 ||
		len(request.PartialImages) > 0 || len(request.Images) > 0 || len(request.Mask) > 0 ||
		len(request.InputFidelity) > 0 || request.Watermark != nil || len(request.WatermarkEnabled) > 0 ||
		len(request.UserId) > 0 || len(request.Image) > 0 || len(request.Extra) > 0 ||
		request.Stream != nil {
		return badRequest("request contains unsupported image fields")
	}
	return nil
}

func rejectExtraFields(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(trimmed, &fields); err != nil || fields == nil {
		return badRequest("extra_fields must be a JSON object")
	}
	if len(fields) > 0 {
		return badRequest("extra_fields are not supported by the published Moxing image profile")
	}
	return nil
}

func hasProviderError(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && string(trimmed) != "null" && string(trimmed) != "{}" && string(trimmed) != `""`
}

func providerApplicationError(raw json.RawMessage) *types.NewAPIError {
	code := providerErrorCode(raw)
	status := http.StatusBadGateway
	if code == "invalid_request_error" {
		status = http.StatusBadRequest
	} else if code == "rate_limit_exceeded" {
		status = http.StatusTooManyRequests
	}
	if code == "invalid_api_key" || code == "insufficient_quota" {
		common.SysError("Moxing image provider credential or balance error")
	}
	message := "image provider returned an application error"
	if code != "" {
		message = fmt.Sprintf("image provider error (%s)", code)
	}
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponse, status, types.ErrOptionWithSkipRetry())
}

func providerHTTPError(statusCode int, body []byte) *types.NewAPIError {
	var envelope providerResponse
	if err := common.Unmarshal(body, &envelope); err == nil && hasProviderError(envelope.Error) {
		return providerApplicationError(envelope.Error)
	}
	status := http.StatusBadGateway
	if statusCode == http.StatusTooManyRequests {
		status = http.StatusTooManyRequests
	}
	return types.NewErrorWithStatusCode(
		fmt.Errorf("image provider returned HTTP %d", statusCode),
		types.ErrorCodeBadResponse,
		status,
		types.ErrOptionWithSkipRetry(),
	)
}

func providerErrorCode(raw json.RawMessage) string {
	var body providerErrorBody
	if err := common.Unmarshal(raw, &body); err != nil {
		return ""
	}
	if code := strings.TrimSpace(body.Code); code != "" {
		return code
	}
	return strings.TrimSpace(body.Type)
}

func applyHeaderOverride(request *http.Request, key, value string) {
	if request == nil {
		return
	}
	if strings.EqualFold(key, "Host") {
		request.Host = value
		return
	}
	request.Header.Set(key, value)
}

func isHTTPURL(raw string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	return err == nil && parsed.Host != "" && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
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
	return types.NewErrorWithStatusCode(
		fmt.Errorf("image provider timed out: %w", err),
		types.ErrorCodeChannelResponseTimeExceeded,
		http.StatusGatewayTimeout,
		types.ErrOptionWithSkipRetry(),
	)
}

func timeoutConfigurationError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New("RELAY_TIMEOUT must be positive for Moxing image channels"),
		types.ErrorCodeInvalidRequest,
		http.StatusServiceUnavailable,
		types.ErrOptionWithSkipRetry(),
	)
}
