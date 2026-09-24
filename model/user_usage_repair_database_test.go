package model

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Opt-in only: DSNs must point to disposable databases created for this test.
// The runner must also explicitly permit resetting their tables.
func TestUsageRepairExternalDatabase(t *testing.T) {
	for _, engine := range []string{"MYSQL", "POSTGRES"} {
		t.Run(engine, func(t *testing.T) {
			dsn := os.Getenv("USAGE_REPAIR_TEST_" + engine + "_DSN")
			if dsn == "" {
				t.Skip("disposable database DSN not configured")
			}
			require.Equal(t, "1", os.Getenv("USAGE_REPAIR_TEST_ALLOW_RESET"), "external tests reset only explicitly authorized disposable databases")
			require.Contains(t, dsn, "usage_repair_tests")
			var driver gorm.Dialector
			kind := common.DatabaseTypeMySQL
			if engine == "MYSQL" {
				driver = mysql.Open(dsn)
			} else {
				driver = postgres.Open(dsn)
				kind = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			conn, err := db.DB()
			require.NoError(t, err)
			defer conn.Close()
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			common.SetDatabaseTypes(kind, kind)
			defer common.SetDatabaseTypes(previousMain, previousLog)
			models := []any{&User{}, &Log{}, &Task{}, &TaskCreateAttempt{}, &TaskBillingDelivery{}, &AuditLog{}, &QuotaData{}, &Channel{}}
			require.NoError(t, db.Migrator().DropTable(models...))
			testUsageRepairSchemaMigration(t, db, kind)
			require.NoError(t, db.AutoMigrate(models...))
			seedUsageRepairFixture(t, db)
			seedUsageRepairRefunds(t, db)
			manifest, evidence := buildUsageRepairManifestForTest(t, db, "external-repair", true)
			assert.Equal(t, fixtureUsedQuota, evidence.UsedQuota)
			columns, err := db.Migrator().ColumnTypes(&User{})
			require.NoError(t, err)
			for _, column := range columns {
				if column.Name() == "used_quota" {
					expectedType := "int8"
					if engine == "MYSQL" {
						expectedType = "bigint"
					}
					assert.Equal(t, expectedType, strings.ToLower(column.DatabaseTypeName()))
					break
				}
			}
			// A conflicting writer must fail with the engine's lock timeout, proving
			// lockForUpdate actually acquired a row lock on this connection.
			tx := db.Begin()
			require.NoError(t, tx.Error)
			defer tx.Rollback()
			var locked User
			require.NoError(t, lockForUpdate(tx).Select("id").Take(&locked, fixtureTargetUserID).Error)
			other, err := conn.Conn(context.Background())
			require.NoError(t, err)
			defer other.Close()
			timeoutSQL := "SET lock_timeout = '100ms'"
			if engine == "MYSQL" {
				timeoutSQL = "SET innodb_lock_wait_timeout = 1"
			}
			_, err = other.ExecContext(context.Background(), timeoutSQL)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err = other.ExecContext(ctx, "UPDATE users SET used_quota = used_quota WHERE id = 91")
			cancel()
			require.Error(t, err)
			if engine == "MYSQL" {
				assert.Contains(t, err.Error(), "1205")
			} else {
				assert.Contains(t, err.Error(), "55P03")
			}
			require.NoError(t, other.Close())
			require.NoError(t, tx.Rollback().Error)
			applied, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
			require.NoError(t, err)
			assert.True(t, applied)
			applied, err = ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
			require.NoError(t, err)
			assert.False(t, applied)
			var user User
			require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
			assert.Equal(t, fixtureExpectedAfter, int64(user.UsedQuota))
			assert.Equal(t, fixtureWalletQuota, int64(user.Quota))
			reverse, already, err := PrepareUsageRepairRollback(db, manifest)
			require.NoError(t, err)
			assert.False(t, already)
			applied, err = ApplyUserUsageRepair(db, reverse, fixtureOperatorID)
			require.NoError(t, err)
			assert.True(t, applied)
			_, already, err = PrepareUsageRepairRollback(db, manifest)
			require.NoError(t, err)
			assert.True(t, already)
			require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
			assert.Equal(t, fixtureUsedQuota, int64(user.UsedQuota))
			require.NoError(t, db.Migrator().DropTable(models...))
			require.NoError(t, db.AutoMigrate(models...))
			seedUsageRepairFixture(t, db)
			seedUsageRepairRefunds(t, db)
			testUsageRepairStartup(t, db)
		})
	}
}
