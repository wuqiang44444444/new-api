package model

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestFunCloudHostedAssetDatabaseLifecycle(t *testing.T) {
	for _, backend := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var dialector gorm.Dialector
			databaseType := common.DatabaseTypeSQLite
			switch backend {
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				dialector = mysql.Open(dsn)
				databaseType = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
				databaseType = common.DatabaseTypePostgreSQL
			default:
				dialector = sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_")))
			}
			prefix := "fhqa_" + strings.ReplaceAll(common.GetUUID(), "-", "")[:12] + "_"
			db, err := gorm.Open(dialector, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
			require.NoError(t, err)
			oldDB, oldType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(databaseType)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&FunCloudHostedAsset{}, &FunCloudHostedAssetGroup{}))
				sqlDB, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, sqlDB.Close())
				DB = oldDB
				common.SetMainDatabaseType(oldType)
			})
			require.NoError(t, db.AutoMigrate(&FunCloudHostedAsset{}, &FunCloudHostedAssetGroup{}))
			group := &FunCloudHostedAssetGroup{ID: "fhgrp_owner", UserID: 7, Model: "model", Name: "group"}
			require.NoError(t, CreateFunCloudHostedAssetGroup(group))
			location := FunCloudHostedStorageLocation{Backend: "s3", Endpoint: "https://store.example", Bucket: "images", Prefix: "prod"}
			asset := &FunCloudHostedAsset{ID: "fhas_owner", UserID: 7, GroupID: group.ID, Model: "model", Name: "image", ObjectKey: "assets/image.png", StorageLocation: location, MimeType: "image/png", SizeBytes: 24, Status: FunCloudHostedAssetStatusReady}
			require.NoError(t, CreateFunCloudHostedAsset(asset))
			require.NoError(t, db.AutoMigrate(&FunCloudHostedAsset{}, &FunCloudHostedAssetGroup{}))
			read, err := GetFunCloudHostedAsset(7, asset.ID)
			require.NoError(t, err)
			require.NotNil(t, read)
			assert.Equal(t, location, read.StorageLocation)
			other, err := GetFunCloudHostedAsset(8, asset.ID)
			require.NoError(t, err)
			assert.Nil(t, other)
			otherGroup, err := GetFunCloudHostedAssetGroup(8, group.ID)
			require.NoError(t, err)
			assert.Nil(t, otherGroup)
			deleted, err := MarkFunCloudHostedAssetDeleted(8, asset.ID)
			require.NoError(t, err)
			assert.False(t, deleted)
			deleted, err = MarkFunCloudHostedAssetDeleted(7, asset.ID)
			require.NoError(t, err)
			assert.True(t, deleted)
			deleted, err = MarkFunCloudHostedAssetDeleted(7, asset.ID)
			require.NoError(t, err)
			assert.True(t, deleted, "repeated deletion succeeds even when UPDATE changes no values")
			read, err = GetFunCloudHostedAsset(7, asset.ID)
			require.NoError(t, err)
			assert.Nil(t, read)
			var historical FunCloudHostedAsset
			require.NoError(t, db.First(&historical, "id = ?", asset.ID).Error)
			assert.Equal(t, FunCloudHostedAssetStatusDeleted, historical.Status)
			assert.Equal(t, location, historical.StorageLocation)
			assert.Equal(t, asset.ObjectKey, historical.ObjectKey)
			// Simulate the original hosted schema, which had no saved location.
			// Upgrade must preserve historical rows without guessing their bucket.
			for _, column := range []string{"storage_backend", "storage_endpoint", "storage_bucket", "storage_prefix"} {
				require.NoError(t, db.Migrator().DropColumn(&FunCloudHostedAsset{}, column))
			}
			require.NoError(t, db.AutoMigrate(&FunCloudHostedAsset{}))
			var upgraded FunCloudHostedAsset
			require.NoError(t, db.First(&upgraded, "id = ?", asset.ID).Error)
			assert.Equal(t, asset.ObjectKey, upgraded.ObjectKey)
			assert.Equal(t, FunCloudHostedAssetStatusDeleted, upgraded.Status)
			assert.Empty(t, upgraded.StorageLocation.Backend)

		})
	}
}
