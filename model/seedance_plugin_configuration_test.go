package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func configurationTestPlugin(version, providerModel string) TaskPlugin {
	source := fmt.Sprintf(`
export const meta = {apiVersion: 2, key: "seedance-link", name: "Test", version: %q,
 author: {name: "test"}, seedanceProtocols: ["feicai_videos_v1"], channelConfiguration: {
 videos: [{protocol: "feicai_videos_v1", label: "Video", models: [%q], modelMetadata: {%q: {minDuration: 4, maxDuration: 15}}, assetProtocols: ["none"], defaultAssetProtocol: "none"}],
 assets: [{protocol: "none", label: "None", groupPolicy: "none", credential: "none"}]
 }};
export const seedance = {feicai_videos_v1: {buildCreate() {}, parseCreateResponse() {}, parseTaskObservation() {}}};`, version, providerModel, providerModel)
	return TaskPlugin{Key: "seedance-link", APIVersion: 2, Version: version, Source: source,
		SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true}
}

func TestSeedanceConfigurationPromotionAndChannelWritesUseSameDefinition(t *testing.T) {
	db := withSeedanceChannelDB(t)
	require.NoError(t, db.AutoMigrate(&TaskPlugin{}, &Task{}, &TaskCreateAttempt{}))
	old := configurationTestPlugin("2.0.0", "provider-one")
	require.NoError(t, SaveTaskPlugin(&old))
	channel := seedanceTestChannel("customer-name", common.ChannelStatusEnabled)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
	channel.ModelMapping = common.GetPointer(`{"customer-name":"provider-one"}`)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(channel).Error; err != nil {
			return err
		}
		return channel.AddAbilities(tx)
	}))
	newVersion := configurationTestPlugin("2.1.0", "provider-two")
	require.NoError(t, SaveTaskPlugin(&newVersion), "a non-active candidate can be uploaded for review")
	require.ErrorContains(t, ActivateTaskPlugin("seedance-link", "2.1.0"), fmt.Sprintf("channels [%d]", channel.Id))
	active, err := GetTaskPluginVersion("seedance-link", "")
	require.NoError(t, err)
	assert.Equal(t, old.Version, active.Version, "failed promotion must leave the old version active")
	assert.True(t, active.Enabled)

	// A write cannot commit a model accepted only by a non-active candidate.
	err = db.Transaction(func(tx *gorm.DB) error {
		candidate := *channel
		candidate.ModelMapping = common.GetPointer(`{"customer-name":"provider-two"}`)
		if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("model_mapping", candidate.ModelMapping).Error; err != nil {
			return err
		}
		return candidate.UpdateAbilities(tx)
	})
	require.ErrorContains(t, err, "mapped Provider model")
	var persisted Channel
	require.NoError(t, db.First(&persisted, channel.Id).Error)
	assert.Equal(t, channel.GetModelMapping(), persisted.GetModelMapping())

	compatible := configurationTestPlugin("2.2.0", "provider-one")
	require.NoError(t, SaveTaskPlugin(&compatible))
	channel.SeedancePluginVersion = old.Version
	require.NoError(t, PinSeedanceChannelConfigurationInput(channel))
	require.NoError(t, ActivateTaskPlugin("seedance-link", compatible.Version))
	require.ErrorContains(t, PinSeedanceChannelConfigurationInput(channel), "reload the channel form")
	require.ErrorContains(t, db.Transaction(func(tx *gorm.DB) error {
		return channel.UpdateAbilities(tx)
	}), "reload the channel form", "promotion between form validation and transaction must reject the stale write")
	channel.SeedancePluginVersion = compatible.Version
	require.NoError(t, PinSeedanceChannelConfigurationInput(channel))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return channel.UpdateAbilities(tx) }))
	active, err = GetTaskPluginVersion("seedance-link", "")
	require.NoError(t, err)
	assert.Equal(t, compatible.Version, active.Version)
	var abilities int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilities).Error)
	assert.Zero(t, abilities, "configuration publication cannot grant native routing eligibility")
}

func TestSeedanceConfigurationPublishRequiresCompleteMetadata(t *testing.T) {
	db := withSeedanceChannelDB(t)
	require.NoError(t, db.AutoMigrate(&TaskPlugin{}, &Task{}, &TaskCreateAttempt{}))
	active := configurationTestPlugin("2.0.0", "provider-one")
	require.NoError(t, SaveTaskPlugin(&active))

	incompleteSource := fmt.Sprintf(`
export const meta = {apiVersion: 2, key: "seedance-link", name: "Test", version: %q,
 author: {name: "test"}, seedanceProtocols: ["feicai_videos_v1"], channelConfiguration: {
 videos: [{protocol: "feicai_videos_v1", label: "Video", models: ["provider-two"], assetProtocols: ["none"], defaultAssetProtocol: "none"}],
 assets: [{protocol: "none", label: "None", groupPolicy: "none", credential: "none"}]
 }};
export const seedance = {feicai_videos_v1: {buildCreate() {}, parseCreateResponse() {}, parseTaskObservation() {}}};`, "2.1.0")
	incomplete := TaskPlugin{Key: "seedance-link", APIVersion: 2, Version: "2.1.0", Source: incompleteSource,
		SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(incompleteSource))), Enabled: true}
	require.NoError(t, SaveTaskPlugin(&incomplete), "an incomplete candidate can be stored for review")
	require.ErrorContains(t, ActivateTaskPlugin("seedance-link", incomplete.Version), "listed Provider model requires declared model metadata",
		"an incomplete declaration must fail activation, not be hidden at projection time")

	persisted, err := GetTaskPluginVersion("seedance-link", "")
	require.NoError(t, err)
	assert.Equal(t, active.Version, persisted.Version, "failed publication must leave the previous version active")

	channel := seedanceTestChannel("customer-name", common.ChannelStatusEnabled)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
	channel.ModelMapping = common.GetPointer(`{"customer-name":"provider-one"}`)
	channel.SeedancePluginVersion = active.Version
	require.NoError(t, PinSeedanceChannelConfigurationInput(channel),
		"channels keep validating against the unchanged active declaration")

	models, err := GetConfiguredSeedancePublicModels()
	require.NoError(t, err)
	for _, published := range models {
		assert.NotEqual(t, "customer-two", published.ModelName,
			"a rejected candidate must not leak models into the public list")
	}
}

func TestSeedanceConfigurationDeletionCannotPromoteIncompatibleVersion(t *testing.T) {
	db := withSeedanceChannelDB(t)
	require.NoError(t, db.AutoMigrate(&TaskPlugin{}, &Task{}, &TaskCreateAttempt{}))
	active := configurationTestPlugin("2.0.0", "provider-one")
	require.NoError(t, SaveTaskPlugin(&active))
	incompatible := configurationTestPlugin("2.1.0", "provider-two")
	require.NoError(t, SaveTaskPlugin(&incompatible))
	channel := seedanceTestChannel("provider-one", common.ChannelStatusManuallyDisabled)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
	require.NoError(t, db.Create(channel).Error)
	_, err := DeleteTaskPluginVersion("seedance-link", active.Version)
	require.ErrorContains(t, err, "incompatible with channels")
	kept, err := GetTaskPluginVersion("seedance-link", active.Version)
	require.NoError(t, err)
	assert.True(t, kept.Active)
	assert.True(t, kept.Enabled)
}
