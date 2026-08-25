package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetBoundaryTestChannel(name string, status int) *Channel {
	channel := seedanceTestChannel(name, status)
	channel.Name = name
	channel.BaseURL = common.GetPointer("https://assets.example.com")
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetProviderProject:  "project-a",
		AssetRegion:           "cn-beijing",
	})
	return channel
}

func TestAssetTenantBoundaryFieldsAreImmutableAfterIdentityCreation(t *testing.T) {
	withSeedanceChannelDB(t)

	tests := []struct {
		name   string
		mutate func(*Channel)
	}{
		{name: "channel type", mutate: func(channel *Channel) { channel.Type = constant.ChannelTypeOpenAI }},
		{name: "base url", mutate: func(channel *Channel) { channel.BaseURL = common.GetPointer("https://other.example.com") }},
		{name: "video protocol", mutate: func(channel *Channel) {
			settings := channel.GetOtherSettings()
			settings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolModelArkV3BytePlus
			channel.SetOtherSettings(settings)
		}},
		{name: "asset protocol", mutate: func(channel *Channel) {
			settings := channel.GetOtherSettings()
			settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolMoxingJoyCreatorV1
			channel.SetOtherSettings(settings)
		}},
		{name: "project", mutate: func(channel *Channel) {
			settings := channel.GetOtherSettings()
			settings.AssetProviderProject = "project-b"
			channel.SetOtherSettings(settings)
		}},
		{name: "region", mutate: func(channel *Channel) {
			settings := channel.GetOtherSettings()
			settings.AssetRegion = "ap-singapore"
			channel.SetOtherSettings(settings)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := assetBoundaryTestChannel("boundary-"+tt.name, common.ChannelStatusManuallyDisabled)
			require.NoError(t, channel.Insert())
			originalScope, err := ChannelAssetReuseScope(channel.Id)
			require.NoError(t, err)

			tt.mutate(channel)
			err = channel.UpdateWithActorAndAssetTenantConfirmation(0, true)
			require.ErrorIs(t, err, ErrAssetTenantBoundaryImmutable)

			stored, err := GetChannelById(channel.Id, true)
			require.NoError(t, err)
			storedScope, err := ChannelAssetReuseScope(channel.Id)
			require.NoError(t, err)
			assert.Equal(t, originalScope, storedScope)
			assert.Equal(t, "https://assets.example.com", stored.GetBaseURL())
			assert.Equal(t, constant.ChannelTypeSeedanceLink, stored.Type)
		})
	}
}

func TestAssetCredentialRotationRequiresExplicitTenantConfirmation(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := assetBoundaryTestChannel("credential-rotation", common.ChannelStatusManuallyDisabled)
	require.NoError(t, channel.Insert())
	originalScope, err := ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)

	channel.Key = "rotated-key"
	err = channel.UpdateWithActor(0)
	require.ErrorIs(t, err, ErrAssetTenantRotationUnconfirmed)

	channel.Key = "rotated-key"
	require.NoError(t, channel.UpdateWithActorAndAssetTenantConfirmation(0, true))
	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "rotated-key", stored.Key)
	rotatedScope, err := ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, originalScope, rotatedScope)
}

func TestFirstAssetProtocolActivationCreatesPermanentIdentity(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := seedanceTestChannel("first-asset-activation", common.ChannelStatusManuallyDisabled)
	require.NoError(t, channel.Insert())
	_, err := ChannelAssetReuseScope(channel.Id)
	require.ErrorIs(t, err, errChannelAssetScopeIdentityMissing)

	settings := channel.GetOtherSettings()
	settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolMoxingVolcAssetsV1
	channel.SetOtherSettings(settings)
	require.NoError(t, channel.UpdateWithActor(0))
	_, err = ChannelAssetReuseScope(channel.Id)
	require.NoError(t, err)

	settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone
	channel.SetOtherSettings(settings)
	err = channel.UpdateWithActorAndAssetTenantConfirmation(0, true)
	require.ErrorIs(t, err, ErrAssetTenantBoundaryImmutable)
}

func TestAssetScopeIdentityBackfillIncludesDisabledChannels(t *testing.T) {
	db := withSeedanceChannelDB(t)
	enabled := assetBoundaryTestChannel("backfill-enabled", common.ChannelStatusEnabled)
	disabled := assetBoundaryTestChannel("backfill-disabled", common.ChannelStatusManuallyDisabled)
	withoutAssets := seedanceTestChannel("backfill-none", common.ChannelStatusManuallyDisabled)
	require.NoError(t, db.Create(enabled).Error)
	require.NoError(t, db.Create(disabled).Error)
	require.NoError(t, db.Create(withoutAssets).Error)

	require.NoError(t, backfillChannelAssetScopeIdentities())
	_, err := ChannelAssetReuseScope(enabled.Id)
	require.NoError(t, err)
	_, err = ChannelAssetReuseScope(disabled.Id)
	require.NoError(t, err)
	_, err = ChannelAssetReuseScope(withoutAssets.Id)
	require.ErrorIs(t, err, errChannelAssetScopeIdentityMissing)
}

func TestAssetScopeIdentityIsUniqueAcrossChannels(t *testing.T) {
	db := withSeedanceChannelDB(t)
	require.NoError(t, db.Create(&ChannelAssetScopeIdentity{ChannelID: 1001, Identity: "same-identity"}).Error)
	err := db.Create(&ChannelAssetScopeIdentity{ChannelID: 1002, Identity: "same-identity"}).Error
	require.Error(t, err)
}
