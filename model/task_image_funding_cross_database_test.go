package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestNativeImageFundingAcrossServerDatabases(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		kind      common.DatabaseType
		open      func(string) gorm.Dialector
	}{
		{"mysql", "TEST_IMAGE_MYSQL_DSN", common.DatabaseTypeMySQL, func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{"postgres", "TEST_IMAGE_POSTGRES_DSN", common.DatabaseTypePostgreSQL, func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skip("isolated test database is not configured")
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			tables := []any{&Channel{}, &User{}, &Token{}, &Task{}, &TaskCreateIdempotency{}, &ImageTaskSlot{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}}
			for _, table := range tables {
				if db.Migrator().HasTable(table) {
					t.Fatal("refusing to modify a non-empty image test database")
				}
			}
			oldDB, oldMain, oldLog := DB, common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(tc.kind, tc.kind)
			initCol()
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(tables...))
				DB = oldDB
				common.SetDatabaseTypes(oldMain, oldLog)
				initCol()
			})
			require.NoError(t, db.AutoMigrate(tables...))
			t.Run("preference_and_settlement", TestNativeImageFundingPreferenceAndAtomicSettlement)
			t.Run("acceptance_rollback", TestNativeImageAdmissionRollsBackSubscriptionWhenTaskWriteFails)
		})
	}
}
