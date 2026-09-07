package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFunCloudModelArkSharesExistingMaterialAcrossFourModels(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := seedanceTestChannel("v3-standard", common.ChannelStatusEnabled)
	channel.Models = "v3-standard,v3-fast,v3-mini,v3-next"
	channel.BaseURL = common.GetPointer("https://funcloud.example.com")
	channel.ModelMapping = common.GetPointer(`{"v3-standard":"seedance-2-0","v3-fast":"seedance-2-0-fast","v3-mini":"seedance-2-0-mini","v3-next":"seedance-2-5"}`)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudMaterial, AssetMinURLTTLSeconds: 3600})
	require.NoError(t, channel.ValidateSettings())
	require.NoError(t, channel.Insert())
	require.NoError(t, SaveChannelDefaultAssetGroup(channel.Id, "existing-provider-group"))
	scope, err := ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	for _, name := range channel.GetModels() {
		api, ok := seedancePublicModelAPI(name, dto.VideoUpstreamProtocolFunCloudModelArkV3, "seedance-2-5", false, dto.AssetUpstreamProtocolFunCloudMaterial, 3600, scope)
		require.True(t, ok)
		require.NotNil(t, api.Assets)
		// Equal inputs to the existing asset projection preserve its opaque reuse scope.
		assert.Equal(t, seedancePublicAssetAPI(name, dto.AssetUpstreamProtocolFunCloudMaterial, 3600, scope), *api.Assets)
	}
	channel.ModelMapping = common.GetPointer(`{"v3-standard":"seedance-2","v3-fast":"seedance-2-0-fast","v3-mini":"seedance-2-0-mini","v3-next":"seedance-2-5"}`)
	require.ErrorContains(t, channel.ValidateSettings(), "model_mapping")
}

func TestFunCloudVideoUpgradeRequiresTenantConfirmation(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := seedanceTestChannel("upgrade-customer", common.ChannelStatusEnabled)
	channel.BaseURL = common.GetPointer("https://funcloud.example.com")
	channel.ModelMapping = common.GetPointer(`{"upgrade-customer":"seedance-2"}`)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudSeedance, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudMaterial, AssetMinURLTTLSeconds: 3600})
	require.NoError(t, channel.Insert())
	require.NoError(t, SaveChannelDefaultAssetGroup(channel.Id, "existing-provider-group"))
	scope, err := ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	settings := channel.GetOtherSettings()
	settings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolFunCloudModelArkV3
	channel.SetOtherSettings(settings)
	channel.ModelMapping = common.GetPointer(`{"upgrade-customer":"seedance-2-0"}`)
	require.ErrorIs(t, channel.UpdateWithActorAndAssetTenantConfirmation(0, false, false), ErrAssetTenantReplacementUnconfirmed)
	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, dto.VideoUpstreamProtocolFunCloudSeedance, stored.GetOtherSettings().VideoUpstreamProtocol)
	group, err := GetChannelDefaultAssetGroup(channel.Id)
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NoError(t, channel.UpdateWithActorAndAssetTenantConfirmation(0, false, true))
	newScope, err := ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.NotEqual(t, scope, newScope)
	group, err = GetChannelDefaultAssetGroup(channel.Id)
	require.NoError(t, err)
	assert.Nil(t, group)
	// Explicitly associating the original opaque group is still permitted.
	require.NoError(t, SaveChannelDefaultAssetGroup(channel.Id, "existing-provider-group"))
}
