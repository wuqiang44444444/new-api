package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentVideoSouthboundAdapterVersionSelectsOmniV2Only(t *testing.T) {
	assert.Equal(t,
		"54:third_party_json_video_omni_reference:v2",
		CurrentVideoSouthboundAdapterVersion(
			constant.ChannelTypeDoubaoVideo,
			dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
		),
	)
	assert.Equal(t,
		"54:official:v1",
		CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeDoubaoVideo, ""),
	)
}

func TestResolveVideoSouthboundAdapterVersionDefaultsOmniToV2AndFailsClosed(t *testing.T) {
	omni, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
		"",
	)
	require.NoError(t, err)
	assert.True(t, omni.IsJSONVideoOmniV2())

	official, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		"",
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, VideoAdapterRevisionV1, official.Revision)

	v2, err := ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
		"54:third_party_json_video_omni_reference:v2",
	)
	require.NoError(t, err)
	assert.True(t, v2.IsJSONVideoOmniV2())

	for _, frozen := range []string{
		"54:third_party_json_video_omni_reference:v1",
		"54:third_party_json_video_omni_reference:v3",
		"54:official:v2",
		"18:third_party_json_video_omni_reference:v2",
		"malformed",
	} {
		_, err := ResolveVideoSouthboundAdapterVersion(
			constant.ChannelTypeDoubaoVideo,
			dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
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
}
