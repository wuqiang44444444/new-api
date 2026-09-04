package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentVideoSouthboundAdapterVersionSelectsRegisteredProfiles(t *testing.T) {
	assert.Equal(t,
		"62:official:v1",
		CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, ""),
	)
	assert.Equal(t,
		"62:third_party_relay:v2",
		CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyRelay),
	)
}

func TestResolveFeicaiAdapterUsesV2AndRetainsV1TaskCompatibility(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyFeicaiVideos
	assert.Equal(t, "62:third_party_feicai_videos:v2", CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile))

	_, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "")
	require.Error(t, err)
	v1, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		profile,
		"62:third_party_feicai_videos:v1",
	)
	require.NoError(t, err)
	assert.True(t, v1.IsFeicaiVideosV1())
	assert.True(t, v1.IsFeicaiVideos())

	v2, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		profile,
		"62:third_party_feicai_videos:v2",
	)
	require.NoError(t, err)
	assert.True(t, v2.IsFeicaiVideosV2())
	assert.True(t, v2.IsFeicaiVideos())

	_, err = ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		profile,
		"62:third_party_feicai_videos:v3",
	)
	require.Error(t, err)
}

func TestResolveSeedanceThirdPartyRelayRequiresV2(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyRelay
	_, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "")
	require.Error(t, err)

	v2, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "62:third_party_relay:v2")
	require.NoError(t, err)
	assert.True(t, v2.IsThirdPartyRelayV2())

	_, err = ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "62:third_party_relay:v1")
	require.Error(t, err)
}

func TestResolveFunCloudAdapterOnlyAcceptsCurrentV3(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyFunCloudSeedance
	assert.Equal(t, "62:third_party_funcloud_seedance:v3", CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile))

	for _, frozen := range []string{"", "62:third_party_funcloud_seedance:v1", "62:third_party_funcloud_seedance:v2", "62:third_party_funcloud_seedance:v4"} {
		_, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, frozen)
		require.Error(t, err, frozen)
	}

	current, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "62:third_party_funcloud_seedance:v3")
	require.NoError(t, err)
	assert.Equal(t, VideoAdapterRevisionV3, current.Revision)
	assert.True(t, current.IsFunCloudSeedanceV3())
}
