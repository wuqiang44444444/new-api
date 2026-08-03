package jsonvideo

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRequestExactContracts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "text defaults",
			input: `{"model":"upstream-model","content":[{"type":"text","text":"hello"}]}`,
			want:  `{"model":"upstream-model","prompt":"hello","duration":5,"ratio":"16:9","reference_mode":"text_to_video"}`,
		},
		{
			name: "omni images and audio",
			input: `{"model":"upstream-model","duration":8,"ratio":"1:1","content":[
				{"type":"text","text":"line one"},{"type":"text","text":"line two"},
				{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/a.png"}},
				{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://example.com/a.mp3"}}]}`,
			want: `{"model":"upstream-model","prompt":"line one\nline two","duration":8,"ratio":"1:1","reference_mode":"omni",
				"input_images":["https://example.com/a.png"],"audio_url_list":["https://example.com/a.mp3"]}`,
		},
		{
			name: "first frame",
			input: `{"model":"upstream-model","content":[
				{"type":"text","text":"animate"},
				{"type":"image_url","role":"first_frame","image_url":{"url":"https://example.com/first.png"}}]}`,
			want: `{"model":"upstream-model","prompt":"animate","duration":5,"ratio":"16:9","reference_mode":"first_frame",
				"input_images":["https://example.com/first.png"]}`,
		},
		{
			name: "both frames preserve order",
			input: `{"model":"upstream-model","content":[
				{"type":"text","text":"animate"},
				{"type":"image_url","role":"first_frame","image_url":{"url":"https://example.com/first.png"}},
				{"type":"image_url","role":"last_frame","image_url":{"url":"https://example.com/last.png"}}]}`,
			want: `{"model":"upstream-model","prompt":"animate","duration":5,"ratio":"16:9","reference_mode":"both_frames",
				"input_images":["https://example.com/first.png","https://example.com/last.png"]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var input dto.ModelArkVideoCreateRequest
			require.NoError(t, common.Unmarshal([]byte(test.input), &input))
			input.Model = model.VideoSKUSeedance20Standard720P
			capability, ok := model.ResolveVideoSKUCapability(input.Model)
			require.True(t, ok)
			got, err := CreateRequest(&input, "upstream-model", capability)
			require.NoError(t, err)
			assert.JSONEq(t, test.want, string(got))
		})
	}
}

func TestCreateRequestRejectsUnsupportedMediaAndMIME(t *testing.T) {
	tests := []string{
		`{"model":"m","content":[{"type":"text","text":"x"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://example.com/v.mp4"}}]}`,
		`{"model":"m","content":[{"type":"text","text":"x"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:audio/mpeg;base64,YQ=="}}]}`,
		`{"model":"m","content":[{"type":"text","text":"x"},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://a"}}]}`,
		`{"model":"m","generate_audio":true,"content":[{"type":"text","text":"x"}]}`,
		`{"model":"m","service_tier":"","content":[{"type":"text","text":"x"}]}`,
	}
	for _, input := range tests {
		var request dto.ModelArkVideoCreateRequest
		require.NoError(t, common.Unmarshal([]byte(input), &request))
		request.Model = model.VideoSKUSeedance20Standard720P
		capability, ok := model.ResolveVideoSKUCapability(request.Model)
		require.True(t, ok)
		_, err := CreateRequest(&request, "upstream-model", capability)
		require.Error(t, err)
	}
}

func TestCreateRequestDurationAndRatioBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		duration *int
		ratio    *string
		wantErr  bool
	}{
		{name: "minimum duration", duration: common.GetPointer(4), ratio: common.GetPointer("16:9")},
		{name: "maximum duration", duration: common.GetPointer(15), ratio: common.GetPointer("21:9")},
		{name: "duration below minimum", duration: common.GetPointer(3), wantErr: true},
		{name: "duration above maximum", duration: common.GetPointer(16), wantErr: true},
		{name: "adaptive ratio rejected", ratio: common.GetPointer("adaptive"), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard720P)
			require.True(t, ok)
			_, err := CreateRequest(&dto.ModelArkVideoCreateRequest{
				Model:    model.VideoSKUSeedance20Standard720P,
				Duration: test.duration,
				Ratio:    test.ratio,
				Content:  []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("hello")}},
			}, "upstream-model", capability)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateRequestMediaCountBoundaries(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model:   model.VideoSKUSeedance20Standard720P,
		Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("hello")}},
	}
	capability, ok := model.ResolveVideoSKUCapability(request.Model)
	require.True(t, ok)
	for index := 0; index < 9; index++ {
		request.Content = append(request.Content, dto.ModelArkVideoContent{
			Type: "image_url", Role: common.GetPointer("reference_image"),
			ImageURL: &dto.VideoMediaURL{URL: "https://example.com/image.png"},
		})
	}
	for index := 0; index < 3; index++ {
		request.Content = append(request.Content, dto.ModelArkVideoContent{
			Type: "audio_url", Role: common.GetPointer("reference_audio"),
			AudioURL: &dto.VideoMediaURL{URL: "data:audio/mpeg;base64,YQ=="},
		})
	}
	_, err := CreateRequest(request, "upstream-model", capability)
	require.NoError(t, err)

	request.Content = append(request.Content, dto.ModelArkVideoContent{
		Type: "audio_url", Role: common.GetPointer("reference_audio"),
		AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"},
	})
	_, err = CreateRequest(request, "upstream-model", capability)
	require.Error(t, err)
}

