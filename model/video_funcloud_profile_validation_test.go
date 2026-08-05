package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestValidateFunCloudVideoProfileChannelSeparatesStandardAndFast(t *testing.T) {
	baseURL := funCloudBaseURL
	settings := &dto.ChannelOtherSettings{
		VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2,
		VideoUpstreamCreatePath:        funCloudFastCreatePath,
		VideoUpstreamQueryPathTemplate: funCloudQueryPath,
	}
	channel := &Channel{Type: constant.ChannelTypeDoubaoVideo, BaseURL: &baseURL, Models: VideoSKUSeedance20Fast}
	require.NoError(t, validateFunCloudVideoProfileChannel(channel, settings))

	channel.Models = VideoSKUSeedance20Standard
	require.Error(t, validateFunCloudVideoProfileChannel(channel, settings))

	settings.VideoUpstreamCreatePath = funCloudStandardCreatePath
	require.NoError(t, validateFunCloudVideoProfileChannel(channel, settings))

	legacyBaseURL := "https://mm-accelerate.leonecloud.com"
	channel.BaseURL = &legacyBaseURL
	require.ErrorContains(t, validateFunCloudVideoProfileChannel(channel, settings), funCloudBaseURL)
}

func TestFunCloudImplementationMatchesRoleRestrictedCapabilities(t *testing.T) {
	baseURL := funCloudBaseURL
	for _, test := range []struct {
		model      string
		createPath string
	}{
		{model: VideoSKUSeedance20Standard, createPath: funCloudStandardCreatePath},
		{model: VideoSKUSeedance20Fast, createPath: funCloudFastCreatePath},
	} {
		t.Run(test.model, func(t *testing.T) {
			channel := &Channel{
				Type: constant.ChannelTypeDoubaoVideo, BaseURL: &baseURL, Models: test.model,
			}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2,
				VideoUpstreamCreatePath:        test.createPath,
				VideoUpstreamQueryPathTemplate: funCloudQueryPath,
				AssetUpstreamProfile:           dto.AssetUpstreamProfileNone,
				LinkImplementation: dto.LinkImplementationRef{
					ID: LinkImplementationFunCloudSeedance, Version: LinkImplementationVersionV1,
				},
			})
			capability, ok := ResolveVideoSKUCapability(test.model)
			require.True(t, ok)
			require.NoError(t, ValidateVideoSKUImplementation(capability, channel))
			require.NoError(t, ValidateLinkSKUAbilityBindings(channel))
		})
	}
}
