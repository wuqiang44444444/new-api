package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduHostedChannelConfigurationAndPublicAssets(t *testing.T) {
	withSeedanceChannelDB(t)
	seedPublishedSeedanceTestArtifact(t)
	for _, region := range []string{"cn", "global"} {
		for _, variant := range []string{"viduq3.1-drama-std", "viduq3-drama-std", "viduq3-drama-fast", "viduq3-drama-mini"} {
			providerModel := variant
			if region == "global" {
				providerModel = strings.Replace(variant, "-drama-", "-drama-ab-", 1)
			}
			t.Run(providerModel, func(t *testing.T) {
				channel := seedanceTestChannel("customer-image", common.ChannelStatusEnabled)
				channel.BaseURL = common.GetPointer("https://provider.example")
				settings := dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted}
				channel.SetOtherSettings(settings)
				mapping, err := common.Marshal(map[string]string{"customer-image": providerModel})
				require.NoError(t, err)
				channel.ModelMapping = common.GetPointer(string(mapping))
				require.NoError(t, channel.ValidateSettings())
				require.NoError(t, validateSeedancePublishedChannelConfiguration(DB, channel))
				api, ok := seedancePublicModelAPI("customer-image", settings.VideoUpstreamProtocol, providerModel, false, settings.AssetUpstreamProtocol, 0, "channel-specific")
				require.True(t, ok)
				assert.True(t, api.Assets.Supported)
				assert.Equal(t, "platform_hosted", api.Assets.ManagementMode)
				assert.Equal(t, hostedPlatformReuseScope, api.Assets.ReuseScope)
				require.Len(t, api.Assets.Media, 1)
				assert.Equal(t, "image", api.Assets.Media[0].MediaType)
				encoded, err := common.Marshal(api)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), providerModel)
				assert.NotContains(t, string(encoded), "vidu_modelark_v3")
				settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone
				channel.SetOtherSettings(settings)
				require.NoError(t, validateSeedancePublishedChannelConfiguration(DB, channel), "existing URL-only channels remain valid")
				settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolFunCloudMaterial
				channel.SetOtherSettings(settings)
				require.Error(t, validateSeedancePublishedChannelConfiguration(DB, channel))
			})
		}
	}
}
