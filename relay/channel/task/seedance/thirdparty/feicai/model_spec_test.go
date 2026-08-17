package feicai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRequestSupportsAllTenExactModelsAtDocumentedRatios(t *testing.T) {
	tests := []struct {
		model      string
		resolution string
		duration   int
		images     int
	}{
		{ProviderModelSeedance20Mini720P, "720p", 4, 0},
		{ProviderModelSeedance20SD2720P, "720p", 11, 1},
		{ProviderModelSeedance20Fast720P, "720p", 4, 0},
		{ProviderModelSeedance20Value720P, "720p", 4, 0},
		{ProviderModelSeedance20Standard720P, "720p", 4, 0},
		{ProviderModelSeedance20Value1080P, "1080p", 4, 0},
		{ProviderModelSeedance20Standard1080P, "1080p", 4, 0},
		{ProviderModelSeedance20Value4K, "4k", 4, 0},
		{ProviderModelSeedance20Standard4K, "4k", 4, 0},
		{ProviderModelSeedance20ProPI720P, "720p", 15, 0},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			request := validRequest(test.resolution, test.duration)
			for range test.images {
				request.Content = append(request.Content, imageContent())
			}
			resolved, err := ResolveRequest(request, test.model)
			require.NoError(t, err)
			assert.Equal(t, "16:9", resolved.Ratio)
			assert.Equal(t, test.resolution, resolved.Spec.Resolution)
		})
	}
}

func TestResolveRequestEnforcesProviderRatioAndResolutionBeforeCreation(t *testing.T) {
	request := validRequest("1080p", 4)
	_, err := ResolveRequest(request, ProviderModelSeedance20Standard720P)
	require.ErrorContains(t, err, "resolution must be \"720p\"")

	request = validRequest("720p", 11)
	request.Ratio = common.GetPointer("1:1")
	request.Content = append(request.Content, imageContent())
	_, err = ResolveRequest(request, ProviderModelSeedance20SD2720P)
	require.ErrorContains(t, err, "aspect ratio \"1:1\" is not supported")

	request = validRequest("720p", 4)
	request.Ratio = common.GetPointer("3:2")
	_, err = ResolveRequest(request, ProviderModelSeedance20Standard720P)
	require.ErrorContains(t, err, "aspect ratio \"3:2\" is not supported")
}

func TestResolveRequestRejectsUnsupportedOutputFormat(t *testing.T) {
	request := validRequest("720p", 4)
	request.OutputFormat = common.GetPointer("mov")

	_, err := ResolveRequest(request, ProviderModelSeedance20Mini720P)
	require.ErrorContains(t, err, "unsupported by the selected customer model")
}

func TestResolveRequestAcceptsSixStandardRatiosAndTwoSD2Ratios(t *testing.T) {
	for _, ratio := range []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"} {
		t.Run("mini_"+ratio, func(t *testing.T) {
			request := validRequest("720p", 4)
			request.Ratio = &ratio
			resolved, err := ResolveRequest(request, ProviderModelSeedance20Mini720P)
			require.NoError(t, err)
			assert.Equal(t, ratio, resolved.Ratio)
		})
	}
	for _, ratio := range []string{"16:9", "9:16"} {
		t.Run("sd2_"+ratio, func(t *testing.T) {
			request := validRequest("720p", 11)
			request.Ratio = &ratio
			request.Content = append(request.Content, imageContent())
			resolved, err := ResolveRequest(request, ProviderModelSeedance20SD2720P)
			require.NoError(t, err)
			assert.Equal(t, ratio, resolved.Ratio)
		})
	}
}

func TestResolveRequestEnforcesModelSpecificMediaAndDuration(t *testing.T) {
	request := validRequest("720p", 11)
	_, err := ResolveRequest(request, ProviderModelSeedance20SD2720P)
	require.ErrorContains(t, err, "image count must be between 1 and 9")

	request = validRequest("720p", 4)
	request.Content = append(request.Content, videoContent())
	_, err = ResolveRequest(request, ProviderModelSeedance20Mini720P)
	require.ErrorContains(t, err, "video count must not exceed 0")

	request = validRequest("720p", 14)
	_, err = ResolveRequest(request, ProviderModelSeedance20ProPI720P)
	require.ErrorContains(t, err, "duration must be between 15 and 15")

	request = validRequest("720p", 15)
	for range 3 {
		request.Content = append(request.Content, videoContent())
	}
	_, err = ResolveRequest(request, ProviderModelSeedance20ProPI720P)
	require.NoError(t, err)
}

