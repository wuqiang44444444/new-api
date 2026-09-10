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
		{VideoUpstreamProtocolModelArkV3CMCC, VideoUpstreamProfileOfficial},
		{VideoUpstreamProtocolTokenSaveMediaTaskV1, VideoUpstreamProfileThirdPartyRelay},
		{VideoUpstreamProtocolMoxingModelArkV1, VideoUpstreamProfileThirdPartyMoxingModelArk},
		{VideoUpstreamProtocolArkMediaV1, VideoUpstreamProfileThirdPartyReverseProxy},
		{VideoUpstreamProtocolFeicaiVideosV1, VideoUpstreamProfileThirdPartyFeicaiVideos},
		{VideoUpstreamProtocolFunCloudModelArkV3, VideoUpstreamProfileThirdPartyFunCloudModelArkV3},
	}

	for _, test := range tests {
		t.Run(string(test.protocol), func(t *testing.T) {
			require.NoError(t, ValidateVideoUpstreamProtocol(test.protocol))
			assert.Equal(t, test.profile, test.protocol.TransportProfile())
		})
	}
	require.Error(t, ValidateVideoUpstreamProtocol("administrator_json"))
	require.Error(t, ValidateVideoUpstreamProtocol("media_task_v1"))
	require.Error(t, ValidateVideoUpstreamProtocol("funcloud_seedance_v2"))
	require.ErrorContains(t, ValidateVideoUpstreamProtocol(VideoUpstreamProtocolFunCloudSeedance), "retired")
}

func TestSeedanceVideoProtocolsResolveFixedPaths(t *testing.T) {
	tests := []struct {
		name          string
		protocol      VideoUpstreamProtocol
		providerModel string
		createPath    string
		queryPath     string
	}{
		{"TokenSave media task", VideoUpstreamProtocolTokenSaveMediaTaskV1, "seedance", "/v1/media/generations", "/v1/media/tasks/{task_id}"},
		{"Moxing ModelArk", VideoUpstreamProtocolMoxingModelArkV1, "seedance", "/v1/media/generations", "/v1/media/tasks/{task_id}"},
		{"ark media", VideoUpstreamProtocolArkMediaV1, "seedance", "/v1/ark/media/generations", "/v1/ark/media/tasks/{task_id}"},
		{"Feicai videos", VideoUpstreamProtocolFeicaiVideosV1, "seedance", "/v1/videos", "/v1/videos/{task_id}"},
		{"retired funcloud query only", VideoUpstreamProtocolFunCloudSeedance, "seedance-2", "", "/api/v2/open/aigc/{task_id}"},
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
		AssetUpstreamProtocolTokenSaveAssetsV1,
		AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetUpstreamProtocolFunCloudMaterial,
		AssetUpstreamProtocolCMCCAICCV2,
	} {
		require.NoError(t, ValidateAssetUpstreamProtocol(protocol))
		assert.True(t, protocol.TransportProfile().IsValid())
	}
	require.Error(t, ValidateAssetUpstreamProtocol("automatic"))
	require.Error(t, ValidateAssetUpstreamProtocol("relay_assets_v1"))
	require.Error(t, ValidateAssetUpstreamProtocol("funcloud_material_v2"))
}
