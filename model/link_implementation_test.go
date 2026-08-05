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

func TestLinkImplementationRegistryRejectsDuplicateNormalizedIdentities(t *testing.T) {
	_, err := buildLinkImplementationRegistryFrom([]LinkImplementation{
		{ID: "duplicate"},
		{ID: " duplicate "},
	})
	require.ErrorContains(t, err, `duplicate Link implementation identity "duplicate"/""`)
}

func TestFeicaiMediaArraysImplementationRequiresExactMappingAndSourceContract(t *testing.T) {
	_, v1Exists := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV1,
	})
	assert.False(t, v1Exists)

	implementation, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV2,
	})
	require.True(t, ok)
	assert.Equal(t, VideoProfileJSONMediaArrays, implementation.RequiredVideoProfile)
	assert.Equal(t, "54:third_party_json_video_media_arrays:v2", implementation.RequiredAdapterVersion)
	assert.Equal(t, int64(3600), implementation.AssetCapability.SourceMinTTLSeconds)
	assert.Equal(t, []string{AssetKindGeneral}, implementation.AssetCapability.AssetKinds)
	assert.ElementsMatch(t, []string{"image", "audio", "video"}, implementation.AssetCapability.MediaTypes)
	assert.Equal(t, 9, implementation.AssetCapability.MaxImages)
	assert.Equal(t, 3, implementation.AssetCapability.MaxAudio)
	assert.Equal(t, 3, implementation.AssetCapability.MaxVideos)
	assert.True(t, implementation.AssetCapability.Supports(LinkAssetResolutionSourceURL))
	assert.ElementsMatch(t, []string{
		VideoSKUSeedance20Mini720P,
		VideoSKUSeedance20SD2720P,
		VideoSKUSeedance20Fast720P,
		VideoSKUSeedance20Value720P,
		VideoSKUSeedance20Standard720P,
		VideoSKUSeedance20Value1080P,
		VideoSKUSeedance20Standard1080P,
		VideoSKUSeedance20Value4K,
		VideoSKUSeedance20Standard4K,
		VideoSKUSeedance20ProPI720P,
	}, implementation.PublicSKUs)

	mapping := `{"seedance-2.0-standard-720p":"seedance-2.0-vip-720p-azhw-feicai"}`
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

	highResolutionMapping := `{"seedance-2.0-standard-1080p":"seedance-2.0-vip-1080p-azhw-feicai"}`
	channel.Models = VideoSKUSeedance20Standard1080P
	channel.ModelMapping = &highResolutionMapping
	require.NoError(t, ValidateLinkImplementationRegistration(channel, &settings))

	badMapping := `{"seedance-2.0-standard-720p":"seedance-2.0-vip-720p-azhw-unknown"}`
	channel.Models = VideoSKUSeedance20Standard720P
	channel.ModelMapping = &badMapping
	require.Error(t, ValidateLinkImplementationRegistration(channel, &settings))
}

func TestMoxingV2ImplementationExcludesHistoricalArkAndRealPersonAssets(t *testing.T) {
	legacy, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationMoxingSeedanceArk, Version: LinkImplementationVersionV1,
	})
	require.True(t, ok)
	assert.True(t, legacy.Deprecated)
	legacyChannel := &Channel{Type: constant.ChannelTypeDoubaoVideo, Models: VideoSKUSeedance20Oversea, Status: common.ChannelStatusManuallyDisabled}
	legacySettings := dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyReverseProxy,
		AssetUpstreamProfile: dto.AssetUpstreamProfileArk,
		LinkImplementation:   dto.LinkImplementationRef{ID: legacy.ID, Version: legacy.Version},
	}
	require.ErrorContains(t, ValidateLinkImplementationRegistration(legacyChannel, &legacySettings), "deprecated")

	implementation, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationMoxingSeedanceMedia, Version: LinkImplementationVersionV2,
	})
	require.True(t, ok)
	assert.Equal(t, VideoProfileThirdPartyRelay, implementation.RequiredVideoProfile)
	assert.Equal(t, "54:third_party_relay:v2", implementation.RequiredAdapterVersion)
	assert.Equal(t, []string{AssetKindGeneral}, implementation.AssetCapability.AssetKinds)
	assert.Equal(t, []string{"image"}, implementation.AssetCapability.MediaTypes)
	assert.Equal(t, 9, implementation.AssetCapability.MaxImages)
	assert.ElementsMatch(t, []string{VideoSKUSeedance20Oversea}, implementation.PublicSKUs)
	assert.Len(t, LinkImplementationsForSKU(VideoSKUSeedance20Oversea), 1)
}

