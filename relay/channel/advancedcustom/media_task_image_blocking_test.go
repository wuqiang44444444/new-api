package advancedcustom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertMediaTaskImageRequestSupportsDocumentedModels(t *testing.T) {
	tests := []struct {
		name    string
		request dto.ImageRequest
		check   func(*testing.T, *mediaTaskImageRequest)
	}{
		{
			name: "Gemini usage",
			request: dto.ImageRequest{
				Model:          "gemini-3.1-flash-image-preview-usage",
				Prompt:         "make this cinematic",
				N:              uintPointer(1),
				Size:           "4K",
				ResponseFormat: "url",
				Image:          rawJSON(t, []string{"https://cdn.example/ref.png"}),
				Extra: map[string]json.RawMessage{
					"capability":   rawJSON(t, mediaTaskImageCapability),
					"aspect_ratio": rawJSON(t, "16:9"),
					"callback_url": rawJSON(t, "https://callback.invalid"),
				},
			},
			check: func(t *testing.T, converted *mediaTaskImageRequest) {
				assert.Equal(t, "4K", converted.Size)
				assert.Equal(t, "16:9", converted.AspectRatio)
				assert.Equal(t, []string{"https://cdn.example/ref.png"}, converted.ReferenceImages)
				assert.Empty(t, converted.Image)
			},
		},
		{
			name: "Seedream 4.5",
			request: dto.ImageRequest{
				Model:  "doubao-seedream-4-5-251128",
				Prompt: "edit the source",
				N:      uintPointer(4),
				Size:   "2048x2048",
				Image:  rawJSON(t, "https://cdn.example/source.png"),
			},
			check: func(t *testing.T, converted *mediaTaskImageRequest) {
				assert.JSONEq(t, `"https://cdn.example/source.png"`, string(converted.Image))
			},
		},
		{
			name: "Seedream 5.0",
			request: dto.ImageRequest{
				Model:  "seedream-5-0-260128",
				Prompt: "three related scenes",
				N:      uintPointer(3),
				Size:   "2K",
				Images: rawJSON(t, []string{"https://cdn.example/one.png", "https://cdn.example/two.png"}),
				Extra: map[string]json.RawMessage{
					"extra": rawJSON(t, map[string]any{
						"sequential_image_generation":         "auto",
						"sequential_image_generation_options": map[string]any{"max_images": 3},
						"watermark":                           false,
					}),
				},
			},
			check: func(t *testing.T, converted *mediaTaskImageRequest) {
				require.NotNil(t, converted.Extra)
				assert.Equal(t, "auto", converted.Extra.SequentialImageGeneration)
				require.NotNil(t, converted.Extra.SequentialImageGenerationOptions)
				assert.Equal(t, uint(3), *converted.Extra.SequentialImageGenerationOptions.MaxImages)
				assert.JSONEq(t, `["https://cdn.example/one.png","https://cdn.example/two.png"]`, string(converted.Image))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			converted, err := convertMediaTaskImageRequest(test.request, test.request.Model)
			require.NoError(t, err)
			assert.Equal(t, mediaTaskImageCapability, converted.Capability)
			assert.Equal(t, "url", converted.ResponseFormat)
			test.check(t, converted)

			body, err := common.Marshal(converted)
			require.NoError(t, err)
			assert.NotContains(t, string(body), "callback_url")
			assert.NotContains(t, string(body), `"images"`)
			if test.name == "Gemini usage" {
				assert.Contains(t, string(body), `"reference_images"`)
				assert.NotContains(t, string(body), `"image"`)
			}
		})
	}
}

