package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestValidateJSONVideoMediaArraysChannelPublishesOnlyEquivalentSKUs(t *testing.T) {
	httpsBaseURL := "https://video.example.com"
	httpBaseURL := "http://video.example.com"
	pathBaseURL := "https://video.example.com/provider-root"
	settings := &dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	}

	require.NoError(t, validateJSONVideoMediaArraysChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  VideoSKUSeedance20Standard720P,
		BaseURL: &httpsBaseURL,
	}, settings))

	require.Error(t, validateJSONVideoMediaArraysChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  "seedance-2.0-vip-720p-azhw",
		BaseURL: &httpsBaseURL,
	}, settings))

	require.Error(t, validateJSONVideoMediaArraysChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  VideoSKUSeedance20Standard720P,
		BaseURL: &httpBaseURL,
	}, settings))

	require.Error(t, validateJSONVideoMediaArraysChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  VideoSKUSeedance20Standard720P,
		BaseURL: &pathBaseURL,
	}, settings))

	require.Error(t, validateJSONVideoMediaArraysChannel(&Channel{
		Type:    constant.ChannelTypeDoubaoVideo,
		Models:  VideoSKUSeedance20Standard720P,
		BaseURL: &httpsBaseURL,
	}, &dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial,
	}))
}
