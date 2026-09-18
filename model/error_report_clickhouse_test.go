package model

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

func TestErrorReportClickHouseDuplicateKeys(t *testing.T) {
	dsn := os.Getenv("TEST_ERROR_REPORT_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("requires isolated ClickHouse opt-in")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", parsed.Hostname(), "acceptance is restricted to local isolated databases")
	root, err := gorm.Open(clickhouse.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	name := "yuan_report_" + strings.ReplaceAll(common.GetUUID(), "-", "")
	require.NoError(t, root.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() {
		require.NoError(t, root.Exec("DROP DATABASE "+name).Error)
		conn, err := root.DB()
		require.NoError(t, err)
		conn.Close()
	})
	parsed.Path = "/" + name
	logDB, err := gorm.Open(clickhouse.Open(parsed.String()), &gorm.Config{})
	require.NoError(t, err)
	conn, err := logDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	mainDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	mainConn, err := mainDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { mainConn.Close() })
	previousDB, previousLog := DB, LOG_DB
	previousMain, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeClickHouse)
	t.Cleanup(func() { DB, LOG_DB = previousDB, previousLog; common.SetDatabaseTypes(previousMain, previousLogType) })
	require.NoError(t, mainDB.AutoMigrate(&Channel{}))
	require.NoError(t, mainDB.Create(&Channel{Id: 9, Name: "fixture"}).Error)
	require.NoError(t, MigrateErrorEvents())
	require.NoError(t, MigrateErrorEvents())
	rows := []ErrorEvent{{CreatedAt: 3600, ChannelId: 9, RequestId: "duplicate", TaskId: "first"}, {CreatedAt: 3600, ChannelId: 9, RequestId: "duplicate", TaskId: "second"}, {CreatedAt: 7200, RequestId: "outside", TaskId: "outside"}}
	require.NoError(t, logDB.Create(&rows).Error)
	var tasks []string
	names, err := WalkErrorEventsForReport(context.Background(), 3600, 7200, func(e *ErrorEvent) error { tasks = append(tasks, e.TaskId); return nil })
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"first", "second"}, tasks)
	assert.Equal(t, "fixture", names[9])
}
