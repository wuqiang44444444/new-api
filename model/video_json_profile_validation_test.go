package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestValidateJSONVideoProfileChannelPublishesOnlyEquivalentSKUs(t *testing.T) {
	httpsBaseURL := "https://video.example.com"
	httpBaseURL := "http://video.example.com"
	settings := &dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
	}

	require.NoError(t, validateJSONVideoProfileChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  VideoSKUSeedance20Standard720P,
		BaseURL: &httpsBaseURL,
	}, settings))

	require.Error(t, validateJSONVideoProfileChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  "seedance-2.0-vip-720p-azhw-feicai",
		BaseURL: &httpsBaseURL,
	}, settings))

	require.Error(t, validateJSONVideoProfileChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  VideoSKUSeedance20Standard720P,
		BaseURL: &httpBaseURL,
	}, settings))

	require.Error(t, validateJSONVideoProfileChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  VideoSKUSeedance20Standard720P,
		BaseURL: &httpsBaseURL,
	}, &dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial,
	}))
}
