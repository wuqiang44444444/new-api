package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentVideoSouthboundAdapterVersionSelectsMediaArraysV2(t *testing.T) {
	assert.Equal(t,
		"61:third_party_json_video_media_arrays:v2",
		CurrentVideoSouthboundAdapterVersion(
			constant.ChannelTypeSeedanceLink,
			dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		),
	)
	assert.Equal(t,
		"61:official:v1",
		CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, ""),
	)
	assert.Equal(t,
		"61:third_party_relay:v2",
		CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyRelay),
	)
}

func TestResolveSeedanceThirdPartyRelayRequiresV2(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyRelay
	_, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "")
	require.Error(t, err)

	v2, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "61:third_party_relay:v2")
	require.NoError(t, err)
	assert.True(t, v2.IsThirdPartyRelayV2())

	_, err = ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "61:third_party_relay:v1")
	require.Error(t, err)
}

func TestResolveVideoSouthboundAdapterVersionRequiresMediaArraysV2AndFailsClosed(t *testing.T) {
	_, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		"",
	)
	require.Error(t, err)

	official, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		"",
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, VideoAdapterRevisionV1, official.Revision)

	v2, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		"61:third_party_json_video_media_arrays:v2",
	)
	require.NoError(t, err)
	assert.True(t, v2.IsJSONVideoMediaArraysV2())

	for _, frozen := range []string{
		"61:third_party_json_video_media_arrays:v1",
		"61:third_party_json_video_media_arrays:v3",
		"61:official:v2",
		"18:third_party_json_video_media_arrays:v1",
		"malformed",
	} {
		_, err := ResolveVideoSouthboundAdapterVersion(
			constant.ChannelTypeSeedanceLink,
			dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
			frozen,
		)
		require.Error(t, err, frozen)
	}
}

func TestResolveFunCloudAdapterRequiresOnlyV2Snapshot(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2
	assert.Equal(t, "61:third_party_funcloud_seedance_v2:v2", CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile))

	for _, frozen := range []string{"", "61:third_party_funcloud_seedance_v2:v1", "61:third_party_funcloud_seedance_v2:v3"} {
		_, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, frozen)
		require.Error(t, err, frozen)
	}

	version, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "61:third_party_funcloud_seedance_v2:v2")
	require.NoError(t, err)
	assert.Equal(t, VideoAdapterRevisionV2, version.Revision)
	assert.True(t, version.IsFunCloudSeedanceV2())
}