func TestConvertMediaTaskImageRequestRejectsUnsafeCountsAndFields(t *testing.T) {
	tests := []struct {
		name    string
		request dto.ImageRequest
		want    string
	}{
		{
			name: "multi image maximum exceeds precharged n",
			request: dto.ImageRequest{
				Model:  "seedream-5-0-260128",
				Prompt: "three scenes",
				N:      uintPointer(1),
				Size:   "2K",
				Extra: map[string]json.RawMessage{
					"extra": rawJSON(t, map[string]any{
						"sequential_image_generation":         "auto",
						"sequential_image_generation_options": map[string]any{"max_images": 3},
					}),
				},
			},
			want: "n must be greater than or equal",
		},
		{
			name: "too many Gemini references",
			request: dto.ImageRequest{
				Model:  "gemini-3.1-flash-image-preview-usage",
				Prompt: "combine",
				N:      uintPointer(1),
				Size:   "1K",
				Image:  rawJSON(t, repeatImageURLs(mediaTaskImageMaxGeminiImages+1)),
			},
			want: "image must not contain more than",
		},
		{
			name: "unknown behavior changing field",
			request: dto.ImageRequest{
				Model:  "gemini-3.1-flash-image-preview-usage",
				Prompt: "a dog",
				N:      uintPointer(1),
				Size:   "1K",
				Extra:  map[string]json.RawMessage{"unsafe_mode": rawJSON(t, true)},
			},
			want: `unsupported image field "unsafe_mode"`,
		},
		{
			name: "known unsupported edit field",
			request: dto.ImageRequest{
				Model:  "seedream-5-0-260128",
				Prompt: "edit",
				N:      uintPointer(1),
				Size:   "2K",
				Mask:   rawJSON(t, "https://cdn.example/mask.png"),
			},
			want: "mask is not supported",
		},
		{
			name: "multi image maximum exceeds global bound",
			request: dto.ImageRequest{
				Model:  "seedream-5-0-260128",
				Prompt: "many scenes",
				N:      uintPointer(dto.MaxImageN),
				Size:   "2K",
				Extra: map[string]json.RawMessage{
					"extra": rawJSON(t, map[string]any{
						"sequential_image_generation":         "auto",
						"sequential_image_generation_options": map[string]any{"max_images": dto.MaxImageN + 1},
					}),
				},
			},
			want: "max_images must be between",
		},
		{
			name: "non URL output format",
			request: dto.ImageRequest{
				Model:          "gemini-3.1-flash-image-preview-usage",
				Prompt:         "a dog",
				N:              uintPointer(1),
				Size:           "1K",
				ResponseFormat: "b64_json",
			},
			want: "response_format must be url",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := convertMediaTaskImageRequest(test.request, test.request.Model)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)

			var apiErr *kittypes.NewAPIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, kittypes.IsSkipRetryError(apiErr))
		})
	}
}

func TestConvertMediaTaskImageRequestRejectsFixedPriceGeminiModel(t *testing.T) {
	tests := []struct {
		name        string
		request     dto.ImageRequest
		originModel string
	}{
		{
			name: "direct model",
			request: dto.ImageRequest{
				Model:  "Gemini-3.1-Flash-Image-Preview",
				Prompt: "a dog",
				N:      uintPointer(1),
				Size:   "1K",
			},
			originModel: "Gemini-3.1-Flash-Image-Preview",
		},
		{
			name: "public alias mapped to fixed price model",
			request: dto.ImageRequest{
				Model:  "Gemini-3.1-Flash-Image-Preview",
				Prompt: "a dog",
				N:      uintPointer(1),
				Size:   "1K",
			},
			originModel: "public-gemini-alias",
		},
		{
			name: "fixed price public model mapped to provider alias",
			request: dto.ImageRequest{
				Model:  "provider-gemini-alias",
				Prompt: "a dog",
				N:      uintPointer(1),
				Size:   "1K",
			},
			originModel: "Gemini-3.1-Flash-Image-Preview",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := convertMediaTaskImageRequest(test.request, test.originModel)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "fixed per-size pricing is disabled")
			var apiErr *kittypes.NewAPIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, kittypes.IsSkipRetryError(apiErr))
		})
	}
}

