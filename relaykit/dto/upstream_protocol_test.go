package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceVideoProtocolsResolveCodeBackedTransport(t *testing.T) {
	tests := []struct {
		protocol VideoUpstreamProtocol
		profile  VideoUpstreamProfile
	}{
		{VideoUpstreamProtocolModelArkV3Volcengine, VideoUpstreamProfileOfficial},
		{VideoUpstreamProtocolModelArkV3BytePlus, VideoUpstreamProfileOfficial},
		{VideoUpstreamProtocolMediaTaskV1, VideoUpstreamProfileThirdPartyRelay},
		{VideoUpstreamProtocolArkMediaV1, VideoUpstreamProfileThirdPartyReverseProxy},
		{VideoUpstreamProtocolMediaArraysV2, VideoUpstreamProfileThirdPartyJSONVideoMediaArrays},
		{VideoUpstreamProtocolFunCloudSeedanceV2, VideoUpstreamProfileThirdPartyFunCloudSeedanceV2},
	}

	for _, test := range tests {
		t.Run(string(test.protocol), func(t *testing.T) {
			require.NoError(t, ValidateVideoUpstreamProtocol(test.protocol))
			assert.Equal(t, test.profile, test.protocol.TransportProfile())
		})
	}
	require.Error(t, ValidateVideoUpstreamProtocol("administrator_json"))
}

func TestSeedanceVideoProtocolsResolveFixedPaths(t *testing.T) {
	tests := []struct {
		name          string
		protocol      VideoUpstreamProtocol
		providerModel string
		createPath    string
		queryPath     string
	}{
		{"media task", VideoUpstreamProtocolMediaTaskV1, "seedance", "/v1/media/generations", "/v1/media/tasks/{task_id}"},
		{"ark media", VideoUpstreamProtocolArkMediaV1, "seedance", "/v1/ark/media/generations", "/v1/ark/media/tasks/{task_id}"},
		{"media arrays", VideoUpstreamProtocolMediaArraysV2, "seedance", "/v1/videos", "/v1/videos/{task_id}"},
		{"funcloud standard", VideoUpstreamProtocolFunCloudSeedanceV2, "seedance-2.0", "/api/v2/open/aigc/seedance2-0", "/api/v2/open/aigc/{task_id}"},
		{"funcloud fast", VideoUpstreamProtocolFunCloudSeedanceV2, "seedance-2.0-fast", "/api/v2/open/aigc/seedance2-0-fast", "/api/v2/open/aigc/{task_id}"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			createPath, queryPath := test.protocol.TransportPaths(test.providerModel)
			assert.Equal(t, test.createPath, createPath)
			assert.Equal(t, test.queryPath, queryPath)
		})
	}
}

func TestSeedanceAssetProtocolsAreExplicit(t *testing.T) {
	for _, protocol := range []AssetUpstreamProtocol{
		AssetUpstreamProtocolNone,
		AssetUpstreamProtocolVolcengineAction,
		AssetUpstreamProtocolBytePlusAction,
		AssetUpstreamProtocolArkAssetsV1,
		AssetUpstreamProtocolRelayAssetsV1,
	} {
		require.NoError(t, ValidateAssetUpstreamProtocol(protocol))
	}
	require.Error(t, ValidateAssetUpstreamProtocol("automatic"))
}
