package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withSeedanceChannelDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
	return db
}

func seedanceTestChannel(modelName string, status int) *Channel {
	channel := &Channel{
		Type:   constant.ChannelTypeSeedanceLink,
		Status: status,
		Models: modelName,
		Group:  "default",
		Key:    "test-key",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
	})
	return channel
}

func TestSeedanceModelUniquenessIsEnforcedOnEnabledManagementWrite(t *testing.T) {
	db := withSeedanceChannelDB(t)
	existing := seedanceTestChannel("seedance-cn", common.ChannelStatusEnabled)
	existing.Name = "CN production"
	require.NoError(t, db.Create(existing).Error)

	conflict := seedanceTestChannel("seedance-cn", common.ChannelStatusEnabled)
	err := ValidateSeedanceChannelModelUniqueness(db, conflict)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CN production")
	assert.Contains(t, err.Error(), "Disable it there before enabling this channel")

	disabled := seedanceTestChannel("seedance-cn", common.ChannelStatusManuallyDisabled)
	require.NoError(t, ValidateSeedanceChannelModelUniqueness(db, disabled))
	different := seedanceTestChannel("seedance-global", common.ChannelStatusEnabled)
	require.NoError(t, ValidateSeedanceChannelModelUniqueness(db, different))
}

func TestFeicaiSeedanceChannelSettingsUseDedicatedProtocolWithoutAssets(t *testing.T) {
	withSeedanceChannelDB(t)
	channel := &Channel{
		Type:    constant.ChannelTypeSeedanceLink,
		Key:     "single-provider-key",
		BaseURL: common.GetPointer("https://feicai.example.com"),
	}
	settings := dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
	}

	require.NoError(t, validateSeedanceChannelSettings(channel, &settings))
	assert.Empty(t, settings.VideoUpstreamProfile)
	assert.Empty(t, settings.VideoUpstreamCreatePath)
	assert.Empty(t, settings.VideoUpstreamQueryPathTemplate)
}

func TestGetEnabledSeedanceChannelUsesDedicatedRoutingWithoutPriorityDistribution(t *testing.T) {
	db := withSeedanceChannelDB(t)
	priority := int64(-999)
	weight := uint(0)
	channel := seedanceTestChannel("seedance-direct", common.ChannelStatusEnabled)
	channel.Priority = &priority
	channel.Weight = &weight
	require.NoError(t, db.Create(channel).Error)

	selected, err := GetEnabledSeedanceChannel("default", "seedance-direct", 0)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)

	selected, err = GetEnabledSeedanceChannel("default", "seedance-direct", channel.Id+1)
	require.NoError(t, err)
	assert.Nil(t, selected)
}

func TestSeedanceChannelIsNotPublishedIntoNativeAbilities(t *testing.T) {
	db := withSeedanceChannelDB(t)
	channel := seedanceTestChannel("seedance-isolated", common.ChannelStatusEnabled)
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(db))

	var abilityCount int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilityCount).Error)
	assert.Zero(t, abilityCount)

	selected, err := GetChannel("default", "seedance-isolated", 0, "/v1/video/generations")
	require.NoError(t, err)
	assert.Nil(t, selected)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	channelSyncLock.Lock()
	previousGroupCache := group2model2channels
	previousChannelCache := channelsIDM
	previousAdvancedCache := channel2advancedCustomConfig
	channelSyncLock.Unlock()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		channelSyncLock.Lock()
		group2model2channels = previousGroupCache
		channelsIDM = previousChannelCache
		channel2advancedCustomConfig = previousAdvancedCache
		channelSyncLock.Unlock()
	})
	common.MemoryCacheEnabled = true
	InitChannelCache()
	selected, err = GetRandomSatisfiedChannel("default", "seedance-isolated", 0, "/v1/video/generations")
	require.NoError(t, err)
	assert.Nil(t, selected)

	assert.Contains(t, GetGroupEnabledModels("default"), "seedance-isolated")
	views, err := GetAllEnableAbilityWithChannels()
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, constant.ChannelTypeSeedanceLink, views[0].ChannelType)
}