func TestConvertMediaTaskImageRequestKeepsOriginModelRulesAfterMapping(t *testing.T) {
	_, err := convertMediaTaskImageRequest(dto.ImageRequest{
		Model:  "provider-seedream-alias",
		Prompt: "too many images",
		N:      uintPointer(5),
		Size:   "2048x2048",
	}, "doubao-seedream-4-5-251128")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "n must be between 1 and 4")
}

func TestConvertMediaTaskImageRequestUsesMappedKnownModelForPublicAlias(t *testing.T) {
	_, err := convertMediaTaskImageRequest(dto.ImageRequest{
		Model:  "seedream-5-0-260128",
		Prompt: "invalid size",
		N:      uintPointer(1),
		Size:   "1024x1024",
	}, "seedream-5-moxing")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "size must be 2K or 3K")
}

func TestConvertMediaTaskImageRequestKeepsLegacyReferenceImagesTemporarily(t *testing.T) {
	converted, err := convertMediaTaskImageRequest(dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "restyle the reference",
		N:      uintPointer(1),
		Size:   "1K",
		Extra: map[string]json.RawMessage{
			"reference_images": rawJSON(t, []string{"https://cdn.example/legacy.png"}),
		},
	}, "nano-banana-2")

	require.NoError(t, err)
	assert.Equal(t, []string{"https://cdn.example/legacy.png"}, converted.ReferenceImages)
}

func TestConvertMediaTaskImageRequestRejectsStandardAndLegacyImageConflict(t *testing.T) {
	_, err := convertMediaTaskImageRequest(dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "restyle the reference",
		N:      uintPointer(1),
		Size:   "1K",
		Image:  rawJSON(t, "https://cdn.example/standard.png"),
		Extra: map[string]json.RawMessage{
			"reference_images": rawJSON(t, []string{"https://cdn.example/legacy.png"}),
		},
	}, "nano-banana-2")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "image and reference_images cannot be used together")
	var apiErr *kittypes.NewAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.True(t, kittypes.IsSkipRetryError(apiErr))
}

func TestMediaTaskImageBlockingPollsOncePerTaskAndSynthesizesOpenAIResponse(t *testing.T) {
	service.InitHttpClient()
	var creates atomic.Int32
	var queries atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer sk-test", request.Header.Get("Authorization"))
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/images/generations":
			creates.Add(1)
			body, err := io.ReadAll(request.Body)
			require.NoError(t, err)
			var upstreamRequest mediaTaskImageRequest
			require.NoError(t, common.Unmarshal(body, &upstreamRequest))
			assert.Equal(t, mediaTaskImageCapability, upstreamRequest.Capability)
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"task_id":"task-1","request_id":"create-body-1","poll_url":"https://attacker.invalid/task-1"}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/media/tasks/task-1":
			query := queries.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			if query == 1 {
				writer.Header().Set("X-Request-Id", "poll-header-1")
				_, _ = writer.Write([]byte(`{"data":{"task_id":"task-1","status":"queued"}}`))
				return
			}
			_, _ = writer.Write([]byte(`{"data":{"task_id":"task-1","request_id":"poll-body-2","status":"succeeded","result":{"primary_url":"https://cdn.example/one.png","urls":["https://cdn.example/one.png","https://cdn.example/two.png"]}}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adaptor, info, c := mediaTaskImageTestAdaptor(server.URL)
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "a dog",
		N:      uintPointer(1),
		Size:   "1K",
	})
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	info.UpstreamRequestBodySize = int64(len(body))

	responseAny, err := adaptor.DoRequest(c, info, bytes.NewReader(body))
	require.NoError(t, err)
	response := responseAny.(*http.Response)
	assert.Equal(t, http.StatusOK, response.StatusCode)

	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(responseBody, &imageResponse))
	require.Len(t, imageResponse.Data, 2)
	assert.Equal(t, "https://cdn.example/one.png", imageResponse.Data[0].Url)
	assert.Equal(t, "https://cdn.example/two.png", imageResponse.Data[1].Url)
	assert.Equal(t, int32(1), creates.Load())
	assert.Equal(t, int32(2), queries.Load())
	trace, ok := relaycommon.GetUpstreamTaskTrace(c)
	require.True(t, ok)
	assert.Equal(t, "task-1", trace.TaskID)
	assert.Equal(t, "create-body-1", trace.CreateRequestID)
	assert.Equal(t, "poll-body-2", trace.LastPollRequestID)
	assert.Equal(t, 2, trace.PollAttempts)
	assert.GreaterOrEqual(t, trace.PollElapsedMilliseconds, int64(0))
	assert.Equal(t, "poll-body-2", c.GetString(common.UpstreamRequestIdKey))
}

func TestMediaTaskImageBlockingPassesThroughDirectSuccess(t *testing.T) {
	service.InitHttpClient()
	var creates atomic.Int32
	var queries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			creates.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"created":123,"data":[{"url":"https://cdn.example/direct.png"}]}`))
		case http.MethodGet:
			queries.Add(1)
			http.NotFound(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adaptor, info, c := mediaTaskImageTestAdaptor(server.URL)
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "a dog",
		N:      uintPointer(1),
		Size:   "1K",
	})
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)

	responseAny, err := adaptor.DoRequest(c, info, bytes.NewReader(body))
	require.NoError(t, err)
	response := responseAny.(*http.Response)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(responseBody, &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "https://cdn.example/direct.png", imageResponse.Data[0].Url)
	assert.Equal(t, int32(1), creates.Load())
	assert.Equal(t, int32(0), queries.Load())
	_, traced := relaycommon.GetUpstreamTaskTrace(c)
	assert.False(t, traced)
}

