package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Only known read-only operations belong here. In particular, this dispatcher
// must never use testChannel, construct a generation body, or infer Link identity.
func runChannelConnectionProbe(ctx context.Context, channel *model.Channel) (result testResult) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ctx, network := observeChannelTestNetwork(ctx)
	defer func() {
		result.upstreamAttempted = network.attempted.Load()
		result.upstreamResponded = network.received.Load()
		result.testedModel = channelAutoCheckModelName(channel)
	}()
	if channel.Type != constant.ChannelTypeOpenAI {
		return testResult{checkUnsupported: true}
	}
	// A private projection avoids advancing the saved multi-key polling cursor.
	snapshot := *channel
	if snapshot.ChannelInfo.IsMultiKey {
		snapshot.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
	}
	key, _, apiErr := snapshot.GetNextEnabledKey()
	if apiErr != nil {
		return testResult{localErr: apiErr, newAPIError: apiErr}
	}
	base, err := url.Parse(strings.TrimRight(channel.GetBaseURL(), "/"))
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		apiErr := types.NewError(errors.New("invalid connection probe base URL"), types.ErrorCodeInvalidApiType)
		return testResult{localErr: apiErr, newAPIError: apiErr}
	}
	// Same OpenAI model-list path as the existing model discovery operation.
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/models"
	base.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return testResult{localErr: errors.New("invalid connection probe request")}
	}
	headers, err := buildFetchModelsHeaders(&snapshot, strings.TrimSpace(key))
	if err != nil {
		apiErr := types.NewError(errors.New("invalid connection probe headers"), types.ErrorCodeInvalidApiType)
		return testResult{localErr: apiErr, newAPIError: apiErr}
	}
	req.Header = headers
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/models", nil)
	c.Set("channel_test_protocol", "openai")
	clienterrlog.InstallHTTPExchange(c)
	result.context = c
	settings := channel.GetSetting()
	client, err := service.GetHttpClientWithProxySettings(settings.Proxy, settings)
	if err != nil {
		apiErr := types.NewError(errors.New("invalid connection probe proxy"), types.ErrorCodeInvalidApiType)
		result.localErr, result.newAPIError = apiErr, apiErr
		return result
	}
	probeClient := *client
	probeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := probeClient.Do(req)
	if err != nil {
		apiErr := types.NewError(errors.New("connection probe could not receive a response"), types.ErrorCodeDoRequestFailed)
		result.localErr, result.newAPIError = apiErr, apiErr
		return result
	}
	defer resp.Body.Close()
	result.upstreamStatus = resp.StatusCode
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return result
	}
	if resp.StatusCode == 404 || resp.StatusCode == 405 || (resp.StatusCode >= 300 && resp.StatusCode < 400) {
		result.checkUnsupported = true
		return result
	}
	apiErr = types.NewError(errors.New("connection probe received an unsuccessful HTTP response"), types.ErrorCodeBadResponseStatusCode, types.ErrOptionWithStatusCode(resp.StatusCode))
	result.localErr, result.newAPIError = apiErr, apiErr
	return result
}

// This is an observation, not a channel health/enablement authority. HTTP errors
// establish a response from some node, not that generation or the backend works.
func channelConnectionObservation(result testResult) string {
	if result.upstreamStatus > 0 {
		switch {
		case result.upstreamStatus == 401 || result.upstreamStatus == 403:
			return "auth_error"
		case result.upstreamStatus == 429:
			return "rate_limited"
		case result.upstreamStatus >= 500:
			return "service_error"
		case result.checkUnsupported:
			return "probe_unsupported"
		default:
			return "response_received"
		}
	}
	if result.upstreamResponded {
		return "response_received"
	}
	if result.upstreamAttempted {
		return "connection_error"
	}
	return "not_verified"
}

// Media signals only prevent generation probes; they never grant Link identity
// or change production routing. Resolve aliases through the existing mapper.
func channelAutoCheckMediaKind(channel *model.Channel) string {
	switch channel.Type {
	case constant.ChannelTypeSeedanceLink, constant.ChannelTypeKling, constant.ChannelTypeVidu, constant.ChannelTypeSora, constant.ChannelTypeDoubaoVideo, constant.ChannelTypeJimeng:
		return "video"
	case constant.ChannelTypeAsyncImage, constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus:
		return "image"
	}
	if validateChannelAutoCheckSettings(channel) != nil {
		return ""
	}
	name := channelAutoCheckModelName(channel)
	provider := name
	if mapping, err := decodeChannelModelMapping(channel); err == nil {
		if resolved, _, err := model.ResolveModelMapping(name, mapping); err == nil {
			provider = resolved
		}
	}
	for _, candidate := range []string{name, provider} {
		if common.IsImageGenerationModel(candidate) {
			return "image"
		}
		// This deployment has Seedance media aliases on ordinary OpenAI channels.
		// A conservative stop signal is not a Seedance routing/capability declaration.
		if strings.HasPrefix(strings.ToLower(candidate), "seedance") {
			return "video"
		}
	}
	if normalizeChannelTestEndpoint(channel, name, "") == string(constant.EndpointTypeImageGeneration) {
		return "image"
	}
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		if cfg := channel.GetOtherSettings().AdvancedCustom; cfg != nil {
			for _, endpoint := range cfg.SupportedEndpointTypesForModel(name) {
				switch endpoint {
				case constant.EndpointTypeImageGeneration:
					return "image"
				case constant.EndpointTypeOpenAIVideo, constant.EndpointTypeModelArkVideo:
					return "video"
				}
			}
		}
	}
	if channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(name, "seedream") {
		return "image"
	}
	return ""
}
