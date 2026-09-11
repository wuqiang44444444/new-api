package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoxing0818MappingRequiresExactRegisteredModel(t *testing.T) {
	withSeedanceChannelDB(t)
	seedPublishedSeedanceTestArtifact(t)
	for _, providerModel := range []string{
		"doubao-seedance-2-0-260128-0818",
		"doubao-seedance-2-0-260128",
		"doubao-seedance-2-0-unregistered",
	} {
		t.Run(providerModel, func(t *testing.T) {
			channel := seedanceTestChannel("customer-video", common.ChannelStatusEnabled)
			channel.BaseURL = common.GetPointer("https://provider.example.com")
			mapping, err := common.Marshal(map[string]string{"customer-video": providerModel})
			require.NoError(t, err)
			channel.ModelMapping = common.GetPointer(string(mapping))
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				VideoUpstreamProtocol: dto.VideoUpstreamProtocolMoxingModelArkV1,
				AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
				AssetMinURLTTLSeconds: 3600, AssetProviderProject: "default",
			})
			err = channel.ValidateSettings()
			if err == nil {
				err = validateSeedancePublishedChannelConfiguration(DB, channel)
			}
			if providerModel == "doubao-seedance-2-0-260128-0818" {
				require.NoError(t, err)
				assert.Equal(t, "customer-video", channel.Models)
			} else {
				require.ErrorContains(t, err, "mapped Provider model")
			}
		})
	}
}
