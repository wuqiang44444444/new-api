package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeicaiV2VideoSKUCapabilitiesCoverAllProviderModelsAndFailClosed(t *testing.T) {
	models := map[string]struct {
		resolution  string
		minDuration int
		minImages   int
		maxAudio    int
		maxVideos   int
		billingMode string
		ratios      []string
		version     string
	}{
		VideoSKUSeedance20Mini720P:      {"720p", 4, 0, 3, 0, VideoBillingModePerSecond, []string{"16:9"}, VideoSKUCapabilityVersionFeicaiV2},
		VideoSKUSeedance20SD2720P:       {"720p", 11, 1, 0, 0, VideoBillingModePerSecond, nil, VideoSKUCapabilityVersionFeicaiV2},
		VideoSKUSeedance20Fast720P:      {"720p", 4, 0, 3, 0, VideoBillingModePerSecond, []string{"16:9"}, VideoSKUCapabilityVersionFeicaiV2R2},
		VideoSKUSeedance20Value720P:     {"720p", 4, 0, 3, 0, VideoBillingModePerSecond, []string{"16:9"}, VideoSKUCapabilityVersionFeicaiV2R3},
		VideoSKUSeedance20Standard720P:  {"720p", 4, 0, 3, 0, VideoBillingModePerSecond, []string{"16:9"}, VideoSKUCapabilityVersionFeicaiV2},
		VideoSKUSeedance20Value1080P:    {"1080p", 4, 0, 3, 0, VideoBillingModePerSecond, nil, VideoSKUCapabilityVersionFeicaiV2},
		VideoSKUSeedance20Standard1080P: {"1080p", 4, 0, 3, 0, VideoBillingModePerSecond, []string{"16:9"}, VideoSKUCapabilityVersionFeicaiV2},
		VideoSKUSeedance20Value4K:       {"4k", 4, 0, 3, 0, VideoBillingModePerSecond, nil, VideoSKUCapabilityVersionFeicaiV2},
		VideoSKUSeedance20Standard4K:    {"4k", 4, 0, 3, 0, VideoBillingModePerSecond, []string{"16:9"}, VideoSKUCapabilityVersionFeicaiV2R2},
		VideoSKUSeedance20ProPI720P:     {"720p", 15, 0, 3, 3, VideoBillingModePerRequest, nil, VideoSKUCapabilityVersionFeicaiV2},
	}
	for publicModel, expected := range models {
		t.Run(publicModel, func(t *testing.T) {
			first, ok := ResolveVideoSKUCapability(publicModel)
			require.True(t, ok)
			second, ok := ResolveVideoSKUCapability(publicModel)
			require.True(t, ok)
			assert.Equal(t, expected.version, first.Version)
			assert.Equal(t, expected.resolution, first.Resolution)
			assert.Equal(t, expected.minDuration, first.MinDuration)
			assert.Equal(t, expected.minImages, first.MinImages)
			assert.Equal(t, expected.maxAudio, first.MaxAudio)
			assert.Equal(t, expected.maxVideos, first.MaxVideos)
			assert.Equal(t, expected.billingMode, first.BillingMode)
			assert.Equal(t, first.ContentHash, second.ContentHash)
			assert.Len(t, first.ContentHash, 64)
			assert.True(t, first.SupportsProfile(VideoProfileJSONMediaArrays))
			assert.Equal(t, expected.ratios, first.Ratios)
			assert.Equal(t, []string{"reference_image"}, first.ImageRoles)
			assert.Equal(t, []string{"reference_audio"}, first.AudioRoles)
			assert.True(t, first.SupportsLinkAssets)
			assert.False(t, first.SupportsMixedMediaPath)
			if expected.maxVideos > 0 {
				assert.Equal(t, []string{"reference_video"}, first.VideoRoles)
				assert.Equal(t, 15, first.DefaultDuration)
			} else {
				assert.Empty(t, first.VideoRoles)
				assert.Zero(t, first.DefaultDuration)
			}
		})
	}
}

