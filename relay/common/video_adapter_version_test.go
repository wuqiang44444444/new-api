package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentVideoSouthboundAdapterVersionSelectsMediaArraysV1(t *testing.T) {
	assert.Equal(t,
		"54:third_party_json_video_media_arrays:v1",
		CurrentVideoSouthboundAdapterVersion(
			constant.ChannelTypeDoubaoVideo,
			dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		),
	)
	assert.Equal(t,
		"54:official:v1",
		CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeDoubaoVideo, ""),
	)
}

func TestResolveVideoSouthboundAdapterVersionRequiresMediaArraysV1AndFailsClosed(t *testing.T) {
	_, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		"",
	)
	require.Error(t, err)

	official, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		"",
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, VideoAdapterRevisionV1, official.Revision)

	v1, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		"54:third_party_json_video_media_arrays:v1",
	)
	require.NoError(t, err)
	assert.True(t, v1.IsJSONVideoMediaArraysV1())

	for _, frozen := range []string{
		"54:third_party_json_video_media_arrays:v2",
		"54:third_party_json_video_media_arrays:v3",
		"54:official:v2",
		"18:third_party_json_video_media_arrays:v1",
		"malformed",
	} {
		_, err := ResolveVideoSouthboundAdapterVersion(
			constant.ChannelTypeDoubaoVideo,
			dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
			frozen,
		)
		require.Error(t, err, frozen)
	}
}

func TestResolveFunCloudAdapterRequiresOnlyV2Snapshot(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2
	assert.Equal(t, "54:third_party_funcloud_seedance_v2:v2", CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeDoubaoVideo, profile))

	for _, frozen := range []string{"", "54:third_party_funcloud_seedance_v2:v1", "54:third_party_funcloud_seedance_v2:v3"} {
		_, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeDoubaoVideo, profile, frozen)
		require.Error(t, err, frozen)
	}

	version, err := ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeDoubaoVideo, profile, "54:third_party_funcloud_seedance_v2:v2")
	require.NoError(t, err)
	assert.Equal(t, VideoAdapterRevisionV2, version.Revision)
	assert.True(t, version.IsFunCloudSeedanceV2())
}
