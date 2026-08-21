package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cmccSeedanceTestChannel(customerModel string, status int) *Channel {
	channel := &Channel{
		Type: constant.ChannelTypeSeedanceLink, Status: status, Name: customerModel,
		Models: customerModel, Group: "default", Key: "video-key",
		BaseURL:      common.GetPointer("https://zhenze-huhehaote.cmecloud.cn"),
		ModelMapping: common.GetPointer(`{"` + customerModel + `":"` + CMCCSeedance20ProviderModel + `"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3CMCC,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolCMCCAICCV2,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "must-be-cleared", AssetRegion: "must-be-cleared",
	})
	return channel
}

func TestCMCCChannelValidationAndStableAssetScope(t *testing.T) {
	db := withSeedanceChannelDB(t)
	require.NoError(t, db.AutoMigrate(&ChannelAssetCredential{}, &ChannelAssetScopeIdentity{}))
	channel := cmccSeedanceTestChannel("seedance-2.0-cmcc", common.ChannelStatusEnabled)
	require.NoError(t, channel.ValidateSettings())
	settings := channel.GetOtherSettings()
	assert.Empty(t, settings.AssetProviderProject)
	assert.Empty(t, settings.AssetRegion)
	require.NoError(t, InsertChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{
		AccessKeyID: "asset-access", SecretAccessKey: "asset-secret",
	}))

	firstScope, err := CMCCAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.NotEmpty(t, firstScope)
	require.NoError(t, UpdateChannelWithAssetCredential(channel, &dto.ChannelAssetCredentialInput{
		AccessKeyID: "rotated-access", SecretAccessKey: "rotated-secret",
	}))
	rotatedScope, err := CMCCAssetReuseScope(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, firstScope, rotatedScope)

	second := cmccSeedanceTestChannel("seedance-2.0-cmcc-private", common.ChannelStatusManuallyDisabled)
	require.NoError(t, second.ValidateSettings())
	require.NoError(t, InsertChannelWithAssetCredential(second, &dto.ChannelAssetCredentialInput{
		AccessKeyID: "asset-access", SecretAccessKey: "asset-secret",
	}))
	secondScope, err := CMCCAssetReuseScope(second.Id)
	require.NoError(t, err)
	assert.NotEqual(t, firstScope, secondScope)

	catalog, err := GetConfiguredSeedancePublicModels()
	require.NoError(t, err)
	var cmcc SeedancePublicModel
	for _, item := range catalog {
		if item.ModelName == "seedance-2.0-cmcc" {
			cmcc = item
			break
		}
	}
	assert.Equal(t, firstScope, cmcc.API.Assets.ReuseScope)
	assert.Equal(t, dto.PublicAssetGroupRequired, publicAssetGroupRequirement(cmcc.API.Assets.Media, AssetKindGeneral, "video"))
	assert.Equal(t, dto.PublicAssetGroupRequired, publicAssetGroupRequirement(cmcc.API.Assets.Media, AssetKindRealPerson, "image"))
	assert.Equal(t, dto.PublicAssetGroupRequired, publicAssetGroupRequirement(cmcc.API.Assets.Media, AssetKindRealPerson, "video"))
	assert.Equal(t, dto.PublicAssetGroupRequired, publicAssetGroupRequirement(cmcc.API.Assets.Media, AssetKindRealPerson, "audio"))

	require.NoError(t, channel.Delete())
	_, err = CMCCAssetReuseScope(channel.Id)
	require.Error(t, err)
}

func TestCMCCChannelRejectsUnverifiedMappingAndAcceptsHTTPGatewayPath(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := cmccSeedanceTestChannel("seedance-2.0-cmcc", common.ChannelStatusEnabled)
	channel.ModelMapping = common.GetPointer(`{"seedance-2.0-cmcc":"doubao-seedance-2-0-fast-260128"}`)
	require.ErrorContains(t, channel.ValidateSettings(), CMCCSeedance20ProviderModel)

	channel = cmccSeedanceTestChannel("seedance-2.0-cmcc", common.ChannelStatusEnabled)
	channel.BaseURL = common.GetPointer("http://zhenze-huhehaote.cmecloud.cn/path")
	require.NoError(t, channel.ValidateSettings())

	channel.BaseURL = common.GetPointer("ftp://zhenze-huhehaote.cmecloud.cn/path")
	require.ErrorContains(t, channel.ValidateSettings(), "http or https")
}