func TestTokenSaveV2ImplementationSupersedesV1WithoutReinterpretingIt(t *testing.T) {
	legacy, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationTokenSaveSeedance, Version: LinkImplementationVersionV1,
	})
	require.True(t, ok)
	assert.True(t, legacy.Deprecated)

	current, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationTokenSaveSeedance, Version: LinkImplementationVersionV2,
	})
	require.True(t, ok)
	assert.False(t, current.Deprecated)
	assert.Equal(t, "54:third_party_relay:v2", current.RequiredAdapterVersion)
	assert.Equal(t, []string{AssetKindGeneral}, current.AssetCapability.AssetKinds)
	assert.Equal(t, []string{"image"}, current.AssetCapability.MediaTypes)
	assert.Equal(t, 9, current.AssetCapability.MaxImages)

	selectable := LinkImplementationsForSKU(VideoSKUDoubaoSeedance20260128)
	require.Len(t, selectable, 1)
	assert.Equal(t, LinkImplementationVersionV2, selectable[0].Version)
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
		LinkImplementationBytePlusSeedanceArk + "/v1":  "sha256:eb24770e7f787d658b91739a18619149a9a73865ffdaafc20ff2493a9b1a1a99",
		LinkImplementationFeicaiSeedanceVideos + "/v2": "sha256:1a6506d9480a4ac0a4354ac7b4a715da9039faeb49488b909004dd8950495304",
		LinkImplementationFunCloudSeedance + "/v1":     "sha256:2f354b4a61b93ac954e53301b631613507ebc1ed90d136ea8cd5eb99a34cdb7b",
		LinkImplementationJimengVideos + "/v1":         "sha256:3426e5c65740859d53046a7d8582d323ea76c3af76ae23c14e97f6b028328623",
		LinkImplementationKlingVideos + "/v1":          "sha256:5902ab950dbcebaf8577ca4e6fc108dfd2559162cba06fb2f25452f489b56efb",
		LinkImplementationMoxingImages + "/v1":         "sha256:4b2f5b74b46db5091ae1150d1ec002d3bd0637335476ffb95460daec37edee38",
		LinkImplementationMoxingSeedanceArk + "/v1":    "sha256:5076adca885671d73bc5f60b6acad16e587d58646e7b66bba7aa829787fb9f45",
		LinkImplementationMoxingSeedanceMedia + "/v2":  "sha256:e78c318a0395689ab5b761d54a7976d47b94fccc96a13603ae361c35d30428cf",
		LinkImplementationQihangImages + "/v1":         "sha256:f42ef37e3acb7f494bc3ffeaea55852e6824931dca905082e122fcfecc7aa424",
		LinkImplementationTokenSaveSeedance + "/v1":    "sha256:cd55b6219fa2036f5ba8103cbfc278229673b34f2b0254f3428139d50f3f498a",
		LinkImplementationTokenSaveSeedance + "/v2":    "sha256:ce8f5401f8d7fdf969bc4e127eaa9463753fd042efe1253957b117c32103e0ee",
	}
	implementations := ListLinkImplementations()
	require.Len(t, implementations, len(expectedHashes))
	for _, registered := range implementations {
		assert.Equal(t, expectedHashes[registered.ID+"/"+registered.Version], registered.ContentHash)
	}
	implementation, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationMoxingSeedanceMedia, Version: LinkImplementationVersionV2,
	})
	require.True(t, ok)
	assert.Contains(t, implementation.ContentHash, "sha256:")

	changed := implementation
	changed.RequiredQueryPath = "/different/{task_id}"
	assert.NotEqual(t, implementation.ContentHash, linkImplementationContentHash(changed))

	implementationsForSKU := LinkImplementationsForSKU(VideoSKUSeedance20Oversea)
	assert.Len(t, implementationsForSKU, 1)
}
