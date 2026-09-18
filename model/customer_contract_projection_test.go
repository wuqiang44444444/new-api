package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCustomerContractMetadataUsesOnlyAllowedChannelCapabilities(t *testing.T) {
	resetPricingEndpointTestTables(t)
	initCol()
	low, high := int64(1), int64(20)
	channels := []Channel{
		{Id: 811, Type: constant.ChannelTypeOpenAI, Group: "default", Models: "contract-media", Status: common.ChannelStatusEnabled, ModelMapping: common.GetPointer(`{"contract-media":"gpt-image-1"}`), Priority: &low},
		{Id: 812, Type: constant.ChannelTypeSora, Group: "other", Models: "contract-media", Status: common.ChannelStatusEnabled, Priority: &high},
	}
	for _, channel := range channels {
		require.NoError(t, DB.Create(&channel).Error)
		require.NoError(t, DB.Create(&Ability{Group: channel.Group, Model: "contract-media", ChannelId: channel.Id, Enabled: true, Priority: channel.Priority}).Error)
	}
	rule := ContractEntityRule{PublicModel: "contract-media", ChannelId: 811, RouteGroup: "default"}
	metadata, err := GetContractModelMetadata([]ContractEntityRule{rule})
	require.NoError(t, err)
	require.NotNil(t, metadata["contract-media"].API)
	assert.NotNil(t, metadata["contract-media"].API.Image)
	assert.Nil(t, metadata["contract-media"].API.Video)
	assert.NotContains(t, metadata["contract-media"].SupportedEndpointTypes, constant.EndpointTypeOpenAIVideo)
	// Once explicitly allowed, the higher-priority other-group source wins
	// media projection, consistently with flattened runtime selection.
	metadata, err = GetContractModelMetadata([]ContractEntityRule{rule, {PublicModel: "contract-media", ChannelId: 812, RouteGroup: "other"}})
	require.NoError(t, err)
	require.NotNil(t, metadata["contract-media"].API)
	assert.NotNil(t, metadata["contract-media"].API.Video)
}
