package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type legacyChannelGroupStorage struct {
	Channel
	Group string `gorm:"type:varchar(64);default:'default'"`
}

func TestChannelGroupStoragePreservesMultipleGroups(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("TEST_" + strings.ToUpper(dialect) + "_DSN")
			if dialect == "sqlite" {
				dsn = "local"
				previousPath := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "groups.db")
				t.Cleanup(func() { common.SQLitePath = previousPath })
			}
			if dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			t.Setenv("CHANNEL_GROUP_TEST_DSN", dsn)
			db, dbType, err := chooseDB("CHANNEL_GROUP_TEST_DSN", false)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			previousType := common.MainDatabaseType()
			common.SetMainDatabaseType(dbType)
			initCol()
			t.Cleanup(func() { common.SetMainDatabaseType(previousType); initCol() })

			const table = "channel_group_storage_test"
			t.Cleanup(func() { _ = db.Migrator().DropTable(table) })
			require.NoError(t, db.Table(table).AutoMigrate(&legacyChannelGroupStorage{}))
			original := legacyChannelGroupStorage{Channel: Channel{Key: "fixture", Name: "short"}, Group: "default,existing"}
			require.NoError(t, db.Table(table).Create(&original).Error)
			require.NoError(t, db.Table(table).AutoMigrate(&Channel{}))

			groups := strings.Repeat("customer-group,", 7) + "last-group"
			channel := Channel{Key: "fixture", Name: "multiple", Group: groups}
			require.NoError(t, db.Table(table).Create(&channel).Error)
			// A second startup must retain the complete list, including its tail.
			require.NoError(t, db.Table(table).AutoMigrate(&Channel{}))
			var stored Channel
			require.NoError(t, db.Table(table).First(&stored, channel.Id).Error)
			assert.Equal(t, groups, stored.Group)
			var short Channel
			require.NoError(t, db.Table(table).First(&short, original.Id).Error)
			assert.Equal(t, original.Group, short.Group)
			defaultGroup := Channel{Key: "fixture", Name: "default-group"}
			require.NoError(t, db.Table(table).Create(&defaultGroup).Error)
			var savedDefault Channel
			require.NoError(t, db.Table(table).First(&savedDefault, defaultGroup.Id).Error)
			assert.Equal(t, "default", savedDefault.Group)
			var ids []int
			require.NoError(t, ApplyChannelGroupFilter(db.Table(table), "last-group").Pluck("id", &ids).Error)
			assert.Equal(t, []int{channel.Id}, ids)
		})
	}
}
