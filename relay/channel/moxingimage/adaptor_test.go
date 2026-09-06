package moxingimage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moxingImageContext(ctx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil).WithContext(ctx)
	return c, recorder
}

func moxingImageInfo(baseURL string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    baseURL,
			ApiKey:            "test-key",
			UpstreamModelName: constant.MoxingImageProviderModelSeedream5Lite,
			ChannelSetting:    dto.ChannelSettings{},
		},
	}
}

func requireMoxingBadRequest(t *testing.T, err error) {
	t.Helper()
	apiErr, ok := err.(*types.NewAPIError)
	require.True(t, ok, "expected NewAPIError, got %T", err)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
}

func TestConvertImageRequestBuildsPublishedModelPayloads(t *testing.T) {
	tests := []struct {
		name  string
		model string
		size  string
	}{
		{name: "lite", model: constant.MoxingImageProviderModelSeedream5Lite, size: constant.MoxingImageSeedream5LiteSize},
		{name: "pro", model: constant.MoxingImageProviderModelSeedream5Pro, size: constant.MoxingImageSeedream5ProSize},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			n := uint(1)
			request := dto.ImageRequest{
				Model:          test.model,
				Prompt:         "  一只猫  ",
				N:              &n,
				ResponseFormat: "url",
			}
			info := moxingImageInfo("https://example.com")
			info.UpstreamModelName = test.model

			converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, request)
			require.NoError(t, err)
			assert.Equal(t, imagePayload{
				Capability:     "image_generation",
				Model:          test.model,
				Prompt:         "一只猫",
				ResponseFormat: "url",
				Size:           test.size,
			}, converted)
		})
	}
}

func TestConvertImageRequestRejectsUnpublishedContract(t *testing.T) {
	two := uint(2)
	streamTrue := true
	cases := []dto.ImageRequest{
		{Model: "doubao-seedream-unknown", Prompt: "x", Size: "2K"},
		{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "x", Size: "4K"},
		{Model: constant.MoxingImageProviderModelSeedream5Pro, Prompt: "x", Size: "1K"},
		{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "x", Size: "2K", N: &two},
		{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "x", Size: "2K", ResponseFormat: "b64_json"},
		{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "x", Size: "2K", Stream: &streamTrue},
		{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "x", Size: "2K", OutputFormat: json.RawMessage(`"png"`)},
		{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "x", Size: "2K", ExtraFields: json.RawMessage(`{"reference_images":["https://example.com/a.png"]}`)},
		{Model: constant.MoxingImageProviderModelSeedream5Lite, Prompt: "x", Size: "2K", Extra: map[string]json.RawMessage{"capability": json.RawMessage(`"image_generation"`)}},
	}

	for _, request := range cases {
		_, err := (&Adaptor{}).ConvertImageRequest(nil, moxingImageInfo("https://example.com"), request)
		requireMoxingBadRequest(t, err)
	}
}

func TestDoRequestUsesFixedEndpointAndHeaders(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 60
	defer func() { common.RelayTimeout = originalRelayTimeout }()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://example.com/v1/images/generations", request.URL.String())
		assert.Equal(t, "Bearer test-key", request.Header.Get("Authorization"))
		assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
		assert.Equal(t, int64(14), request.ContentLength)
		assert.Equal(t, "provider.internal", request.Host)
		return testHTTPResponse(http.StatusOK, `{"data":[{"url":"https://cdn.example.com/image.png"}]}`), nil
	})}
	info := moxingImageInfo("https://example.com/")
	info.ChannelMeta.HeadersOverride = map[string]any{"Host": "provider.internal"}
	c, _ := moxingImageContext(context.Background())

	response, err := (&Adaptor{client: client}).DoRequest(c, info, bytes.NewBufferString(`{"prompt":"x"}`))

	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestDoResponseWritesUnifiedImageAndZeroUsage(t *testing.T) {
	c, recorder := moxingImageContext(context.Background())
	info := moxingImageInfo("https://example.com")
	response := testHTTPResponse(http.StatusOK, `{
		"model":"doubao-seedream-5-0-260128",
		"data":[{"url":"https://cdn.example.com/image.png","size":"2048x2048"}],
		"usage":{"generated_images":"1","output_tokens":"16384","total_tokens":"16384"}
	}`)

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, info)

	require.Nil(t, apiErr)
	require.IsType(t, &dto.Usage{}, usage)
	assert.Equal(t, &dto.Usage{}, usage)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "https://cdn.example.com/image.png", imageResponse.Data[0].Url)
}

