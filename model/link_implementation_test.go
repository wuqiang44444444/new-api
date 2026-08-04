package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/provider_exposure_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkImplementationRegistryAndExplicitChannelRegistration(t *testing.T) {
	require.NoError(t, ValidateLinkImplementationRegistry())

	channel := &Channel{
		Type: constant.ChannelTypeDoubaoVideo, Models: VideoSKUSeedance20Standard,
		Status: common.ChannelStatusEnabled,
	}
	settings := dto.ChannelOtherSettings{
		VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2,
		VideoUpstreamCreatePath:        "/api/v2/open/aigc/seedance2-0",
		VideoUpstreamQueryPathTemplate: "/api/v2/open/aigc/{task_id}",
		AssetUpstreamProfile:           dto.AssetUpstreamProfileNone,
		LinkImplementation: dto.LinkImplementationRef{
			ID: LinkImplementationFunCloudSeedance, Version: LinkImplementationVersionV1,
		},
	}
	require.NoError(t, ValidateLinkImplementationRegistration(channel, &settings))

	settings.VideoUpstreamCreatePath = "/api/v2/open/aigc/seedance2-0-fast"
	assert.Error(t, ValidateLinkImplementationRegistration(channel, &settings))

	settings.VideoUpstreamCreatePath = "/api/v2/open/aigc/seedance2-0"
	settings.LinkImplementation.Version = "v2"
	assert.Error(t, ValidateLinkImplementationRegistration(channel, &settings))

	settings.LinkImplementation = dto.LinkImplementationRef{}
	assert.Error(t, ValidateLinkImplementationRegistration(channel, &settings))
}

func TestLinkImplementationRegistryRejectsDuplicateNormalizedIDs(t *testing.T) {
	_, err := buildLinkImplementationRegistryFrom([]LinkImplementation{
		{ID: "duplicate"},
		{ID: " duplicate "},
	})
	require.ErrorContains(t, err, `duplicate Link implementation ID "duplicate"`)
}

func TestFeicaiMediaArraysImplementationRequiresExactMappingAndSourceContract(t *testing.T) {
	implementation, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV1,
	})
	require.True(t, ok)
	assert.Equal(t, VideoProfileJSONMediaArrays, implementation.RequiredVideoProfile)
	assert.Equal(t, "54:third_party_json_video_media_arrays:v1", implementation.RequiredAdapterVersion)
	assert.Equal(t, int64(3600), implementation.AssetCapability.SourceMinTTLSeconds)
	assert.Equal(t, []string{AssetKindGeneral}, implementation.AssetCapability.AssetKinds)
	assert.ElementsMatch(t, []string{"image", "audio"}, implementation.AssetCapability.MediaTypes)
	assert.Equal(t, 9, implementation.AssetCapability.MaxImages)
	assert.Equal(t, 3, implementation.AssetCapability.MaxAudio)
	assert.Zero(t, implementation.AssetCapability.MaxVideos)
	assert.True(t, implementation.AssetCapability.Supports(LinkAssetResolutionSourceURL))
	assert.ElementsMatch(t, []string{VideoSKUSeedance20Standard720P, VideoSKUSeedance20Value720P}, implementation.PublicSKUs)

	mapping := `{"seedance-2.0-standard-720p":"seedance-2.0-vip-720p-azhw"}`
	channel := &Channel{
		Type: constant.ChannelTypeDoubaoVideo, Models: VideoSKUSeedance20Standard720P,
		Status: common.ChannelStatusEnabled, ModelMapping: &mapping,
	}
	settings := dto.ChannelOtherSettings{
		VideoUpstreamProfile:    dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		VideoUpstreamCreatePath: "/v1/videos", VideoUpstreamQueryPathTemplate: "/v1/videos/{task_id}",
		AssetUpstreamProfile: dto.AssetUpstreamProfileNone,
		LinkImplementation:   dto.LinkImplementationRef{ID: implementation.ID, Version: implementation.Version},
	}
	require.NoError(t, ValidateLinkImplementationRegistration(channel, &settings))

	highResolutionMapping := `{"seedance-2.0-standard-1080p":"seedance-2.0-vip-1080p-azhw"}`
	channel.Models = VideoSKUSeedance20Standard1080P
	channel.ModelMapping = &highResolutionMapping
	require.Error(t, ValidateLinkImplementationRegistration(channel, &settings))

	badMapping := `{"seedance-2.0-standard-720p":"seedance-2.0-vip-720p-azhw-feicai"}`
	channel.Models = VideoSKUSeedance20Standard720P
	channel.ModelMapping = &badMapping
	require.Error(t, ValidateLinkImplementationRegistration(channel, &settings))
}

