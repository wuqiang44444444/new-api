package mediaarrays

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRequestUsesVerifiedMediaArraysContract(t *testing.T) {
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	duration, resolution, ratio := 6, "720p", "9:16"
	request := &dto.ModelArkVideoCreateRequest{
		Model: model.VideoSKUSeedance20Standard720P, Duration: &duration, Resolution: &resolution, Ratio: &ratio,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("scene one")},
			{Type: "text", Text: common.GetPointer("scene two")},
			{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "https://cdn.example.com/ref.png"}},
			{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://cdn.example.com/ref.mp3"}},
		},
	}
	body, err := CreateRequest(request, "seedance-2.0-vip-720p-azhw", capability)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-2.0-vip-720p-azhw","prompt":"scene one\nscene two","duration":6,"size":"720x1280","images":["https://cdn.example.com/ref.png"],"audios":["https://cdn.example.com/ref.mp3"]}`, string(body))
}

func TestResolveVideoSizeRejectsUnverifiedHighResolution(t *testing.T) {
	for _, resolution := range []string{
		"1080p",
		"4k",
	} {
		t.Run(resolution, func(t *testing.T) {
			_, ok := ResolveVideoSize(resolution, "16:9")
			assert.False(t, ok)
			_, ok = ResolveVideoSize(resolution, "9:16")
			assert.False(t, ok)
		})
	}
}

func TestCreateRequestRejectsUnpublishedRolesAndUnresolvedAssets(t *testing.T) {
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	request := &dto.ModelArkVideoCreateRequest{Model: capability.PublicModel, Content: []dto.ModelArkVideoContent{
		{Type: "text", Text: common.GetPointer("move")},
		{Type: "image_url", Role: common.GetPointer("first_frame"), ImageURL: &dto.VideoMediaURL{URL: "https://cdn.example.com/ref.png"}},
	}}
	_, err := CreateRequest(request, "upstream", capability)
	require.ErrorContains(t, err, "image role")

	request.Content[1].Role = common.GetPointer("reference_image")
	request.Content[1].ImageURL.URL = "asset://ast_unresolved"
	_, err = CreateRequest(request, "upstream", capability)
	require.ErrorContains(t, err, "not resolved")
}

func TestResponsesUseIDAsOnlyTaskIdentity(t *testing.T) {
	created, err := CreateResponse([]byte(`{"id":"task_public","task_id":"provider_internal","status":"queued"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task_public"}`, string(created))

	completed, err := TaskResponse(
		[]byte(`{"id":"task_public","task_id":"different_internal","status":"completed","video_url":"https://video.example.com/v1/videos/task_public/content"}`),
		"task_public",
		TaskResponseContext{BaseURL: "https://video.example.com"},
	)
	require.NoError(t, err)
	assert.Contains(t, string(completed), `"status":"succeeded"`)
	assert.NotContains(t, string(completed), `"task_id"`)
	assert.NotContains(t, string(completed), `"model"`)

	_, err = TaskResponse([]byte(`{"id":"different","status":"processing"}`), "task_public", TaskResponseContext{BaseURL: "https://video.example.com"})
	require.Error(t, err)
	_, err = TaskResponse([]byte(`{"id":"task_public","status":"in_progress"}`), "task_public", TaskResponseContext{BaseURL: "https://video.example.com"})
	require.Error(t, err)
}

func TestCreateResponseDoesNotFallBackToTaskID(t *testing.T) {
	_, err := CreateResponse([]byte(`{"task_id":"provider_internal","status":"queued"}`))
	require.ErrorContains(t, err, "no id")

	_, err = CreateResponse([]byte("{\"id\":\"bad\\u000aid\"}"))
	require.ErrorContains(t, err, "invalid id")
}

func TestCompletedResponseRequiresSameOriginHTTPSURL(t *testing.T) {
	tests := []string{
		`{"id":"task_public","status":"completed","video_url":"http://video.example.com/result.mp4"}`,
		`{"id":"task_public","status":"completed","video_url":"https://cdn.example.com/result.mp4"}`,
		`{"id":"task_public","status":"completed"}`,
	}
	for _, body := range tests {
		_, err := TaskResponse([]byte(body), "task_public", TaskResponseContext{BaseURL: "https://video.example.com"})
		require.Error(t, err)
	}
}