func TestDoResponseAcceptsPublishedProModel(t *testing.T) {
	c, recorder := moxingImageContext(context.Background())
	info := moxingImageInfo("https://example.com")
	info.UpstreamModelName = constant.MoxingImageProviderModelSeedream5Pro
	response := testHTTPResponse(http.StatusOK, `{
		"model":"doubao-seedream-5-0-pro-260628",
		"data":[{"url":"https://cdn.example.com/pro.png","size":"2048x2048"}]
	}`)

	usage, apiErr := (&Adaptor{}).DoResponse(c, response, info)

	require.Nil(t, apiErr)
	assert.Equal(t, &dto.Usage{}, usage)
	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestModelListContainsEveryPublishedMoxingModel(t *testing.T) {
	want := []string{
		constant.MoxingImageProviderModelSeedream5Lite,
		constant.MoxingImageProviderModelSeedream5Pro,
	}
	models := (&Adaptor{}).GetModelList()
	assert.Equal(t, want, models)

	models[0] = "mutated-by-caller"
	assert.Equal(t, want, (&Adaptor{}).GetModelList())
}

func TestDoResponseRejectsProviderErrorsAndInvalidResults(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantStatus int
	}{
		{
			name:       "invalid request",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"code":"invalid_request_error","message":"secret provider detail"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid key",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"code":"invalid_api_key","message":"secret provider detail"}}`,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "rate limited without retry",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"code":"rate_limit_exceeded","message":"secret provider detail"}}`,
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name:       "empty data",
			statusCode: http.StatusOK,
			body:       `{"data":[]}`,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "multiple images",
			statusCode: http.StatusOK,
			body:       `{"data":[{"url":"https://a.example/image.png"},{"url":"https://b.example/image.png"}]}`,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "base64 result",
			statusCode: http.StatusOK,
			body:       `{"data":[{"b64_json":"secret-image-data"}]}`,
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := moxingImageContext(context.Background())
			_, apiErr := (&Adaptor{}).DoResponse(c, testHTTPResponse(test.statusCode, test.body), moxingImageInfo("https://example.com"))
			require.NotNil(t, apiErr)
			assert.Equal(t, test.wantStatus, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.NotContains(t, apiErr.Error(), "secret provider detail")
			assert.NotContains(t, apiErr.Error(), "secret-image-data")
		})
	}
}

func TestDoRequestUsesFixedTimeoutWithoutGlobalConfiguration(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	defer func() { common.RelayTimeout = originalRelayTimeout }()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		require.True(t, ok)
		remaining := time.Until(deadline)
		assert.Greater(t, remaining, 9*time.Minute)
		assert.LessOrEqual(t, remaining, 10*time.Minute)
		return testHTTPResponse(http.StatusOK, `{"data":[{"url":"https://cdn.example.com/image.png"}]}`), nil
	})}
	c, _ := moxingImageContext(context.Background())
	response, err := (&Adaptor{client: client}).DoRequest(c, moxingImageInfo("https://example.com"), strings.NewReader(`{}`))
	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestDoRequestTreatsCanceledPOSTAsUnknownAndDoesNotRetry(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.RelayTimeout = 60
	common.AutomaticDisableChannelEnabled = true
	defer func() {
		common.RelayTimeout = originalRelayTimeout
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
	}()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})}
	c, _ := moxingImageContext(context.Background())

	_, err := (&Adaptor{client: client}).DoRequest(c, moxingImageInfo("https://example.com"), strings.NewReader(`{}`))

	apiErr, ok := err.(*types.NewAPIError)
	require.True(t, ok)
	assert.Equal(t, 499, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCode("request_canceled"), apiErr.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(apiErr))
	assert.False(t, types.IsChannelError(apiErr))
	assert.False(t, types.IsRecordErrorLog(apiErr))
	assert.False(t, service.ShouldDisableChannel(apiErr))
}

func TestDoResponseMapsResponseBodyDeadlineToGatewayTimeout(t *testing.T) {
	c, _ := moxingImageContext(context.Background())
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(deadlineReader{}),
	}

	_, apiErr := (&Adaptor{}).DoResponse(c, response, moxingImageInfo("https://example.com"))

	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusGatewayTimeout, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
	assert.True(t, types.IsChannelError(apiErr))
}

type deadlineReader struct{}

func (deadlineReader) Read([]byte) (int, error) {
	return 0, context.DeadlineExceeded
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func testHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