func TestModelArkCapabilityRegistryUsesCanonicalVocabularyAndPinnedImplementationHashes(t *testing.T) {
	for publicModel, capability := range videoSKUCapabilities {
		if capability.ContractID != string(dto.VideoContractModelArkV3) {
			continue
		}
		require.NoError(t, validateModelArkCapabilityVocabulary(capability), publicModel)
		assert.Equal(t, capability.ContentHash, videoSKUImplementationHashes[publicModel], publicModel)
	}
}

func TestRegisteredModelArkCapabilityProjectionIncludesEverySeedanceSKU(t *testing.T) {
	projections := RegisteredModelArkVideoCapabilityProjection()
	require.Len(t, projections, 15)
	projected := make(map[string]ModelArkVideoCapabilityProjection, len(projections))
	for _, projection := range projections {
		projected[projection.PublicModel] = projection
		assert.NotEqual(t, VideoSKUCapabilityVersionV1, projection.Version, projection.PublicModel)
	}
	for publicModel, capability := range videoSKUCapabilities {
		if capability.ContractID != string(dto.VideoContractModelArkV3) {
			continue
		}
		projection, ok := projected[publicModel]
		require.True(t, ok, publicModel)
		assert.Equal(t, capability.Version, projection.Version, publicModel)
		assert.Equal(t, capability.ContentHash, projection.ContentHash, publicModel)
	}
}

func TestFeicaiV2SD2AndProPIEnforceDistinctMediaContracts(t *testing.T) {
	sd2, ok := ResolveVideoSKUCapability(VideoSKUSeedance20SD2720P)
	require.True(t, ok)
	sd2.Ratios = []string{"16:9"}
	duration, resolution, ratio := 11, "720p", "16:9"
	sd2Request := &dto.ModelArkVideoCreateRequest{
		Model: VideoSKUSeedance20SD2720P, Duration: &duration, Resolution: &resolution, Ratio: &ratio,
		Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
	}
	require.ErrorContains(t, sd2.ValidateModelArkRequest(sd2Request), "at least 1")
	sd2Request.Content = append(sd2Request.Content, dto.ModelArkVideoContent{
		Type: "image_url", Role: common.GetPointer("reference_image"),
		ImageURL: &dto.VideoMediaURL{URL: "https://example.com/reference.png"},
	})
	require.NoError(t, sd2.ValidateModelArkRequest(sd2Request))

	proPI, ok := ResolveVideoSKUCapability(VideoSKUSeedance20ProPI720P)
	require.True(t, ok)
	proPI.Ratios = []string{"16:9"}
	proRequest := &dto.ModelArkVideoCreateRequest{
		Model: VideoSKUSeedance20ProPI720P, Resolution: &resolution, Ratio: &ratio,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("move")},
			{Type: "video_url", Role: common.GetPointer("reference_video"), VideoURL: &dto.VideoMediaURL{URL: "https://example.com/reference.mp4"}},
		},
	}
	require.NoError(t, proPI.ValidateModelArkRequest(proRequest))
}