func TestLinkImplementationRequiresActiveExposurePolicyAtRuntime(t *testing.T) {
	policy := provider_exposure_setting.GetSetting()
	original := *policy
	t.Cleanup(func() { *policy = original })

	channel := &Channel{
		Type: constant.ChannelTypeDoubaoVideo, Models: VideoSKUSeedance20Standard,
		Status: common.ChannelStatusEnabled,
	}
	settings := dto.ChannelOtherSettings{
		VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2,
		VideoUpstreamCreatePath:        "/api/v2/open/aigc/seedance2-0",
		VideoUpstreamQueryPathTemplate: "/api/v2/open/aigc/{task_id}",
		AssetUpstreamProfile:           dto.AssetUpstreamProfileNone,
		LinkImplementation: dto.LinkImplementationRef{
			ID: LinkImplementationFunCloudSeedance, Version: LinkImplementationVersionV1,
		},
	}
	channel.SetOtherSettings(settings)

	*policy = provider_exposure_setting.PolicySetting{
		Enabled:                  false,
		MonitoredImplementations: LinkImplementationFunCloudSeedance + "/" + LinkImplementationVersionV1,
	}
	assert.Error(t, ValidateLinkImplementationRegistration(channel, &settings))
	assert.Error(t, ValidateChannelLinkImplementationForSKU(channel, VideoSKUSeedance20Standard))

	channel.Status = common.ChannelStatusManuallyDisabled
	assert.NoError(t, ValidateLinkImplementationRegistration(channel, &settings))
	assert.Error(t, ValidateChannelLinkImplementationForSKU(channel, VideoSKUSeedance20Standard))

	policy.Enabled = true
	assert.NoError(t, ValidateChannelLinkImplementationForSKU(channel, VideoSKUSeedance20Standard))
}

func TestLinkImplementationContentHashIsCanonicalAndScoped(t *testing.T) {
	expectedHashes := map[string]string{
		LinkImplementationBytePlusSeedanceArk:  "sha256:54ddaa65892ebaa8c688769125a38f69d957f21c8799902cb61558aade6a4777",
		LinkImplementationFeicaiSeedanceVideos: "sha256:155df9ab499c00e18605986f06b040d32aa86bb13a3c63142ee57977b6fe45d8",
		LinkImplementationFunCloudSeedance:     "sha256:d07d02f6c7362508bc9550b556865b5a1fb2067c45bb75daff08c9144f333c27",
		LinkImplementationJimengVideos:         "sha256:3c65f2a82fc6c11a0c76b0af9fdba9b155f4fcf4ea46c76dc312737335c5fce7",
		LinkImplementationKlingVideos:          "sha256:9b9839f6c33508abf6d304932b24af7f1b10259e5e20a115287e5ee2e3cd084f",
		LinkImplementationMoxingImages:         "sha256:a02a43716619140cac7ca8cdf886cce8bdbebd3ec741ddb4fb451ef487beaf5c",
		LinkImplementationMoxingSeedanceArk:    "sha256:8a2b07ddc0286a1a20e23643eb1112496678bf623e6530de8c5a765daf217740",
		LinkImplementationMoxingSeedanceMedia:  "sha256:cb4152fd07a5e9f5582bb57a7557b40af73b3476ea39fe8eb64d4891cfa0acd7",
		LinkImplementationQihangImages:         "sha256:8cced3abda6325184f9f34e3a4088c0b65164393f1f957bb64dd47d3f2571cac",
		LinkImplementationTokenSaveSeedance:    "sha256:300190cf339c6446e4dfcd87337bd3245adcc5339631846e53499091439279ce",
	}
	implementations := ListLinkImplementations()
	require.Len(t, implementations, len(expectedHashes))
	for _, registered := range implementations {
		assert.Equal(t, expectedHashes[registered.ID], registered.ContentHash)
	}
	implementation, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationMoxingSeedanceMedia, Version: LinkImplementationVersionV1,
	})
	require.True(t, ok)
	assert.Contains(t, implementation.ContentHash, "sha256:")

	changed := implementation
	changed.RequiredQueryPath = "/different/{task_id}"
	assert.NotEqual(t, implementation.ContentHash, linkImplementationContentHash(changed))

	implementationsForSKU := LinkImplementationsForSKU(VideoSKUSeedance20Oversea)
	assert.Len(t, implementationsForSKU, 2)
}