func TestMediaTaskImageBlockingRecognizesHTTP200QueuedTask(t *testing.T) {
	service.InitHttpClient()
	var creates atomic.Int32
	var queries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			creates.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"code":200,"object":"media.task","task_id":"task-http-200","status":"queued"}`))
		case http.MethodGet:
			queries.Add(1)
			_, _ = writer.Write([]byte(`{"task_id":"task-http-200","status":"succeeded","result":{"primary_url":"https://cdn.example/http-200.png"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adaptor, info, c := mediaTaskImageTestAdaptor(server.URL)
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "a dog",
		N:      uintPointer(1),
		Size:   "1K",
	})
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)

	responseAny, err := adaptor.DoRequest(c, info, bytes.NewReader(body))
	require.NoError(t, err)
	response := responseAny.(*http.Response)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(responseBody, &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "https://cdn.example/http-200.png", imageResponse.Data[0].Url)
	assert.Equal(t, int32(1), creates.Load())
	assert.Equal(t, int32(1), queries.Load())
	trace, ok := relaycommon.GetUpstreamTaskTrace(c)
	require.True(t, ok)
	assert.Equal(t, "task-http-200", trace.TaskID)
}

func TestMediaTaskImageBlockingNormalizesHTTP200CompletedTask(t *testing.T) {
	service.InitHttpClient()
	var creates atomic.Int32
	var queries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodPost:
			creates.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"object":"media.task","task_id":"task-http-200-completed","status":"completed","result":{"primary_url":"https://cdn.example/completed.png"}}`))
		case http.MethodGet:
			queries.Add(1)
			http.NotFound(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adaptor, info, c := mediaTaskImageTestAdaptor(server.URL)
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "a dog",
		N:      uintPointer(1),
		Size:   "1K",
	})
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)

	responseAny, err := adaptor.DoRequest(c, info, bytes.NewReader(body))
	require.NoError(t, err)
	response := responseAny.(*http.Response)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(responseBody, &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "https://cdn.example/completed.png", imageResponse.Data[0].Url)
	assert.Equal(t, int32(1), creates.Load())
	assert.Equal(t, int32(0), queries.Load())
}

func TestMediaTaskImageBlockingFailedTaskIsNotRetried(t *testing.T) {
	service.InitHttpClient()
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			creates.Add(1)
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"data":{"task_id":"task-failed"}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"status":"failed","error_message":"provider rejected the prompt"}`))
	}))
	defer server.Close()

	adaptor, info, c := mediaTaskImageTestAdaptor(server.URL)
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "a dog",
		N:      uintPointer(1),
		Size:   "1K",
	})
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)

	_, err = adaptor.DoRequest(c, info, bytes.NewReader(body))
	require.Error(t, err)
	var apiErr *kittypes.NewAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.True(t, kittypes.IsSkipRetryError(apiErr))
	assert.Contains(t, apiErr.Error(), "provider rejected the prompt")
	assert.Equal(t, int32(1), creates.Load())
}