func TestMoxingV2CapabilityRejectsUnverifiedFieldsAndRequiresExplicitDimensions(t *testing.T) {
	capability, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Oversea)
	require.True(t, ok)
	assert.Equal(t, VideoSKUCapabilityVersionMoxingV2, capability.Version)
	assert.Equal(t, []string{"480p", "720p"}, capability.Resolutions)
	assert.Equal(t, []string{"", "first_frame", "last_frame", "reference_image"}, capability.ImageRoles)
	assert.True(t, capability.RequiresDuration)
	assert.True(t, capability.RequiresResolution)
	assert.True(t, capability.RequiresRatio)
	assert.Equal(t, 2500, capability.MaxPromptCharacters)
	assert.True(t, capability.SupportsGenerateAudio)
	assert.False(t, capability.Lifecycle.SupportsLastFrame)

	request := &dto.ModelArkVideoCreateRequest{
		Model:   VideoSKUSeedance20Oversea,
		Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
	}
	require.ErrorContains(t, capability.ValidateModelArkRequest(request), "duration is required")

	duration, resolution, ratio := -1, "720p", "16:9"
	generateAudio := false
	request.Duration = &duration
	request.Resolution = &resolution
	request.Ratio = &ratio
	request.GenerateAudio = &generateAudio
	require.NoError(t, capability.ValidateModelArkRequest(request))

	resolution = "1080p"
	require.ErrorContains(t, capability.ValidateModelArkRequest(request), "resolution")
	resolution = "720p"
	watermark := false
	request.Watermark = &watermark
	require.ErrorContains(t, capability.ValidateModelArkRequest(request), "watermark")
}

func TestTokenSaveV2CapabilityIsIndependentAndRejectsUnverifiedMultimodalFields(t *testing.T) {
	capability, ok := ResolveVideoSKUCapability(VideoSKUDoubaoSeedance20260128)
	require.True(t, ok)
	assert.Equal(t, VideoSKUCapabilityVersionTokenSaveV2, capability.Version)
	assert.Equal(t, []string{"480p", "720p", "1080p"}, capability.Resolutions)
	assert.Equal(t, []string{"", "first_frame", "last_frame", "reference_image"}, capability.ImageRoles)
	assert.True(t, capability.RequiresDuration)
	assert.True(t, capability.RequiresResolution)
	assert.True(t, capability.RequiresRatio)
	assert.True(t, capability.RequiresText)
	assert.Zero(t, capability.MaxVideos)
	assert.Zero(t, capability.MaxAudio)
	assert.False(t, capability.Lifecycle.SupportsLastFrame)

	duration, resolution, ratio := 4, "1080p", "16:9"
	request := &dto.ModelArkVideoCreateRequest{
		Model: VideoSKUDoubaoSeedance20260128, Duration: &duration, Resolution: &resolution, Ratio: &ratio,
		Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
	}
	require.NoError(t, capability.ValidateModelArkRequest(request))

	request.Content = append(request.Content, dto.ModelArkVideoContent{
		Type: "video_url", Role: common.GetPointer("reference_video"),
		VideoURL: &dto.VideoMediaURL{URL: "https://example.com/reference.mp4"},
	})
	require.ErrorContains(t, capability.ValidateModelArkRequest(request), "video content")
}

func TestVideoSKUCapabilityLimitsAreDataDriven(t *testing.T) {
	capability, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	capability.MaxImages = 1
	capability.Ratios = []string{"16:9"}
	duration, resolution, ratio := 4, "720p", "16:9"

	err := capability.ValidateModelArkRequest(&dto.ModelArkVideoCreateRequest{
		Model: capability.PublicModel, Duration: &duration, Resolution: &resolution, Ratio: &ratio,
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("hello")},
			{
				Type:     "image_url",
				Role:     common.GetPointer("reference_image"),
				ImageURL: &dto.VideoMediaURL{URL: "https://example.com/one.png"},
			},
			{
				Type:     "image_url",
				Role:     common.GetPointer("reference_image"),
				ImageURL: &dto.VideoMediaURL{URL: "https://example.com/two.png"},
			},
		},
	})

	require.ErrorContains(t, err, "maximum of 1")
}

