package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestValidateFunCloudVideoProfileChannelSeparatesStandardAndFast(t *testing.T) {
	baseURL := "https://mm.example.com"
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
}
