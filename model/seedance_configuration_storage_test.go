package model

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This storage contract deliberately depends only on native Channel/TaskPlugin
// storage and the extra credential table. The same test is replayed on upstream.
func TestSeedanceConfigurationNativeStorageContract(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "configuration.db")), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousType := DB, common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { DB = previousDB; common.SetMainDatabaseType(previousType) })
	require.NoError(t, db.AutoMigrate(&Channel{}, &TaskPlugin{}, &ChannelAssetCredential{}))
	assert.False(t, db.Migrator().HasColumn(&Channel{}, "seedance_plugin_version"))
	candidate := func(version, providerModel string) TaskPlugin {
		source := fmt.Sprintf(`export const meta = {apiVersion:2,key:"seedance-link",name:"Test",version:%q,author:{name:"test"},seedanceProtocols:["feicai_videos_v1"],channelConfiguration:{videos:[{protocol:"feicai_videos_v1",label:"Video",models:[%q],modelMetadata:{%q:{minDuration:4,maxDuration:15}},assetProtocols:["none"],defaultAssetProtocol:"none"}],assets:[{protocol:"none",label:"None",groupPolicy:"none",credential:"none"}]}}; export const seedance={feicai_videos_v1:{buildCreate(){},parseCreateResponse(){},parseTaskObservation(){}}};`, version, providerModel, providerModel)
		return TaskPlugin{Key: "seedance-link", Version: version, APIVersion: 2, Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true}
	}
	first := candidate("2.0.0", "provider-one")
	require.NoError(t, SaveTaskPlugin(&first))
	channel := Channel{Type: constant.ChannelTypeSeedanceLink, Models: "customer", Key: "fixture-only", Status: common.ChannelStatusEnabled, OtherSettings: `{"video_upstream_protocol":"feicai_videos_v1","asset_upstream_protocol":"none"}`, ModelMapping: common.GetPointer(`{"customer":"provider-one"}`), SeedancePluginVersion: first.Version}
	require.NoError(t, PinSeedanceChannelConfigurationInput(&channel))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&channel).Error; err != nil {
			return err
		}
		return validateSeedancePublishedChannelConfiguration(tx, &channel)
	}))
	incompatible := candidate("2.1.0", "provider-two")
	require.NoError(t, SaveTaskPlugin(&incompatible))
	require.ErrorContains(t, ActivateTaskPlugin(first.Key, incompatible.Version), "incompatible with channels")
	active, err := GetTaskPluginVersion(first.Key, "")
	require.NoError(t, err)
	assert.Equal(t, first.Version, active.Version)
	compatible := candidate("2.2.0", "provider-one")
	require.NoError(t, SaveTaskPlugin(&compatible))
	require.NoError(t, ActivateTaskPlugin(first.Key, compatible.Version))
	require.ErrorContains(t, db.Transaction(func(tx *gorm.DB) error { return validateSeedancePublishedChannelConfiguration(tx, &channel) }), "reload the channel form")
	var saved Channel
	require.NoError(t, db.First(&saved, channel.Id).Error)
	assert.Equal(t, channel.OtherSettings, saved.OtherSettings)
	assert.Equal(t, channel.GetModelMapping(), saved.GetModelMapping())
	require.NoError(t, SetTaskPluginEnabled(first.Key, false))
	require.NoError(t, SetTaskPluginEnabled(first.Key, true))
	// The native plugin namespace retains ordinary storage and enable behavior.
	native := TaskPlugin{Key: "native-fixture", Version: "1.0.0", APIVersion: 1, Source: "native-source", SourceHash: "native-hash", Enabled: true}
	require.NoError(t, SaveTaskPlugin(&native))
	require.NoError(t, SetTaskPluginEnabled(native.Key, false))
	got, err := GetTaskPluginVersion(native.Key, "")
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, native.Source, got.Source)
}
