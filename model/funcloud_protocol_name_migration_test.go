package model

import (
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
)

func setupFunCloudMigrationTestDB(t *testing.T) (*gorm.DB, map[string][]string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousType := common.MainDatabaseType()
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	require.NoError(t, db.AutoMigrate(
		&Option{}, &Channel{}, &Task{}, &TaskCreateAttempt{}, &TaskCreateIdempotency{},
		&ProviderCostExposure{},
	))
	originalDB := DB
	DB = db
	t.Cleanup(func() {
		DB = originalDB
		common.SetMainDatabaseType(previousType)
		initCol()
	})

	columnsByTable := map[string][]string{}
	for _, target := range funCloudProtocolMigrationTargets {
		require.True(t, db.Migrator().HasTable(target.table), target.table)
		require.True(t, db.Migrator().HasColumn(target.table, target.column), target.table+"."+target.column)
		columnsByTable[target.table] = append(columnsByTable[target.table], target.column)
	}
	return db, columnsByTable
}

func TestMigrateFunCloudProtocolNamesRewritesDurableFactsWithoutAliases(t *testing.T) {
	db, columnsByTable := setupFunCloudMigrationTestDB(t)
	for table, columns := range columnsByTable {
		values := map[string]any{"id": 1}
		for _, column := range columns {
			values[column] = legacyFunCloudVideoProtocol + "|" + legacyFunCloudAssetProtocol + "|" + legacyFunCloudVideoProfile + ":v2"
		}
		switch table {
		case "channels":
			values["key"] = "test-key"
		case "tasks":
			values["status"] = TaskStatusSuccess
		case "task_create_attempts":
			values["status"] = TaskCreateAttemptComplete
		case "task_create_idempotencies":
			values["status"] = TaskCreateIdempotencyComplete
		}
		require.NoError(t, db.Table(table).Create(values).Error)
	}

	require.NoError(t, migrateFunCloudProtocolNames())
	require.NoError(t, migrateFunCloudProtocolNames())
	remaining, err := countFunCloudLegacyProtocolFacts(db, []string{
		legacyFunCloudVideoProtocol, legacyFunCloudAssetProtocol, legacyFunCloudVideoProfile,
	})
	require.NoError(t, err)
	assert.Zero(t, remaining)

	var marker Option
	require.NoError(t, db.Where(&Option{Key: funCloudProtocolMigrationKey}).First(&marker).Error)
	assert.Equal(t, "done", marker.Value)

	for _, target := range funCloudProtocolMigrationTargets {
		var value string
		require.NoError(t, db.Table(target.table).Select(target.column).Where("id = ?", 1).Scan(&value).Error)
		assert.NotContains(t, value, legacyFunCloudVideoProtocol)
		assert.NotContains(t, value, legacyFunCloudAssetProtocol)
		assert.NotContains(t, value, legacyFunCloudVideoProfile)
		assert.Contains(t, value, "funcloud_seedance")
		assert.Contains(t, value, "funcloud_material")
		assert.Contains(t, value, "third_party_funcloud_seedance:v2")
	}
}

func TestFunCloudProtocolRenameBlocksActiveDurableFacts(t *testing.T) {
	db, _ := setupFunCloudMigrationTestDB(t)
	require.NoError(t, db.Table("tasks").Create(map[string]any{
		"id": 1, "private_data": legacyFunCloudVideoProfile + ":v2", "status": TaskStatusInProgress,
	}).Error)
	require.NoError(t, db.Table("task_create_attempts").Create(map[string]any{
		"id": 1, "upstream_protocol": legacyFunCloudVideoProtocol, "status": TaskCreateAttemptSending,
	}).Error)
	require.NoError(t, db.Table("task_create_idempotencies").Create(map[string]any{
		"id": 1, "recovery_snapshot": legacyFunCloudVideoProfile + ":v2", "status": TaskCreateIdempotencyCreating,
	}).Error)

	err := migrateFunCloudProtocolNames()
	require.ErrorContains(t, err, "tasks=1")
	require.ErrorContains(t, err, "attempts=1")
	require.ErrorContains(t, err, "idempotencies=1")
	require.ErrorContains(t, err, "drain these facts")
	var markerCount int64
	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: funCloudProtocolMigrationKey}).Count(&markerCount).Error)
	assert.Zero(t, markerCount)
	var privateData string
	require.NoError(t, db.Table("tasks").Select("private_data").Where("id = ?", 1).Scan(&privateData).Error)
	assert.Equal(t, legacyFunCloudVideoProfile+":v2", privateData)

	require.NoError(t, db.Table("tasks").Where("id = ?", 1).Update("status", TaskStatusSuccess).Error)
	require.NoError(t, db.Table("task_create_attempts").Where("id = ?", 1).Update("status", TaskCreateAttemptComplete).Error)
	require.NoError(t, db.Table("task_create_idempotencies").Where("id = ?", 1).Update("status", TaskCreateIdempotencyComplete).Error)
	require.NoError(t, migrateFunCloudProtocolNames())
}