func TestConfiguredSeedancePublicModelsIncludeDisabledAPIContracts(t *testing.T) {
	db := withSeedanceChannelDB(t)
	channels := []*Channel{
		seedanceTestChannel("seedance-official-disabled", common.ChannelStatusManuallyDisabled),
		seedanceTestChannel("seedance-funcloud-disabled", common.ChannelStatusManuallyDisabled),
		seedanceTestChannel("seedance-moxing-disabled", common.ChannelStatusManuallyDisabled),
		seedanceTestChannel("seedance-feicai-disabled", common.ChannelStatusManuallyDisabled),
		seedanceTestChannel("seedance-tokensave-disabled", common.ChannelStatusManuallyDisabled),
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction,
		AssetMinURLTTLSeconds: 3600,
	})
	channels[1].SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudSeedance,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudMaterial,
		AssetMinURLTTLSeconds: 3600,
	})
	channels[2].SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolMoxingMediaTaskV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingJoyCreatorV1,
		AssetMinURLTTLSeconds: 3600,
	})
	channels[3].SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone,
	})
	channels[4].SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolTokenSaveMediaTaskV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolTokenSaveAssetsV1,
		AssetMinURLTTLSeconds: 3600,
	})
	for _, channel := range channels {
		require.NoError(t, db.Create(channel).Error)
	}

	catalog, err := GetConfiguredSeedancePublicModels()
	require.NoError(t, err)
	require.Len(t, catalog, 5)

	byModel := make(map[string]SeedancePublicModel, len(catalog))
	for _, item := range catalog {
		byModel[item.ModelName] = item
	}
	official := byModel["seedance-official-disabled"]
	assert.False(t, official.Enabled)
	assert.Equal(t, "modelark_v3", official.API.Video.Protocol)
	assert.True(t, official.API.Assets.Supported)
	assert.Equal(t, "caller_managed_stateless", official.API.Assets.ManagementMode)
	assert.True(t, official.API.Assets.RequiresModel)
	assert.False(t, publicOperationSupported(official.API.Assets.Operations, "list_assets"))
	assert.True(t, publicOperationSupported(official.API.Assets.Operations, "get_asset_group_verification"))
	require.NotNil(t, official.API.Assets.Creation)
	assert.Equal(t, 3600, int(official.API.Assets.Creation.Source.ExpiresAtMinRemainingSeconds))
	assert.Equal(t, dto.PublicAssetGroupOptional, publicAssetGroupRequirement(official.API.Assets.Media, AssetKindGeneral, "image"))
	assert.Equal(t, dto.PublicAssetGroupRequired, publicAssetGroupRequirement(official.API.Assets.Media, AssetKindRealPerson, "image"))

	funCloud := byModel["seedance-funcloud-disabled"]
	assert.True(t, funCloud.API.Assets.Supported)
	assert.False(t, publicOperationSupported(funCloud.API.Assets.Operations, "update_asset"))
	assert.False(t, publicOperationSupported(funCloud.API.Assets.Operations, "delete_asset"))
	assert.True(t, publicOperationSupported(funCloud.API.Assets.Operations, "delete_asset_group"))
	require.NotNil(t, funCloud.API.Assets.Creation)
	assert.Contains(t, funCloud.API.Assets.Creation.RequiredFields, "asset_group_id")
	assert.Equal(t, int64(dto.PublicAssetFunCloudMaxBytes), funCloud.API.Assets.Creation.Source.MaxBytes)
	assert.True(t, funCloud.API.Assets.Creation.Source.ContentTypeMustMatchMedia)
	assert.Equal(t, dto.PublicAssetGroupRequired, publicAssetGroupRequirement(funCloud.API.Assets.Media, AssetKindGeneral, "image"))

	moxing := byModel["seedance-moxing-disabled"]
	assert.True(t, moxing.API.Assets.Supported)
	assert.True(t, publicOperationSupported(moxing.API.Assets.Operations, "update_asset"))
	assert.False(t, publicOperationSupported(moxing.API.Assets.Operations, "delete_asset_group"))

	feicai := byModel["seedance-feicai-disabled"]
	assert.False(t, feicai.API.Assets.Supported)
	assert.False(t, publicOperationSupported(feicai.API.Assets.Operations, "create_asset"))
	assert.Nil(t, feicai.API.Assets.Creation)

	tokenSave := byModel["seedance-tokensave-disabled"]
	assert.True(t, tokenSave.API.Assets.Supported)
	assert.False(t, publicOperationSupported(tokenSave.API.Assets.Operations, "create_asset_group"))
	require.NotNil(t, tokenSave.API.Assets.Creation)
	assert.NotContains(t, tokenSave.API.Assets.Creation.RequiredFields, "asset_group_id")
	assert.Equal(t, dto.PublicAssetGroupUnsupported, publicAssetGroupRequirement(tokenSave.API.Assets.Media, AssetKindGeneral, "image"))
}

