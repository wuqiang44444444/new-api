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
	implementation := dto.LinkImplementationRef{
		ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
	}
	key := videoSizeRegistryKey{
		ImplementationID: implementation.ID, ImplementationVersion: implementation.Version,
		ProviderModel: model.FeicaiProviderModelSeedance20Standard720P,
		Resolution:    "720p", Ratio: "9:16",
	}
	videoSizes[key] = VideoSize{
		Value: "720x1280", Multiplier: 1, BillingClass: "standard-720p", EvidenceVersion: "test-evidence",
	}
	t.Cleanup(func() { delete(videoSizes, key) })

	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	capability.Ratios = []string{"9:16"}
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
	body, err := CreateRequest(request, implementation, model.FeicaiProviderModelSeedance20Standard720P, capability)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-2.0-vip-720p-azhw-feicai","prompt":"scene one\nscene two","duration":6,"size":"720x1280","images":["https://cdn.example.com/ref.png"],"audios":["https://cdn.example.com/ref.mp3"]}`, string(body))
}

func TestResolveVideoSizeRequiresExactImplementationAndProviderEvidence(t *testing.T) {
	implementation := dto.LinkImplementationRef{
		ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
	}
	for _, resolution := range []string{"720p", "1080p", "4k"} {
		t.Run(resolution, func(t *testing.T) {
			_, ok := ResolveVideoSize(implementation, model.FeicaiProviderModelSeedance20Value1080P, resolution, "16:9")
			assert.False(t, ok)
			_, ok = ResolveVideoSize(implementation, model.FeicaiProviderModelSeedance20Value1080P, resolution, "9:16")
			assert.False(t, ok)
		})
	}
}

func TestResolveVideoSizeIncludesVerifiedFeicaiGatewayCandidates(t *testing.T) {
	implementation := dto.LinkImplementationRef{
		ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
	}
	tests := []struct {
		providerModel string
		resolution    string
		evidence      string
	}{
		{providerModel: model.FeicaiProviderModelSeedance20Mini720P, resolution: "720p", evidence: feicaiV2Evidence20260805},
		{providerModel: model.FeicaiProviderModelSeedance20Fast720P, resolution: "720p", evidence: feicaiV2Evidence20260806},
		{providerModel: model.FeicaiProviderModelSeedance20Value720P, resolution: "720p", evidence: feicaiV2Evidence20260806R3},
		{providerModel: model.FeicaiProviderModelSeedance20Standard720P, resolution: "720p", evidence: feicaiV2Evidence20260805},
		{providerModel: model.FeicaiProviderModelSeedance20Standard1080P, resolution: "1080p", evidence: feicaiV2Evidence20260805},
		{providerModel: model.FeicaiProviderModelSeedance20Standard4K, resolution: "4k", evidence: feicaiV2Evidence20260806},
	}
	for _, test := range tests {
		size, ok := ResolveVideoSize(implementation, test.providerModel, test.resolution, "16:9")
		require.True(t, ok, test.providerModel)
		assert.Equal(t, "1280x720", size.Value, test.providerModel)
		assert.Equal(t, 1.0, size.Multiplier, test.providerModel)
		assert.NotEmpty(t, size.BillingClass, test.providerModel)
		assert.Equal(t, test.evidence, size.EvidenceVersion, test.providerModel)
	}
}

func TestCreateRequestRejectsUnpublishedRolesAndUnresolvedAssets(t *testing.T) {
	implementation := dto.LinkImplementationRef{
		ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
	}
	key := videoSizeRegistryKey{
		ImplementationID: implementation.ID, ImplementationVersion: implementation.Version,
		ProviderModel: model.FeicaiProviderModelSeedance20Standard720P,
		Resolution:    "720p", Ratio: "16:9",
	}
	videoSizes[key] = VideoSize{
		Value: "1280x720", Multiplier: 1, BillingClass: "standard-720p", EvidenceVersion: "test-evidence",
	}
	t.Cleanup(func() { delete(videoSizes, key) })

	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	capability.Ratios = []string{"16:9"}
	duration, resolution, ratio := 4, "720p", "16:9"
	request := &dto.ModelArkVideoCreateRequest{
		Model: capability.PublicModel, Duration: &duration, Resolution: &resolution, Ratio: &ratio,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("move")},
			{Type: "image_url", Role: common.GetPointer("first_frame"), ImageURL: &dto.VideoMediaURL{URL: "https://cdn.example.com/ref.png"}},
		}}
	_, err := CreateRequest(request, implementation, model.FeicaiProviderModelSeedance20Standard720P, capability)
	require.ErrorContains(t, err, "image role")

	request.Content[1].Role = common.GetPointer("reference_image")
	request.Content[1].ImageURL.URL = "asset://ast_unresolved"
	_, err = CreateRequest(request, implementation, model.FeicaiProviderModelSeedance20Standard720P, capability)
	require.ErrorContains(t, err, "not resolved")
}

func TestCreateRequestProPIIncludesVerifiedReferenceVideoArray(t *testing.T) {
	implementation := dto.LinkImplementationRef{
		ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
	}
	key := videoSizeRegistryKey{
		ImplementationID: implementation.ID, ImplementationVersion: implementation.Version,
		ProviderModel: model.FeicaiProviderModelSeedance20ProPI720P,
		Resolution:    "720p", Ratio: "16:9",
	}
	videoSizes[key] = VideoSize{
		Value: "1280x720", Multiplier: 1, BillingClass: "pro-pi-720p", EvidenceVersion: "test-evidence",
	}
	t.Cleanup(func() { delete(videoSizes, key) })

	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20ProPI720P)
	require.True(t, ok)
	capability.Ratios = []string{"16:9"}
	resolution, ratio := "720p", "16:9"
	request := &dto.ModelArkVideoCreateRequest{
		Model: model.VideoSKUSeedance20ProPI720P, Resolution: &resolution, Ratio: &ratio,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("animate the reference")},
			{Type: "video_url", Role: common.GetPointer("reference_video"), VideoURL: &dto.VideoMediaURL{URL: "https://cdn.example.com/reference.mp4"}},
		},
	}
	body, err := CreateRequest(request, implementation, model.FeicaiProviderModelSeedance20ProPI720P, capability)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-933-pro-pi-feicai","prompt":"animate the reference","duration":15,"size":"1280x720","videos":["https://cdn.example.com/reference.mp4"]}`, string(body))
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
	inProgress, err := TaskResponse(
		[]byte(`{"id":"task_public","status":"in_progress"}`),
		"task_public",
		TaskResponseContext{BaseURL: "https://video.example.com"},
	)
	require.NoError(t, err)
	assert.Contains(t, string(inProgress), `"status":"running"`)
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
