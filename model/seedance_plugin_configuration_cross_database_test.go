package model

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Exercise publication against a real concurrent Channel write. The targets
// are disposable databases; existing tables are never altered by this fixture.
func TestSeedanceConfigurationPublicationAcrossServerDatabases(t *testing.T) {
	for _, target := range seedanceCrossDBTargets {
		t.Run(target.name, func(t *testing.T) {
			dsn := os.Getenv(target.env)
			if dsn == "" {
				t.Skipf("%s is not configured", target.env)
			}
			db, err := gorm.Open(target.dialector(dsn), &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			tables := []any{&Channel{}, &TaskPlugin{}}
			for _, table := range tables {
				require.False(t, db.Migrator().HasTable(table), "refusing a non-empty test database")
			}
			previousDB := DB
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(target.database, target.database)
			t.Cleanup(func() {
				DB = previousDB
				common.SetDatabaseTypes(previousMain, previousLog)
				require.NoError(t, db.Migrator().DropTable(tables...))
			})
			require.NoError(t, db.AutoMigrate(tables...))
			require.False(t, db.Migrator().HasColumn(&Channel{}, "seedance_plugin_version"))
			// With no existing version row PostgreSQL still has to serialize
			// first publication; an empty UPDATE alone cannot take that lock.
			start := make(chan struct{})
			saved := make(chan error, 2)
			for _, version := range []string{"1.8.0", "1.9.0"} {
				go func(version string) {
					<-start
					plugin := configurationTestPlugin(version, "provider-one")
					saved <- SaveTaskPlugin(&plugin)
				}(version)
			}
			close(start)
			successful := 0
			for range 2 {
				var saveErr error
				require.NoError(t, waitForChannel(t, saved, &saveErr))
				if saveErr == nil {
					successful++
				} else {
					require.True(t, target.database == common.DatabaseTypeMySQL && strings.Contains(saveErr.Error(), "1213"), "only an InnoDB gap-lock deadlock may reject a concurrent initial insert: %v", saveErr)
				}
			}
			require.Positive(t, successful)
			var activeCount int64
			require.NoError(t, db.Model(&TaskPlugin{}).Where("active = ?", true).Count(&activeCount).Error)
			assert.EqualValues(t, 1, activeCount)
			require.NoError(t, db.Where("1 = 1").Delete(&TaskPlugin{}).Error)
			old := configurationTestPlugin("2.0.0", "provider-one")
			candidate := configurationTestPlugin("2.1.0", "provider-two")
			require.NoError(t, SaveTaskPlugin(&old))
			require.NoError(t, SaveTaskPlugin(&candidate))

			channel := seedanceTestChannel("provider-one", common.ChannelStatusManuallyDisabled)
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			channel.SeedancePluginVersion = old.Version
			require.NoError(t, PinSeedanceChannelConfigurationInput(channel))
			writer := db.Begin()
			require.NoError(t, writer.Error)
			t.Cleanup(func() { _ = writer.Rollback().Error })
			require.NoError(t, validateSeedancePublishedChannelConfiguration(writer, channel))
			require.NoError(t, writer.Create(channel).Error)

			started := make(chan struct{})
			published := make(chan error, 1)
			go func() {
				close(started)
				published <- ActivateTaskPlugin("seedance-link", candidate.Version)
			}()
			<-started
			require.NoError(t, writer.Commit().Error)
			var publicationErr error
			require.NoError(t, waitForChannel(t, published, &publicationErr))
			require.ErrorContains(t, publicationErr, "incompatible with channels")
			active, err := GetTaskPluginVersion("seedance-link", "")
			require.NoError(t, err)
			assert.Equal(t, old.Version, active.Version)

			// Reverse order: promotion commits after the page was read. The
			// final write must reject the old form even if its values still fit.
			compatible := configurationTestPlugin("2.2.0", "provider-one")
			require.NoError(t, SaveTaskPlugin(&compatible))
			require.NoError(t, ActivateTaskPlugin("seedance-link", compatible.Version))
			require.ErrorContains(t, db.Transaction(func(tx *gorm.DB) error {
				return validateSeedancePublishedChannelConfiguration(tx, channel)
			}), "reload the channel form")
			persisted := Channel{}
			require.NoError(t, db.First(&persisted, channel.Id).Error)
			assert.Equal(t, "provider-one", persisted.Models)
		})
	}
}

// Credential deletion must read the published declaration under the same
// publication lock chain as Channel writes and activation, and its decision
// must follow the declaration active at commit time. The interleaving with a
// concurrent activation is serialized by that lock chain (channel row lock →
// plugin configuration lock), the same chain already raced against
// publication in the test above.
func TestSeedanceCredentialDeletionAcrossServerDatabases(t *testing.T) {
	for _, target := range seedanceCrossDBTargets {
		t.Run(target.name, func(t *testing.T) {
			dsn := os.Getenv(target.env)
			if dsn == "" {
				t.Skipf("%s is not configured", target.env)
			}
			db, err := gorm.Open(target.dialector(dsn), &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			tables := []any{&Channel{}, &TaskPlugin{}, &ChannelAssetCredential{}}
			for _, table := range tables {
				require.False(t, db.Migrator().HasTable(table), "refusing a non-empty test database")
			}
			previousDB := DB
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(target.database, target.database)
			t.Cleanup(func() {
				DB = previousDB
				common.SetDatabaseTypes(previousMain, previousLog)
				require.NoError(t, db.Migrator().DropTable(tables...))
			})
			require.NoError(t, db.AutoMigrate(tables...))

			published := func(version, credential string) *TaskPlugin {
				source := fmt.Sprintf(`export const meta = {apiVersion:2,key:"seedance-link",name:"Test",version:%q,author:{name:"test"},seedanceProtocols:["feicai_videos_v1"],channelConfiguration:{videos:[{protocol:"feicai_videos_v1",label:"Video",models:["provider-one"],modelMetadata:{"provider-one":{minDuration:4,maxDuration:15}},assetProtocols:["tokensave_assets_v1","none"],defaultAssetProtocol:"none"}],assets:[{protocol:"tokensave_assets_v1",label:"TokenSave",groupPolicy:"default_fallback",credential:%q,defaultURLTTLSeconds:3600},{protocol:"none",label:"None",groupPolicy:"none",credential:"none"}]}}; export const seedance={feicai_videos_v1:{buildCreate(){},parseCreateResponse(){},parseTaskObservation(){}}}; export const seedanceAssets={tokensave_assets_v1:{buildRequest(){},parseResponse(){}}};`, version, credential)
				return &TaskPlugin{Key: "seedance-link", Version: version, APIVersion: 2, Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Enabled: true}
			}
			require.NoError(t, SaveTaskPlugin(published("2.0.0", "asset_key_pair")))

			channel := seedanceTestChannel("customer", common.ChannelStatusEnabled)
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolTokenSaveAssetsV1, AssetMinURLTTLSeconds: 3600})
			channel.ModelMapping = common.GetPointer(`{"customer":"provider-one"}`)
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, db.Create(&ChannelAssetCredential{ChannelID: channel.Id, AccessKeyID: "access", SecretAccessKey: "secret"}).Error)

			// The active declaration binds this protocol to the key-pair slot.
			require.ErrorContains(t, DeleteChannelAssetCredential(channel.Id), "must be disabled")

			// Once publication moves the protocol to the channel credential slot,
			// the same delete decision follows the newly active declaration.
			require.NoError(t, SaveTaskPlugin(published("2.1.0", "channel")))
			require.NoError(t, ActivateTaskPlugin("seedance-link", "2.1.0"))
			require.NoError(t, DeleteChannelAssetCredential(channel.Id))
		})
	}
}