func publicOperationSupported(operations []dto.PublicAPIOperation, name string) bool {
	for _, operation := range operations {
		if operation.Operation == name {
			return operation.Supported
		}
	}
	return false
}

func publicAssetGroupRequirement(media []dto.PublicAssetMedia, kind, mediaType string) string {
	for _, item := range media {
		if item.Kind == kind && item.MediaType == mediaType {
			return item.AssetGroupRequirement
		}
	}
	return ""
}

func TestSeedanceSettingsPersistOnlyCodeBackedProtocols(t *testing.T) {
	channel := seedanceTestChannel("seedance-cn", common.ChannelStatusEnabled)

	require.NoError(t, channel.ValidateSettings())
	normalized := channel.GetOtherSettings()
	assert.Equal(t, dto.VideoUpstreamProtocolModelArkV3Volcengine, normalized.VideoUpstreamProtocol)
	assert.Empty(t, normalized.VideoUpstreamProfile)
	assert.Empty(t, normalized.AssetUpstreamProfile)
}

func TestSeedanceSettingsRequireOneCredentialAndMatchingAssetProtocol(t *testing.T) {
	multiKey := seedanceTestChannel("seedance-cn", common.ChannelStatusEnabled)
	multiKey.Key = "first-key\nsecond-key"
	require.ErrorContains(t, multiKey.ValidateSettings(), "one channel credential")

	mismatched := seedanceTestChannel("seedance-global", common.ChannelStatusEnabled)
	mismatched.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3BytePlus,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolTokenSaveAssetsV1,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "project-a",
		AssetRegion:           "ap-southeast-1",
	})
	require.ErrorContains(t, mismatched.ValidateSettings(), "Media Task V1")

	matched := seedanceTestChannel("customer-standard-a", common.ChannelStatusEnabled)
	baseURL := "https://relay.example.com"
	matched.BaseURL = &baseURL
	matched.ModelMapping = common.GetPointer(`{"customer-standard-a":"doubao-seedance-2-0-260128"}`)
	matched.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolTokenSaveMediaTaskV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolTokenSaveAssetsV1,
		AssetMinURLTTLSeconds: 3600,
	})
	require.NoError(t, matched.ValidateSettings())
}

func TestSeedanceSettingsAcceptVolcengineOfficialAssetProtocol(t *testing.T) {
	channel := seedanceTestChannel("seedance-cn", common.ChannelStatusEnabled)
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "default",
		AssetRegion:           VolcengineAssetActionRegion,
	})
	require.NoError(t, channel.ValidateSettings())
	assert.Equal(t, VolcengineAssetActionBaseURL, AssetActionBaseURL(
		dto.AssetUpstreamProtocolVolcengineAction,
		VolcengineAssetActionRegion,
	))

	settings := channel.GetOtherSettings()
	settings.AssetRegion = "ap-southeast-1"
	channel.SetOtherSettings(settings)
	require.ErrorContains(t, channel.ValidateSettings(), VolcengineAssetActionRegion)
}

func TestMoxingTokenSaveSettingsRequireExactCustomerModelMapping(t *testing.T) {
	tests := []struct {
		name          string
		customerModel string
		providerModel string
		video         dto.VideoUpstreamProtocol
		asset         dto.AssetUpstreamProtocol
	}{
		{
			name: "standard line A", customerModel: "customer-standard-a", providerModel: "doubao-seedance-2-0-260128",
			video: dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, asset: dto.AssetUpstreamProtocolTokenSaveAssetsV1,
		},
		{
			name: "standard line B", customerModel: "customer-standard-b", providerModel: "doubao-seedance-2-0-260128",
			video: dto.VideoUpstreamProtocolMoxingMediaTaskV1, asset: dto.AssetUpstreamProtocolMoxingJoyCreatorV1,
		},
		{
			name: "fast line", customerModel: "customer-fast", providerModel: "doubao-seedance-2-0-fast-260128",
			video: dto.VideoUpstreamProtocolMoxingModelArkV1, asset: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		},
		{
			name: "mini line", customerModel: "customer-mini", providerModel: "doubao-seedance-2-0-mini-260615",
			video: dto.VideoUpstreamProtocolMoxingModelArkV1, asset: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		},
		{
			name: "next line", customerModel: "customer-next", providerModel: "doubao-seedance-2-5-260628",
			video: dto.VideoUpstreamProtocolMoxingModelArkV1, asset: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := seedanceTestChannel(test.customerModel, common.ChannelStatusEnabled)
			channel.BaseURL = common.GetPointer("https://provider.example.com")
			channel.ModelMapping = common.GetPointer(fmt.Sprintf(`{"%s":"%s"}`, test.customerModel, test.providerModel))
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				VideoUpstreamProtocol: test.video, AssetUpstreamProtocol: test.asset, AssetMinURLTTLSeconds: 3600,
			})
			require.NoError(t, channel.ValidateSettings())
			if test.asset == dto.AssetUpstreamProtocolMoxingVolcAssetsV1 {
				assert.Equal(t, "default", channel.GetOtherSettings().AssetProviderProject)
			}

			channel.ModelMapping = common.GetPointer(fmt.Sprintf(`{"%s":"wrong-model"}`, test.customerModel))
			require.ErrorContains(t, channel.ValidateSettings(), "model_mapping")
		})
	}
}

