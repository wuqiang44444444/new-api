package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type usageRepairLegacyUser struct {
	ID           int   `gorm:"primaryKey"`
	Quota        int64 `gorm:"type:bigint"`
	UsedQuota    int64 `gorm:"type:int;default:0"`
	AffQuota     int64 `gorm:"type:bigint"`
	AffHistory   int64 `gorm:"type:bigint"`
	RequestCount int
}

func TestUsageRepairSchemaSQLitePreservesCumulativeAmount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	testUsageRepairSchemaMigration(t, db, common.DatabaseTypeSQLite)
}

// Called against SQLite and both opt-in disposable engines. It proves startup
// upgrades an actual legacy column, preserves funds, and can be run repeatedly.
func testUsageRepairSchemaMigration(t *testing.T, db *gorm.DB, kind common.DatabaseType) {
	t.Helper()
	t.Setenv("SKIP_64BIT_QUOTA_SCHEMA_CHECK", "false")
	require.NoError(t, db.Table("users").AutoMigrate(&usageRepairLegacyUser{}))
	before := usageRepairLegacyUser{ID: 1, Quota: 321, UsedQuota: 123, AffQuota: 456, AffHistory: 789, RequestCount: 12}
	require.NoError(t, db.Table("users").Create(&before).Error)
	if kind != common.DatabaseTypeSQLite {
		// GORM may map Go int/int64 to bigint despite type:int. Install the
		// actual old SQL types explicitly, otherwise this never tests widening.
		usageSQL := "ALTER TABLE users ALTER COLUMN used_quota TYPE INTEGER"
		walletSQL := "ALTER TABLE users ALTER COLUMN aff_quota TYPE INTEGER"
		if kind == common.DatabaseTypeMySQL {
			usageSQL = "ALTER TABLE users MODIFY COLUMN used_quota INT DEFAULT 0"
			walletSQL = "ALTER TABLE users MODIFY COLUMN aff_quota INT"
		}
		require.NoError(t, db.Exec(usageSQL).Error)
		// Reject a legacy wallet before performing any projection migration.
		require.NoError(t, db.Exec(walletSQL).Error)
		err := ensureUserQuotaColumns(db, kind)
		require.ErrorContains(t, err, "users.aff_quota")
		columns, err := db.Migrator().ColumnTypes(&User{})
		require.NoError(t, err)
		for _, column := range columns {
			if column.Name() == "used_quota" {
				assert.False(t, is64BitIntegerType(kind, column.DatabaseTypeName()), "rejected startup must not partially migrate usage")
			}
		}
		require.NoError(t, db.Table("users").Migrator().AlterColumn(&usageRepairLegacyUser{}, "AffQuota"))
	}
	require.NoError(t, ensureUserQuotaColumns(db, kind))
	var actual usageRepairLegacyUser
	require.NoError(t, db.Table("users").Take(&actual, before.ID).Error)
	assert.Equal(t, before, actual)
	require.NoError(t, db.Table("users").Where("id = ?", before.ID).UpdateColumn("used_quota", fixtureExpectedAfter).Error)
	require.NoError(t, ensureUserQuotaColumns(db, kind))
	require.NoError(t, db.Table("users").Take(&actual, before.ID).Error)
	before.UsedQuota = fixtureExpectedAfter
	assert.Equal(t, before, actual)
	require.NoError(t, db.Migrator().DropTable("users"))
}
