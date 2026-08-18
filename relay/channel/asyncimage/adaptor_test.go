package asyncimage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newImageRequestContext(ctx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil).WithContext(ctx)
	return c, recorder
}

func asyncImageInfo(baseURL, model string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    baseURL,
			ApiKey:            "test-key",
			UpstreamModelName: model,
			ChannelSetting:    dto.ChannelSettings{},
		},
	}
}

func requireBadRequest(t *testing.T, err error) {
	t.Helper()
	apiErr, ok := err.(*types.NewAPIError)
	require.True(t, ok, "expected NewAPIError, got %T", err)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
}

func TestConvertImageRequestSupportsAllPublishedModels(t *testing.T) {
	n := uint(1)
	cases := []struct {
		name    string
		request dto.ImageRequest
		want    imagePayload
	}{
		{
			name: "nano banana lite",
			request: dto.ImageRequest{
				Model: "nano-banana-2-lite", Prompt: "a cat", N: &n,
				ExtraFields: json.RawMessage(`{"aspect_ratio":"16:9"}`),
			},
			want: imagePayload{Prompt: "a cat", AspectRatio: "16:9"},
		},
		{
			name: "nano banana",
			request: dto.ImageRequest{
				Model: "nano-banana-2", Prompt: "a dog", Size: "1K", OutputFormat: json.RawMessage(`"png"`),
				ExtraFields: json.RawMessage(`{"resolution":"1K"}`),
			},
			want: imagePayload{Prompt: "a dog", Resolution: "1K", OutputFormat: "png"},
		},
		{
			name: "seedream lite",
			request: dto.ImageRequest{
				Model: "seedream-5.0-lite", Prompt: "一只猫", Size: "2K", Quality: "basic",
				ExtraFields: json.RawMessage(`{"aspect_ratio":"3:2"}`),
			},
			want: imagePayload{Prompt: "一只猫", GenType: "t2i", AspectRatio: "3:2", Quality: "basic"},
		},
		{
			name:    "seedream pro",
			request: dto.ImageRequest{Model: "seedream-5.0-pro", Prompt: "a landscape", Size: "1K", Quality: "basic"},
			want:    imagePayload{Prompt: "a landscape", GenType: "t2i", Quality: "basic"},
		},
	}

	adaptor := &Adaptor{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := asyncImageInfo("https://example.com", tc.request.Model)
			got, err := adaptor.ConvertImageRequest(nil, info, tc.request)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestConvertImageRequestRejectsUnsupportedContract(t *testing.T) {
	n := uint(2)
	cases := []dto.ImageRequest{
		{Model: nanoBanana2, Prompt: "x", N: &n},
		{Model: nanoBanana2, Prompt: "x", ResponseFormat: "b64_json"},
		{Model: nanoBanana2, Prompt: "x", Extra: map[string]json.RawMessage{"callbackUrl": json.RawMessage(`"https://example.com/cb"`)}},
		{Model: nanoBanana2, Prompt: "x", ExtraFields: json.RawMessage(`{"unknown":true}`)},
		{Model: nanoBanana2, Prompt: "x", OutputFormat: json.RawMessage(`null`)},
		{Model: nanoBanana2, Prompt: "x", OutputFormat: json.RawMessage(`"png"`), ExtraFields: json.RawMessage(`{"output_format":"jpg"}`)},
		{Model: seedream5Pro, Prompt: "x", Size: "1K", Quality: "basic", OutputFormat: json.RawMessage(`"png"`)},
		{Model: nanoBanana2, Prompt: "x", ExtraFields: json.RawMessage(`{"reference_images":null}`)},
		{Model: nanoBanana2, Prompt: "x", ExtraFields: json.RawMessage(`{"reference_images":["ftp://example.com/a.png"]}`)},
		{Model: nanoBanana2, Prompt: "x", ExtraFields: json.RawMessage(`{"reference_images":["https://example.com/a.png"],"resolution":"2K"}`), Size: "1K"},
		{Model: seedream5Lite, Prompt: "x", Size: "3K", Quality: "high"},
		{Model: seedream5Pro, Prompt: "x", Size: "2K", Quality: "high"},
		{Model: seedream5Lite, Prompt: "x", ExtraFields: json.RawMessage(`{"reference_images":["https://example.com/a.png"]}`)},
	}

	adaptor := &Adaptor{}
	for _, request := range cases {
		_, err := adaptor.ConvertImageRequest(nil, asyncImageInfo("https://example.com", request.Model), request)
		requireBadRequest(t, err)
	}
}

func TestDoRequestAndDoResponsePollsProcessingTask(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 60
	defer func() { common.RelayTimeout = originalRelayTimeout }()

	var postCount atomic.Int32
	var getCount atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer test-key", request.Header.Get("Authorization"))
		switch request.Method {
		case http.MethodPost:
			postCount.Add(1)
			body, readErr := io.ReadAll(request.Body)
			assert.NoError(t, readErr)
			assert.Contains(t, string(body), `"prompt":"a cat"`)
			return testHTTPResponse(http.StatusOK, `{"code":0,"data":{"taskId":"task-1","status":"processing"}}`), nil
		case http.MethodGet:
			getCount.Add(1)
			return testHTTPResponse(http.StatusOK, `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/result.png"]}}`), nil
		default:
			return testHTTPResponse(http.StatusMethodNotAllowed, ""), nil
		}
	})}
	c, recorder := newImageRequestContext(context.Background())
	info := asyncImageInfo("https://example.com", nanoBanana2)
	adaptor := &Adaptor{client: client}
	resp, err := adaptor.DoRequest(c, info, bytes.NewBufferString(`{"prompt":"a cat"}`))
	require.NoError(t, err)
	usage, apiErr := adaptor.DoResponse(c, resp.(*http.Response), info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, int32(1), postCount.Load())
	assert.Equal(t, int32(1), getCount.Load())
	assert.Equal(t, http.StatusOK, recorder.Code)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "https://cdn.example.com/result.png", imageResponse.Data[0].Url)
}

