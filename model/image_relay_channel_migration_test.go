package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withImageRelayMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}, &Channel{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	return db
}

func TestImageRelayMigrationWritesExplicitProtocolsAndRetiresType63(t *testing.T) {
	db := withImageRelayMigrationDB(t)
	require.NoError(t, db.Create(&Channel{
		Type:          constant.ChannelTypeAsyncImage,
		Name:          "FunCloud",
		Models:        "nano-banana-2",
		Key:           "key",
		OtherSettings: `{"allow_service_tier":true,"future_extension":{"nested":7}}`,
	}).Error)
	require.NoError(t, db.Create(&Channel{
		Type: legacyMoxingImageChannelType, Name: "Moxing", Models: "customer", Key: "key",
	}).Error)

	require.NoError(t, migrateImageRelayChannels())
	require.NoError(t, migrateImageRelayChannels())

	var channels []Channel
	require.NoError(t, db.Order("id").Find(&channels).Error)
	require.Len(t, channels, 2)
	assert.Equal(t, constant.ChannelTypeAsyncImage, channels[0].Type)
	assert.Equal(t, dto.ImageUpstreamProtocolFunCloudAIGCV2, channels[0].GetOtherSettings().ImageUpstreamProtocol)
	assert.True(t, channels[0].GetOtherSettings().AllowServiceTier)
	assert.Equal(t, funCloudImageDefaultBaseURL, channels[0].GetBaseURL())
	assert.Equal(t, constant.ChannelTypeAsyncImage, channels[1].Type)
	assert.Equal(t, dto.ImageUpstreamProtocolMoxingImagesV1, channels[1].GetOtherSettings().ImageUpstreamProtocol)
	assert.Equal(t, moxingImageDefaultBaseURL, channels[1].GetBaseURL())

	var legacyCount int64
	require.NoError(t, db.Model(&Channel{}).Where("type = ?", legacyMoxingImageChannelType).Count(&legacyCount).Error)
	assert.Zero(t, legacyCount)
	var marker Option
	require.NoError(t, db.Where(&Option{Key: imageRelayMigrationKey}).First(&marker).Error)
	assert.Equal(t, "done", marker.Value)

	var migratedSettings map[string]any
	require.NoError(t, common.UnmarshalJsonStr(channels[0].OtherSettings, &migratedSettings))
	assert.Equal(t, map[string]any{"nested": float64(7)}, migratedSettings["future_extension"])
	require.NoError(t, verifyImageRelayMigrationState())
}

func TestImageRelayMigrationRejectsConflictingLegacyProtocol(t *testing.T) {
	db := withImageRelayMigrationDB(t)
	settings, err := common.Marshal(dto.ChannelOtherSettings{
		ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Channel{
		Type: legacyMoxingImageChannelType, Name: "conflict", Key: "key", OtherSettings: string(settings),
	}).Error)

	err = migrateImageRelayChannels()
	require.ErrorContains(t, err, "conflicting image protocol")
	var channel Channel
	require.NoError(t, db.First(&channel).Error)
	assert.Equal(t, legacyMoxingImageChannelType, channel.Type)
}

func TestImageRelayMigrationRejectsExistingParameterOverride(t *testing.T) {
	db := withImageRelayMigrationDB(t)
	require.NoError(t, db.Create(&Channel{
		Type:          constant.ChannelTypeAsyncImage,
		Name:          "unsafe",
		Key:           "key",
		ParamOverride: common.GetPointer(`{"resolution":"4K"}`),
	}).Error)

	err := migrateImageRelayChannels()
	require.ErrorContains(t, err, "parameter overrides")
	var markerCount int64
	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: imageRelayMigrationKey}).Count(&markerCount).Error)
	assert.Zero(t, markerCount)
}

func TestImageRelayMigrationVerificationRejectsLegacyRowsWrittenAfterMarker(t *testing.T) {
	db := withImageRelayMigrationDB(t)
	require.NoError(t, migrateImageRelayChannels())
	require.NoError(t, db.Create(&Channel{
		Type: legacyMoxingImageChannelType,
		Name: "written-by-old-node",
		Key:  "key",
	}).Error)

	err := verifyImageRelayMigrationState()
	require.ErrorContains(t, err, "legacy channel")
}

func TestImageRelayMigrationVerificationRejectsParamOverrideWrittenAfterMarker(t *testing.T) {
	db := withImageRelayMigrationDB(t)
	require.NoError(t, migrateImageRelayChannels())
	settings, err := common.Marshal(dto.ChannelOtherSettings{
		ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Channel{
		Type:          constant.ChannelTypeAsyncImage,
		Name:          "unsafe-new-write",
		Key:           "key",
		BaseURL:       common.GetPointer("https://provider.example"),
		OtherSettings: string(settings),
		ParamOverride: common.GetPointer(`{"resolution":"4K"}`),
	}).Error)

	err = verifyImageRelayMigrationState()
	require.ErrorContains(t, err, "parameter overrides")
}

func TestImageRelayMigrationVerificationRequiresCompletedMarker(t *testing.T) {
	withImageRelayMigrationDB(t)

	err := verifyImageRelayMigrationState()
	require.ErrorContains(t, err, "migration is not complete")
}
