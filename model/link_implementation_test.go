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
		LinkImplementationBytePlusSeedanceArk:  "sha256:eb24770e7f787d658b91739a18619149a9a73865ffdaafc20ff2493a9b1a1a99",
		LinkImplementationFeicaiSeedanceVideos: "sha256:2126cded7a1e8007f6affb2083a266d9fda785f92dcb0675175191838d9df987",
		LinkImplementationFunCloudSeedance:     "sha256:2f354b4a61b93ac954e53301b631613507ebc1ed90d136ea8cd5eb99a34cdb7b",
		LinkImplementationJimengVideos:         "sha256:3426e5c65740859d53046a7d8582d323ea76c3af76ae23c14e97f6b028328623",
		LinkImplementationKlingVideos:          "sha256:5902ab950dbcebaf8577ca4e6fc108dfd2559162cba06fb2f25452f489b56efb",
		LinkImplementationMoxingImages:         "sha256:4b2f5b74b46db5091ae1150d1ec002d3bd0637335476ffb95460daec37edee38",
		LinkImplementationMoxingSeedanceArk:    "sha256:5076adca885671d73bc5f60b6acad16e587d58646e7b66bba7aa829787fb9f45",
		LinkImplementationMoxingSeedanceMedia:  "sha256:3f78ff7d430ae9ca9461ef10ca34131e9ae89a71c210b9401951baab68bca92d",
		LinkImplementationQihangImages:         "sha256:f42ef37e3acb7f494bc3ffeaea55852e6824931dca905082e122fcfecc7aa424",
		LinkImplementationTokenSaveSeedance:    "sha256:cd55b6219fa2036f5ba8103cbfc278229673b34f2b0254f3428139d50f3f498a",
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