func TestDoRequestSetsContentLengthAndHostOverride(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 60
	defer func() { common.RelayTimeout = originalRelayTimeout }()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, int64(13), request.ContentLength)
		assert.Equal(t, "provider.internal", request.Host)
		assert.Equal(t, "override", request.Header.Get("X-Provider-Mode"))
		return testHTTPResponse(http.StatusOK, `{"code":0,"data":{"status":"success","result":["https://cdn.example.com/result.png"]}}`), nil
	})}
	c, _ := newImageRequestContext(context.Background())
	info := asyncImageInfo("https://example.com", nanoBanana2)
	info.UpstreamRequestBodySize = 13
	info.ChannelMeta.HeadersOverride = map[string]any{
		"Host":            "provider.internal",
		"X-Provider-Mode": "override",
	}
	resp, err := (&Adaptor{client: client}).DoRequest(c, info, strings.NewReader(`{"prompt":"x"}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestDoRequestKeepsProviderEnvelopeForHTTPErrorMapping(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 60
	defer func() { common.RelayTimeout = originalRelayTimeout }()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return testHTTPResponse(http.StatusBadRequest, `{"code":10002,"msg":"provider detail"}`), nil
	})}
	c, _ := newImageRequestContext(context.Background())
	info := asyncImageInfo("https://example.com", nanoBanana2)
	resp, err := (&Adaptor{client: client}).DoRequest(c, info, strings.NewReader(`{"prompt":"x"}`))
	require.NoError(t, err)
	_, apiErr := (&Adaptor{client: client}).DoResponse(c, resp.(*http.Response), info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.NotContains(t, apiErr.Error(), "provider detail")
}

func TestDoRequestRejectsInfiniteTimeoutConfiguration(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 0
	defer func() { common.RelayTimeout = originalRelayTimeout }()

	c, _ := newImageRequestContext(context.Background())
	info := asyncImageInfo("https://example.com", nanoBanana2)
	_, err := (&Adaptor{client: &http.Client{}}).DoRequest(c, info, strings.NewReader(`{"prompt":"x"}`))
	apiErr, ok := err.(*types.NewAPIError)
	require.True(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDoResponseRejectsProviderErrorsAndInvalidResults(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		statusCode int
	}{
		{name: "parameter error", body: `{"code":10002,"msg":"secret provider detail"}`, statusCode: http.StatusBadRequest},
		{name: "auth error", body: `{"code":10005,"msg":"secret provider detail"}`, statusCode: http.StatusBadGateway},
		{name: "missing task", body: `{"code":30003,"msg":"secret provider detail"}`, statusCode: http.StatusBadGateway},
		{name: "insufficient balance", body: `{"code":40001,"msg":"secret provider detail"}`, statusCode: http.StatusBadGateway},
		{name: "provider failure", body: `{"code":90003,"msg":"secret provider detail"}`, statusCode: http.StatusBadGateway},
		{name: "empty task id", body: `{"code":0,"data":{"status":"processing"}}`, statusCode: http.StatusBadGateway},
		{name: "unknown status", body: `{"code":0,"data":{"status":"running"}}`, statusCode: http.StatusBadGateway},
		{name: "empty result", body: `{"code":0,"data":{"status":"success","result":[]}}`, statusCode: http.StatusBadGateway},
		{name: "multiple results", body: `{"code":0,"data":{"status":"success","result":["https://a","https://b"]}}`, statusCode: http.StatusBadGateway},
	}

	adaptor := &Adaptor{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newImageRequestContext(context.Background())
			info := asyncImageInfo("https://example.com", nanoBanana2)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body))}
			_, apiErr := adaptor.DoResponse(c, resp, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, tc.statusCode, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
			assert.NotContains(t, apiErr.Error(), "secret provider detail")
		})
	}
}

func TestDoResponseHonorsCanceledContext(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 60
	defer func() { common.RelayTimeout = originalRelayTimeout }()

	ctx, cancel := context.WithCancel(context.Background())
	c, _ := newImageRequestContext(ctx)
	cancel()
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}
	info := asyncImageInfo("https://example.com", nanoBanana2)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"taskId":"task-1","status":"processing"}}`))}
	_, apiErr := (&Adaptor{client: client}).DoResponse(c, resp, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusGatewayTimeout, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
}
