package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This acceptance test runs only on an explicitly named, isolated copy of the
// imported database. Migration remains in the standalone Python script.
func TestImportedPostgresApplicationStartup(t *testing.T) {
	dsn := os.Getenv("TEST_IMPORTED_POSTGRES_DSN")
	expected := os.Getenv("TEST_IMPORTED_POSTGRES_DATABASE")
	if dsn == "" || expected == "" {
		t.Skip("requires an isolated imported PostgreSQL database")
	}
	require.True(t, strings.HasPrefix(expected, "oneapi_acceptance_"), "use an isolated acceptance database")
	probe, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	sqlDB, err := probe.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var database string
	require.NoError(t, probe.Raw("SELECT current_database()").Scan(&database).Error)
	require.Equal(t, expected, database)

	// Include every original column of the durable financial/execution tables.
	// users.used_quota is a statistic with an existing audited startup repair,
	// so wallet and account ownership fields are asserted separately.
	invariants := map[string][]string{
		"users":  {"id", "quota", "aff_quota", "aff_history", "request_count", "password", "role", "status", "group", "deleted_at"},
		"tokens": nil, "channels": nil, "tasks": nil,
		"task_create_attempts": nil, "task_create_idempotencies": nil,
		"task_billing_deliveries": nil, "provider_cost_exposures": nil,
		"fun_cloud_hosted_assets": nil, "fun_cloud_hosted_asset_groups": nil,
		"customer_contracts": nil, "customer_contract_entity_rules": nil,
		"logs": nil,
	}
	before := make(map[string]string)
	for table, columns := range invariants {
		if columns == nil {
			require.NoError(t, probe.Raw("SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=? ORDER BY ordinal_position", table).Scan(&columns).Error)
			invariants[table] = columns
		}
		require.NotEmpty(t, columns)
		before[table] = importedInvariantDigest(t, probe, table, columns)
	}
	var originalGroups []struct {
		ID    int
		Group string
	}
	require.NoError(t, probe.Model(&model.Channel{}).Select([]string{"id", "group"}).Order("id").Scan(&originalGroups).Error)
	require.NotEmpty(t, originalGroups)

	// InitResources reads .env; a clean directory ensures it cannot load any
	// developer/production connections. No Provider workers are started here.
	t.Chdir(t.TempDir())
	t.Setenv("SQL_DSN", dsn)
	t.Setenv("LOG_SQL_DSN", "")
	t.Setenv("REDIS_CONN_STRING", "")
	t.Setenv("NODE_TYPE", "master")
	t.Setenv("DEBUG", "false")
	t.Setenv("SESSION_SECRET", "isolated-migration-acceptance-session")
	t.Setenv("CRYPTO_SECRET", "isolated-migration-acceptance-crypto")
	t.Setenv("TASK_REQUEST_EVIDENCE_ENABLED", "false")
	t.Setenv("SQL_MAX_OPEN_CONNS", "10")
	t.Setenv("SQL_MAX_IDLE_CONNS", "2")
	previousLogDir := *common.LogDir
	*common.LogDir = ""
	t.Cleanup(func() { *common.LogDir = previousLogDir })

	for startup := 1; startup <= 2; startup++ {
		require.NoError(t, InitResources())
		for table, columns := range invariants {
			assert.Equal(t, before[table], importedInvariantDigest(t, model.DB, table, columns), "startup=%d table=%s invariant changed", startup, table)
		}
		var groupColumn struct {
			DataType               string
			CharacterMaximumLength int64
		}
		require.NoError(t, model.DB.Raw("SELECT data_type,character_maximum_length FROM information_schema.columns WHERE table_schema='public' AND table_name='channels' AND column_name='group'").Scan(&groupColumn).Error)
		assert.Equal(t, "character varying", groupColumn.DataType)
		assert.EqualValues(t, 128, groupColumn.CharacterMaximumLength)
		for _, original := range originalGroups {
			if len(original.Group) <= 64 {
				continue
			}
			groups := strings.Split(original.Group, ",")
			var count int64
			require.NoError(t, model.ApplyChannelGroupFilter(model.DB.Model(&model.Channel{}), groups[len(groups)-1]).Where("id = ?", original.ID).Count(&count).Error)
			assert.EqualValues(t, 1, count, "a group at the tail of a long list must remain selectable")
		}
		require.NoError(t, model.CloseDB())
	}

	// Reopen for ORM reads and real route registration on the migrated schema.
	require.NoError(t, model.InitDB())
	require.NoError(t, model.InitLogDB())
	t.Cleanup(func() { _ = model.CloseDB() })
	channels, err := model.GetAllChannels(0, len(originalGroups)+1, true, true)
	require.NoError(t, err)
	assert.Len(t, channels, len(originalGroups))
	var tasks []model.Task
	require.NoError(t, model.DB.Order("id").Find(&tasks).Error)
	assert.NotEmpty(t, tasks)
	model.InitChannelCache()
	assert.NotEmpty(t, model.GetPricing())

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	require.NotPanics(t, func() { router.SetRouter(engine, router.WebAssets{BuildFS: buildFS, IndexPage: indexPage}) })
	for _, path := range []string{"/api/status", "/api/channel/", "/v1/models"} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if path == "/api/status" {
			assert.Equal(t, http.StatusOK, recorder.Code)
		} else {
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		}
	}
}

func importedInvariantDigest(t *testing.T, db *gorm.DB, table string, columns []string) string {
	t.Helper()
	fields := make([]string, len(columns))
	for i, column := range columns {
		fields[i] = `"` + strings.ReplaceAll(column, `"`, `""`) + `"::text`
	}
	rows, err := db.Raw(`SELECT ` + strings.Join(fields, ",") + ` FROM "` + table + `" ORDER BY id`).Rows()
	require.NoError(t, err)
	defer rows.Close()
	digest := sha256.New()
	for rows.Next() {
		values := make([]*string, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))
		encoded, err := common.Marshal(values)
		require.NoError(t, err)
		_, _ = digest.Write(encoded)
		_, _ = digest.Write([]byte{0})
	}
	require.NoError(t, rows.Err())
	return hex.EncodeToString(digest.Sum(nil))
}