func TestMediaTaskImageBlockingReusesOpenAIImageSettlementCount(t *testing.T) {
	adaptor, info, c := mediaTaskImageTestAdaptor("https://upstream.example")
	info.PriceData = types.PriceData{UsePrice: true}
	info.PriceData.AddOtherRatio("n", 1)
	response, err := mediaTaskImageSuccessResponse(&mediaTaskImageResult{
		URLs: []string{
			"https://cdn.example/one.png",
			"https://cdn.example/two.png",
		},
	})
	require.NoError(t, err)

	usage, apiErr := adaptor.DoResponse(c, response, info)

	require.Nil(t, apiErr)
	require.IsType(t, &dto.Usage{}, usage)
	assert.Equal(t, float64(2), info.PriceData.OtherRatios()["n"])
}

func TestMediaTaskImageSynchronousResponseRejectsProviderOverdelivery(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"created":1,"data":[{"url":"https://cdn.example/one.png"},{"url":"https://cdn.example/two.png"}]}`,
		)),
	}

	err := validateMediaTaskImageSynchronousCount(response, 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requested n=1")
	require.NotNil(t, response.Body)
	body, readErr := io.ReadAll(response.Body)
	require.NoError(t, readErr)
	assert.JSONEq(t,
		`{"created":1,"data":[{"url":"https://cdn.example/one.png"},{"url":"https://cdn.example/two.png"}]}`,
		string(body),
	)
}

func TestMediaTaskImageSynchronousResponseRequiresOpenAIDataArray(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"data":`},
		{name: "missing data", body: `{}`},
		{name: "null data", body: `{"data":null}`},
		{name: "object data", body: `{"data":{"url":"https://cdn.example/one.png"}}`},
		{name: "empty data", body: `{"data":[]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(test.body)),
			}

			err := validateMediaTaskImageSynchronousCount(response, 1)

			require.Error(t, err)
			require.NotNil(t, response.Body)
			body, readErr := io.ReadAll(response.Body)
			require.NoError(t, readErr)
			assert.Equal(t, test.body, string(body))
		})
	}
}

func TestMediaTaskImageSynchronousResponseAcceptsRequestedImageCount(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"created":1,"data":[{"url":"https://cdn.example/one.png"}]}`,
		)),
	}

	require.NoError(t, validateMediaTaskImageSynchronousCount(response, 1))
}

func TestPersistentMediaTaskImageRejectsUnfrozenAuthenticationInputsBeforeCreate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Adaptor, *relaycommon.RelayInfo)
	}{
		{
			name: "header override",
			mutate: func(_ *Adaptor, info *relaycommon.RelayInfo) {
				info.HeadersOverride = map[string]interface{}{"X-Provider-Secret": "literal-secret"}
			},
		},
		{
			name: "literal route secret",
			mutate: func(adaptor *Adaptor, _ *relaycommon.RelayInfo) {
				adaptor.route.Auth.Value = "Bearer literal-secret"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adaptor, info, c := mediaTaskImageTestAdaptor("https://upstream.example")
			converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
				Model: "gemini-3.1-flash-image-preview-usage", Prompt: "a dog", N: uintPointer(1), Size: "1K",
			})
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			info.IsChannelTest = false
			test.mutate(adaptor, info)

			_, err = adaptor.DoRequest(c, info, bytes.NewReader(body))

			require.Error(t, err)
			assert.Contains(t, err.Error(), "persistent media image")
		})
	}
}

