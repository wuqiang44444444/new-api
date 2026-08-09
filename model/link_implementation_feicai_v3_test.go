package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeicaiV2AndV3RemainSelectableWithDisjointProviderModels(t *testing.T) {
	v2, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV2,
	})
	require.True(t, ok)
	v3, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV3,
	})
	require.True(t, ok)

	assert.False(t, v2.Deprecated)
	assert.False(t, v3.Deprecated)
	assert.NotEqual(t, v2.PlanName, v3.PlanName)
	assert.Equal(t, v2.PublicSKUs, v3.PublicSKUs)
	assert.NotEqual(t, v2.ContentHash, v3.ContentHash)

	v2ModelsBySKU := make(map[string]string, len(v2.ExecutionBindings))
	for _, binding := range v2.ExecutionBindings {
		v2ModelsBySKU[binding.LinkSKU] = binding.ProviderModel
	}
	v3ModelsBySKU := make(map[string]string, len(v3.ExecutionBindings))
	for _, binding := range v3.ExecutionBindings {
		v3ModelsBySKU[binding.LinkSKU] = binding.ProviderModel
	}
	require.Len(t, v2ModelsBySKU, 10)
	require.Len(t, v3ModelsBySKU, 10)
	assert.Equal(t, map[string]string{
		VideoSKUSeedance20Mini720P:      FeicaiV3ProviderModelSeedance20Mini720P,
		VideoSKUSeedance20SD2720P:       FeicaiV3ProviderModelSeedance20SD2720P,
		VideoSKUSeedance20Fast720P:      FeicaiV3ProviderModelSeedance20Fast720P,
		VideoSKUSeedance20Value720P:     FeicaiV3ProviderModelSeedance20Value720P,
		VideoSKUSeedance20Standard720P:  FeicaiV3ProviderModelSeedance20Standard720P,
		VideoSKUSeedance20Value1080P:    FeicaiV3ProviderModelSeedance20Value1080P,
		VideoSKUSeedance20Standard1080P: FeicaiV3ProviderModelSeedance20Standard1080P,
		VideoSKUSeedance20Value4K:       FeicaiV3ProviderModelSeedance20Value4K,
		VideoSKUSeedance20Standard4K:    FeicaiV3ProviderModelSeedance20Standard4K,
		VideoSKUSeedance20ProPI720P:     FeicaiV3ProviderModelSeedance20ProPI720P,
	}, v3ModelsBySKU)
	for linkSKU, v3ProviderModel := range v3ModelsBySKU {
		assert.Equal(t, v3ProviderModel+"-feicai", v2ModelsBySKU[linkSKU])
	}

	implementations := LinkImplementationsForSKU(VideoSKUSeedance20Mini720P)
	require.Len(t, implementations, 2)
	assert.Equal(t, []string{LinkImplementationVersionV2, LinkImplementationVersionV3}, []string{
		implementations[0].Version,
		implementations[1].Version,
	})
}

func TestFeicaiV3AcceptsOnlyUnsuffixedMappingsAndFailsClosedWithoutEvidence(t *testing.T) {
	v2Settings := dto.ChannelOtherSettings{
		VideoUpstreamProfile:    dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		VideoUpstreamCreatePath: "/v1/videos", VideoUpstreamQueryPathTemplate: "/v1/videos/{task_id}",
		AssetUpstreamProfile: dto.AssetUpstreamProfileNone,
		LinkImplementation: dto.LinkImplementationRef{
			ID: LinkImplementationFeicaiSeedanceVideos, Version: LinkImplementationVersionV2,
		},
	}
	v3Settings := v2Settings
	v3Settings.LinkImplementation.Version = LinkImplementationVersionV3

	channel := &Channel{
		Type:   constant.ChannelTypeDoubaoVideo,
		Models: VideoSKUSeedance20Mini720P,
		Status: common.ChannelStatusManuallyDisabled,
	}
	channel.ModelMapping = common.GetPointer(`{"seedance-2.0-mini-720p":"seedance-2.0-vip-720p-mini-azhw-feicai"}`)
	require.NoError(t, ValidateLinkImplementationRegistration(channel, &v2Settings))
	require.Error(t, ValidateLinkImplementationRegistration(channel, &v3Settings))

	channel.ModelMapping = common.GetPointer(`{"seedance-2.0-mini-720p":"seedance-2.0-vip-720p-mini-azhw"}`)
	require.Error(t, ValidateLinkImplementationRegistration(channel, &v2Settings))
	require.NoError(t, ValidateLinkImplementationRegistration(channel, &v3Settings))

	channel.Status = common.ChannelStatusEnabled
	channel.SetOtherSettings(v3Settings)
	require.NoError(t, ValidateLinkSKUAbilityBindings(channel))
	require.ErrorContains(
		t,
		ValidateLinkSKUAbilityPublicationReadiness(channel),
		"no verified provider ratio/size evidence",
	)
}
