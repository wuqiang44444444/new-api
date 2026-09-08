package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkChannelModelMappingAndHostedProjection(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := seedanceTestChannel("synlink-customer", common.ChannelStatusEnabled)
	channel.BaseURL = common.GetPointer("http://provider.example")
	settings := dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted}
	channel.SetOtherSettings(settings)
	for _, providerModel := range kitdto.SynlinkVideoModels() {
		mapping, err := common.Marshal(map[string]string{"synlink-customer": providerModel})
		require.NoError(t, err)
		channel.ModelMapping = common.GetPointer(string(mapping))
		require.NoError(t, channel.ValidateSettings())
		api, ok := seedancePublicModelAPI("synlink-customer", settings.VideoUpstreamProtocol, providerModel, false, settings.AssetUpstreamProtocol, 0, "unrelated-channel-scope")
		require.True(t, ok)
		assert.Equal(t, "platform_hosted", api.Assets.ManagementMode)
		assert.Equal(t, hostedPlatformReuseScope, api.Assets.ReuseScope)
		require.Len(t, api.Assets.Media, 1)
		assert.Equal(t, "image", api.Assets.Media[0].MediaType)
		encoded, err := common.Marshal(api)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), providerModel)
		assert.NotContains(t, string(encoded), "synlink_video_v1")
	}
	channel.ModelMapping = common.GetPointer(`{"synlink-customer":"doubao-seedance-2-0-mini-260615-max"}`)
	require.Error(t, channel.ValidateSettings())
	channel.ModelMapping = common.GetPointer(`{"synlink-customer":"doubao-seedance-2-0-260128"}`)
	settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolFunCloudMaterial
	channel.SetOtherSettings(settings)
	require.Error(t, channel.ValidateSettings())
}