func TestMediaTaskImageTaskIDUsesAllowlist(t *testing.T) {
	tests := []string{
		"../../admin",
		"task/child",
		"task%2Fchild",
		"task id",
		"任务-1",
	}
	for _, taskID := range tests {
		t.Run(taskID, func(t *testing.T) {
			response := &http.Response{
				Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"task_id":%q}`, taskID))),
			}

			_, _, err := mediaTaskImageTaskID(response)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsafe characters")
		})
	}

	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(`{"task_id":"task_01:part-2.~"}`)),
	}
	taskID, _, err := mediaTaskImageTaskID(response)
	require.NoError(t, err)
	assert.Equal(t, "task_01:part-2.~", taskID)
}

func TestMediaTaskImageWaitStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForMediaTaskImagePoll(ctx, mediaTaskImageInitialPollDelay, &relaycommon.RelayInfo{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestMediaTaskImageBlockingTimeoutDoesNotCreateAnotherTask(t *testing.T) {
	service.InitHttpClient()
	var creates atomic.Int32
	var queries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			creates.Add(1)
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"task_id":"task-timeout"}`))
			return
		}
		queries.Add(1)
		_, _ = writer.Write([]byte(`{"status":"running"}`))
	}))
	defer server.Close()

	adaptor, info, c := mediaTaskImageTestAdaptor(server.URL)
	info.ChannelOtherSettings.DisableTaskPollingSleep = false
	ctx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3.1-flash-image-preview-usage",
		Prompt: "a dog",
		N:      uintPointer(1),
		Size:   "1K",
	})
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)

	_, err = adaptor.DoRequest(c, info, bytes.NewReader(body))
	require.Error(t, err)
	var apiErr *kittypes.NewAPIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusGatewayTimeout, apiErr.StatusCode)
	assert.True(t, kittypes.IsSkipRetryError(apiErr))
	assert.Equal(t, int32(1), creates.Load())
	assert.Equal(t, int32(0), queries.Load())
}

func mediaTaskImageTestAdaptor(baseURL string) (*Adaptor, *relaycommon.RelayInfo, *gin.Context) {
	config := &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{
			{
				IncomingPath: "/v1/images/generations",
				UpstreamPath: "/v1/images/generations",
				Converter:    dto.AdvancedCustomConverterMediaTaskImageBlocking,
				Auth: &dto.AdvancedCustomRouteAuth{
					Type:  dto.AdvancedCustomAuthTypeHeader,
					Name:  "Authorization",
					Value: "Bearer {api_key}",
				},
			},
		},
	}
	info := advancedCustomRelayInfo(config)
	info.RelayFormat = kittypes.RelayFormatOpenAIImage
	info.RelayMode = relayconstant.RelayModeImagesGenerations
	info.RequestURLPath = "/v1/images/generations"
	info.OriginModelName = "gemini-3.1-flash-image-preview-usage"
	info.UpstreamModelName = info.OriginModelName
	info.ChannelBaseUrl = baseURL
	info.IsChannelTest = true
	info.ChannelOtherSettings.DisableTaskPollingSleep = true
	c := advancedCustomGinContext("/v1/images/generations")
	return &Adaptor{}, info, c
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	body, err := common.Marshal(value)
	require.NoError(t, err)
	return body
}

func uintPointer(value uint) *uint {
	return &value
}

func repeatImageURLs(count int) []string {
	values := make([]string, 0, count)
	for index := 0; index < count; index++ {
		values = append(values, fmt.Sprintf("https://cdn.example/%d.png", index))
	}
	return values
}
