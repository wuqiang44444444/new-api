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
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolRelayAssetsV1,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "project-a",
		AssetRegion:           "ap-southeast-1",
	})
	require.ErrorContains(t, mismatched.ValidateSettings(), "Media Task V1")

	matched := seedanceTestChannel("seedance-global", common.ChannelStatusEnabled)
	baseURL := "https://relay.example.com"
	matched.BaseURL = &baseURL
	matched.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolMediaTaskV1,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolRelayAssetsV1,
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
	assert.Equal(t, "https://ark.cn-beijing.volces.com", AssetActionBaseURL(
		dto.AssetUpstreamProtocolVolcengineAction,
		VolcengineAssetActionRegion,
	))

	settings := channel.GetOtherSettings()
	settings.AssetRegion = "ap-southeast-1"
	channel.SetOtherSettings(settings)
	require.ErrorContains(t, channel.ValidateSettings(), VolcengineAssetActionRegion)
}