func TestTaskResponseRequiresExactStatusAndTaskID(t *testing.T) {
	context := TaskResponseContext{BaseURL: "https://video.example.com"}
	body, err := TaskResponseV2([]byte(`{"id":"upstream-1","status":"completed","video_url":"https://video.example.com/result.mp4"}`), "upstream-1", context)
	require.NoError(t, err)
	var normalized map[string]any
	require.NoError(t, common.Unmarshal(body, &normalized))
	assert.Equal(t, "succeeded", normalized["status"])

	_, err = TaskResponseV2([]byte(`{"id":"other","status":"completed","video_url":"https://video.example.com/result.mp4"}`), "upstream-1", context)
	require.Error(t, err)
	_, err = TaskResponseV2([]byte(`{"id":"upstream-1","status":"COMPLETED","video_url":"https://video.example.com/result.mp4"}`), "upstream-1", context)
	require.Error(t, err)
}

func TestTaskResponseNormalizesOnlyPublishedStatusesAndIgnoresMetadataURL(t *testing.T) {
	tests := []struct {
		upstream   string
		normalized string
	}{
		{upstream: "queued", normalized: "queued"},
		{upstream: "in_progress", normalized: "running"},
		{upstream: "processing", normalized: "running"},
		{upstream: "completed", normalized: "succeeded"},
		{upstream: "failed", normalized: "failed"},
	}
	for _, test := range tests {
		t.Run(test.upstream, func(t *testing.T) {
			body, err := TaskResponseV2([]byte(`{
				"id":"upstream-1",
				"status":"`+test.upstream+`",
				"video_url":"https://video.example.com/result.mp4",
				"metadata":{"url":"https://user:secret@evil.example/video?signature=secret"}
			}`), "upstream-1", TaskResponseContext{BaseURL: "https://video.example.com"})
			require.NoError(t, err)
			var normalized map[string]any
			require.NoError(t, common.Unmarshal(body, &normalized))
			assert.Equal(t, test.normalized, normalized["status"])
			assert.NotContains(t, string(body), "evil.example")
			if test.upstream == "completed" {
				assert.Contains(t, string(body), `"video_url":"https://video.example.com/result.mp4"`)
			}
		})
	}
}

func TestTaskResponseV2RequiresValidatedTopLevelVideoURL(t *testing.T) {
	body, err := TaskResponseV2(
		[]byte(`{
			"id":"upstream-1",
			"status":"completed",
			"video_url":"https://video.example.com/v1/a/result.mp4?signature=secret"
		}`),
		"upstream-1",
		TaskResponseContext{BaseURL: "https://video.example.com/v1"},
	)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"status":"succeeded"`)
	assert.Contains(t, string(body), `"video_url":"https://video.example.com/v1/a/result.mp4?signature=secret"`)
	assert.NotContains(t, string(body), "/v1/videos/upstream-1/content")

	for _, videoURL := range []string{
		"",
		"http://video.example.com/v1/a/result.mp4",
		"https://user@video.example.com/v1/a/result.mp4",
		"https://cdn.example.com/v1/a/result.mp4",
	} {
		_, err := TaskResponseV2(
			[]byte(`{"id":"upstream-1","status":"completed","video_url":"`+videoURL+`"}`),
			"upstream-1",
			TaskResponseContext{BaseURL: "https://video.example.com"},
		)
		require.Error(t, err, videoURL)
		var violation *relaycommon.UpstreamContractViolation
		assert.True(t, errors.As(err, &violation))
	}
}

func TestCreateResponseRequiresStringTaskID(t *testing.T) {
	_, err := CreateResponse([]byte(`{"id":123}`))
	require.Error(t, err)
	_, err = CreateResponse([]byte(`{"id":" "}`))
	require.Error(t, err)
	_, err = CreateResponse([]byte(`{"id":"` + strings.Repeat("a", 192) + `"}`))
	require.Error(t, err)
	_, err = CreateResponse([]byte("{\"id\":\"unsafe\\u0000id\"}"))
	require.Error(t, err)
}

func TestTaskResponseRedactsSensitiveFailureDetails(t *testing.T) {
	body, err := TaskResponseV2([]byte(`{
		"id":"upstream-1",
		"status":"failed",
		"error":{"code":"provider_error","message":"fetch https://signed.example/result?token=secret"}
	}`), "upstream-1", TaskResponseContext{BaseURL: "https://video.example.com"})
	require.NoError(t, err)
	assert.NotContains(t, string(body), "signed.example")
	assert.NotContains(t, string(body), "secret")
	assert.Contains(t, string(body), "upstream task failed")
}