func TestCreateRequestUsesMappedProviderModelAndForwardsRatio(t *testing.T) {
	request := validRequest("4k", 4)
	request.Ratio = common.GetPointer("21:9")
	body, err := CreateRequest(request, ProviderModelSeedance20Standard4K)
	require.NoError(t, err)

	var upstream createRequest
	require.NoError(t, common.Unmarshal(body, &upstream))
	assert.Equal(t, ProviderModelSeedance20Standard4K, upstream.Model)
	assert.Equal(t, "21:9", upstream.Ratio)
	assert.Equal(t, 4, upstream.Duration)
	assert.NotContains(t, string(body), `"size"`)
}

func TestCurrentModelSpecsAreUniqueAndResolvable(t *testing.T) {
	specs := CurrentModelSpecs()
	require.Len(t, specs, 10)
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		_, duplicate := seen[spec.ProviderModel]
		assert.False(t, duplicate, spec.ProviderModel)
		seen[spec.ProviderModel] = struct{}{}

		resolved, ok := ResolveModelSpec(spec.ProviderModel)
		require.True(t, ok, spec.ProviderModel)
		assert.Equal(t, spec, resolved)
		if spec.ProviderModel == ProviderModelSeedance20SD2720P {
			assert.Equal(t, []string{"16:9", "9:16"}, resolved.Ratios)
		} else {
			assert.Equal(t, []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}, resolved.Ratios)
		}
	}
}

func TestResolveRequestValidatesFeicaiMediaRepresentations(t *testing.T) {
	validDataURL := validRequest("720p", 4)
	validDataURL.Content = append(validDataURL.Content, dto.ModelArkVideoContent{
		Type: "image_url", Role: common.GetPointer("reference_image"),
		ImageURL: &dto.VideoMediaURL{URL: "data:image/png;base64,aW1hZ2U="},
	})
	_, err := ResolveRequest(validDataURL, ProviderModelSeedance20Mini720P)
	require.NoError(t, err)

	tests := []struct {
		name    string
		content dto.ModelArkVideoContent
		message string
	}{
		{
			name: "audio requires HTTPS",
			content: dto.ModelArkVideoContent{Type: "audio_url", Role: common.GetPointer("reference_audio"),
				AudioURL: &dto.VideoMediaURL{URL: "http://media.example.com/reference.mp3"}},
			message: "https URL",
		},
		{
			name: "video rejects data URL",
			content: dto.ModelArkVideoContent{Type: "video_url", Role: common.GetPointer("reference_video"),
				VideoURL: &dto.VideoMediaURL{URL: "data:video/mp4;base64,dmlkZW8="}},
			message: "data URL is not supported",
		},
		{
			name: "image MIME is restricted",
			content: dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"),
				ImageURL: &dto.VideoMediaURL{URL: "data:image/gif;base64,aW1hZ2U="}},
			message: "MIME or encoding is not supported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest("720p", 4)
			request.Content = append(request.Content, test.content)
			_, err := ResolveRequest(request, ProviderModelSeedance20Mini720P)
			require.ErrorContains(t, err, test.message)
		})
	}

	request := validRequest("720p", 4)
	request.Content = append(request.Content, dto.ModelArkVideoContent{
		Type: "image_url", Role: common.GetPointer("reference_image"),
		ImageURL: &dto.VideoMediaURL{URL: "asset://provider-opaque-id"},
	})
	_, err = ResolveRequest(request, ProviderModelSeedance20Mini720P)
	require.NoError(t, err)
}

func validRequest(resolution string, duration int) *dto.ModelArkVideoCreateRequest {
	return &dto.ModelArkVideoCreateRequest{
		Duration:   &duration,
		Resolution: &resolution,
		Ratio:      common.GetPointer("16:9"),
		Content: []dto.ModelArkVideoContent{{
			Type: "text",
			Text: common.GetPointer("A paper boat crosses a quiet lake"),
		}},
	}
}

func imageContent() dto.ModelArkVideoContent {
	return dto.ModelArkVideoContent{
		Type: "image_url", Role: common.GetPointer("reference_image"),
		ImageURL: &dto.VideoMediaURL{URL: "https://media.example.com/reference.png"},
	}
}

func videoContent() dto.ModelArkVideoContent {
	return dto.ModelArkVideoContent{
		Type: "video_url", Role: common.GetPointer("reference_video"),
		VideoURL: &dto.VideoMediaURL{URL: "https://media.example.com/reference.mp4"},
	}
}