func TestFunCloudVideoSKUCapabilitiesMatchTheirEndpoints(t *testing.T) {
	standard, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Standard)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"480p", "720p", "1080p"}, standard.Resolutions)
	assert.True(t, standard.SupportsLinkAssets)
	assert.False(t, standard.AllowsAutomaticDuration)
	assert.True(t, standard.SupportsProfile(VideoProfileFunCloudSeedanceV2))
	assert.Equal(t, ModelArkResolution720P, standard.DefaultResolution)
	assert.Equal(t, "16:9", standard.DefaultRatio)
	assert.ElementsMatch(t, []string{"reference_image", "first_frame", "last_frame"}, standard.ImageRoles)
	assert.Equal(t, []string{"reference_video"}, standard.VideoRoles)
	assert.Equal(t, []string{"reference_audio"}, standard.AudioRoles)

	fast, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Fast)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"480p", "720p"}, fast.Resolutions)
	assert.True(t, fast.SupportsLinkAssets)
	assert.False(t, fast.AllowsAutomaticDuration)
	assert.ElementsMatch(t, standard.ImageRoles, fast.ImageRoles)
	assert.Equal(t, standard.VideoRoles, fast.VideoRoles)
	assert.Equal(t, standard.AudioRoles, fast.AudioRoles)

	duration := -1
	request := &dto.ModelArkVideoCreateRequest{
		Model:    VideoSKUSeedance20Standard,
		Duration: &duration,
		Content:  []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
	}
	require.ErrorContains(t, standard.ValidateModelArkRequest(request), "duration")

}

func TestModelArkCapabilityDefaultsAndCombinationMatrixArePartOfValidation(t *testing.T) {
	capability, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Standard)
	require.True(t, ok)
	capability.ResolutionRatioCombinations = []VideoResolutionRatioCombination{
		{Resolution: ModelArkResolution720P, Ratio: "16:9"},
	}
	capability.ContentHash = videoSKUCapabilityHash(capability)
	request := &dto.ModelArkVideoCreateRequest{
		Model:   capability.PublicModel,
		Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
	}
	require.NoError(t, capability.ValidateModelArkRequest(request))

	ratio := "9:16"
	request.Ratio = &ratio
	require.ErrorContains(t, capability.ValidateModelArkRequest(request), "not supported together")

	bytePlus, ok := ResolveVideoSKUCapability(VideoSKUSeedanceBytePlus)
	require.True(t, ok)
	request.Model = bytePlus.PublicModel
	request.Ratio = nil
	require.ErrorContains(t, bytePlus.ValidateModelArkRequest(request), "resolution is required")

	fast, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Fast)
	require.True(t, ok)
	request.Model = fast.PublicModel
	request.Tools = common.GetPointer([]dto.ModelArkVideoTool{})
	require.ErrorContains(t, fast.ValidateModelArkRequest(request), "tools is not supported")
}

func TestKlingAndJimengCapabilitiesOwnPublishedRequestValidation(t *testing.T) {
	kling, ok := ResolveVideoSKUCapability(VideoSKUKlingV1)
	require.True(t, ok)
	err := kling.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractKlingV1,
		Kling: &dto.KlingVideoCreateRequest{
			ModelName:   common.GetPointer(VideoSKUKlingV1),
			Prompt:      common.GetPointer("move"),
			Duration:    common.GetPointer("15"),
			AspectRatio: common.GetPointer("16:9"),
		},
	})
	require.ErrorContains(t, err, "duration")

	err = kling.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractKlingV1,
		Kling: &dto.KlingVideoCreateRequest{
			ModelName: common.GetPointer(VideoSKUKlingV1),
			Prompt:    common.GetPointer("move"),
			Duration:  common.GetPointer("10"),
		},
	})
	require.NoError(t, err)

	jimeng, ok := ResolveVideoSKUCapability(VideoSKUJimengVGFMT2VL20)
	require.True(t, ok)
	err = jimeng.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractJimeng,
		Jimeng:     &dto.JimengVideoCreateRequest{ReqKey: "unknown"},
	})
	require.ErrorContains(t, err, "does not match")
	require.NoError(t, jimeng.ValidateContractRequest(dto.VideoContractRequest{
		ContractID: dto.VideoContractJimeng,
		Jimeng:     &dto.JimengVideoCreateRequest{ReqKey: VideoSKUJimengVGFMT2VL20},
	}))
}