func TestFunCloudSettingsRequireExactModelAndMaterialSupport(t *testing.T) {
	tests := []struct {
		customerModel string
		providerModel string
		material      bool
	}{
		{customerModel: "public-standard", providerModel: "seedance-2", material: true},
		{customerModel: "public-fast", providerModel: "seedance-2-fast", material: true},
		{customerModel: "public-mini", providerModel: "seedance-2-mini", material: true},
		{customerModel: "public-next", providerModel: "seedance-2-5", material: false},
	}

	for _, test := range tests {
		t.Run(test.customerModel, func(t *testing.T) {
			channel := seedanceTestChannel(test.customerModel, common.ChannelStatusEnabled)
			channel.BaseURL = common.GetPointer("https://funcloud.example.com")
			channel.ModelMapping = common.GetPointer(fmt.Sprintf(`{"%s":"%s"}`, test.customerModel, test.providerModel))
			assetProtocol := dto.AssetUpstreamProtocolNone
			if test.material {
				assetProtocol = dto.AssetUpstreamProtocolFunCloudMaterial
			}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudSeedance,
				AssetUpstreamProtocol: assetProtocol,
				AssetMinURLTTLSeconds: 3600,
			})
			require.NoError(t, channel.ValidateSettings())

			channel.ModelMapping = common.GetPointer(fmt.Sprintf(`{"%s":"wrong"}`, test.customerModel))
			require.ErrorContains(t, channel.ValidateSettings(), "model_mapping")
		})
	}

	unsupportedMaterial := seedanceTestChannel("public-next", common.ChannelStatusEnabled)
	unsupportedMaterial.BaseURL = common.GetPointer("https://funcloud.example.com")
	unsupportedMaterial.ModelMapping = common.GetPointer(`{"public-next":"seedance-2-5"}`)
	unsupportedMaterial.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudSeedance,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudMaterial,
		AssetMinURLTTLSeconds: 3600,
	})
	require.ErrorContains(t, unsupportedMaterial.ValidateSettings(), "2.5 does not support")
}

func TestSeedancePublicAssetReuseScopeUsesConfiguredAssetDomain(t *testing.T) {
	first := seedanceTestChannel("public-fast-a", common.ChannelStatusEnabled)
	first.BaseURL = common.GetPointer("https://assets.example.com")
	first.SetOtherSettings(dto.ChannelOtherSettings{
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetProviderProject:  "default",
	})
	second := seedanceTestChannel("public-mini-a", common.ChannelStatusEnabled)
	second.BaseURL = common.GetPointer("https://assets.example.com/")
	second.SetOtherSettings(dto.ChannelOtherSettings{
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetProviderProject:  "default",
	})
	differentProtocol := seedanceTestChannel("public-standard-a", common.ChannelStatusEnabled)
	differentProtocol.BaseURL = common.GetPointer("https://assets.example.com")
	differentProtocol.SetOtherSettings(dto.ChannelOtherSettings{
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolMoxingJoyCreatorV1,
	})

	firstScope := seedancePublicAssetReuseScope(first, first.GetOtherSettings().AssetUpstreamProtocol)
	secondScope := seedancePublicAssetReuseScope(second, second.GetOtherSettings().AssetUpstreamProtocol)
	differentScope := seedancePublicAssetReuseScope(differentProtocol, differentProtocol.GetOtherSettings().AssetUpstreamProtocol)

	require.NotEmpty(t, firstScope)
	assert.Equal(t, firstScope, secondScope)
	assert.NotEqual(t, firstScope, differentScope)
	assert.Empty(t, seedancePublicAssetReuseScope(first, dto.AssetUpstreamProtocolNone))
}