func TestFunCloudProtocolRenameRollsBackAllWritesOnUpdateFailure(t *testing.T) {
	db, columnsByTable := setupFunCloudMigrationTestDB(t)
	for table, columns := range columnsByTable {
		values := map[string]any{"id": 1}
		for _, column := range columns {
			values[column] = legacyFunCloudVideoProtocol
		}
		switch table {
		case "channels":
			values["key"] = "test-key"
		case "tasks":
			values["status"] = TaskStatusSuccess
		case "task_create_attempts":
			values["status"] = TaskCreateAttemptComplete
		case "task_create_idempotencies":
			values["status"] = TaskCreateIdempotencyComplete
		}
		require.NoError(t, db.Table(table).Create(values).Error)
	}
	require.NoError(t, db.Exec(`
		CREATE TRIGGER fail_funcloud_exposure_update
		BEFORE UPDATE OF upstream_profile ON provider_cost_exposures
		BEGIN
			SELECT RAISE(FAIL, 'forced migration failure');
		END
	`).Error)

	err := migrateFunCloudProtocolNames()
	require.ErrorContains(t, err, "forced migration failure")

	var settings string
	require.NoError(t, db.Table("channels").Select("settings").Where("id = ?", 1).Scan(&settings).Error)
	assert.Equal(t, legacyFunCloudVideoProtocol, settings)
	var markerCount int64
	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: funCloudProtocolMigrationKey}).Count(&markerCount).Error)
	assert.Zero(t, markerCount)
}

func TestFunCloudProtocolMigrationRequiresRealSchema(t *testing.T) {
	db, _ := setupFunCloudMigrationTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&ProviderCostExposure{}))

	err := migrateFunCloudProtocolNames()
	require.ErrorContains(t, err, "requires table provider_cost_exposures")
}

func TestFunCloudLegacySearchConditionUsesSupportedDialectCasts(t *testing.T) {
	for _, test := range []struct {
		name     string
		dialect  string
		contains string
	}{
		{name: "SQLite", dialect: "sqlite", contains: "private_data LIKE ?"},
		{name: "MySQL", dialect: "mysql", contains: "CAST(private_data AS CHAR) LIKE ?"},
		{name: "PostgreSQL", dialect: "postgres", contains: "CAST(private_data AS TEXT) LIKE ?"},
	} {
		t.Run(test.name, func(t *testing.T) {
			condition, args := funCloudLegacySearchCondition(test.dialect, []string{"private_data"}, []string{"legacy_%!"})
			assert.Contains(t, condition, test.contains)
			assert.Contains(t, condition, "ESCAPE '!'")
			assert.Equal(t, []any{"%legacy!_!%!!%"}, args)
		})
	}
}

func testFunCloudLegacySearchConditionOnDatabase(t *testing.T, db *gorm.DB, dialect string) {
	t.Helper()
	condition, args := funCloudLegacySearchCondition(dialect, []string{"probe_value"}, []string{"legacy_%!"})
	queryArgs := append([]any{"prefixlegacy_%!suffix"}, args...)
	var matched int
	require.NoError(t, db.Raw(
		"SELECT 1 FROM (SELECT ? AS probe_value) AS migration_probe WHERE "+condition,
		queryArgs...,
	).Scan(&matched).Error)
	assert.Equal(t, 1, matched)
}

func TestFunCloudLegacySearchConditionExecutesOnSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	testFunCloudLegacySearchConditionOnDatabase(t, db, "sqlite")
}

func TestFunCloudLegacySearchConditionExecutesOnConfiguredDatabases(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		dialector func(string) gorm.Dialector
	}{
		{name: "mysql", env: "TEST_MYSQL_DSN", dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{name: "postgres", env: "TEST_POSTGRES_DSN", dialector: func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.env))
			if dsn == "" {
				t.Skip(test.env + " is not configured")
			}
			db, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			testFunCloudLegacySearchConditionOnDatabase(t, db, test.name)
		})
	}
}
