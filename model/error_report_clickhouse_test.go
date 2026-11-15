package model

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
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
	require.NoError(t, MigrateErrorEvents())
	require.NoError(t, MigrateErrorEvents())
	rows := []ErrorEvent{{CreatedAt: 3600, ChannelId: 9, RequestId: "duplicate", TaskId: "first"}, {CreatedAt: 3600, ChannelId: 9, RequestId: "duplicate", TaskId: "second"}, {CreatedAt: 7200, RequestId: "outside", TaskId: "outside"}}
	require.NoError(t, logDB.Create(&rows).Error)
	slice, err := ReadDailyReportSlice(context.Background(), 3600, 7200)
	require.NoError(t, err)
	assert.EqualValues(t, 2, slice.Total)
	assert.EqualValues(t, 2, slice.Signals[DailyErrorOther])
	require.Len(t, slice.Groups, 1)
	assert.Equal(t, 9, slice.Groups[0].ChannelID)

	require.NoError(t, mainDB.AutoMigrate(&User{}))
	require.NoError(t, mainDB.Create(&[]User{
		{Id: 1, Username: "root", AffCode: "r1", Role: common.RoleRootUser},
		{Id: 2, Username: "customer", AffCode: "c2", Role: common.RoleCommonUser},
	}).Error)
	var events []ErrorEvent
	for i := range 70 {
		events = append(events, ErrorEvent{CreatedAt: 10800, RequestId: fmt.Sprintf("root-%03d", i), UserId: 1,
			EventType: clienterrlog.EventAPIError, Status: 403, Reason: clienterrlog.WalletQuotaInsufficient})
	}
	for i := range 7 {
		events = append(events, ErrorEvent{CreatedAt: 10800, RequestId: fmt.Sprintf("customer-%03d", i), UserId: 2,
			EventType: clienterrlog.EventAPIError, Status: 403, Reason: clienterrlog.WalletQuotaInsufficient})
	}
	require.NoError(t, logDB.Create(&events).Error)
	samples, err := ReadMailErrorSamples(context.Background(), MailErrorScope{Start: 10800, End: 10801, CustomerOnly: true, FailuresOnly: true})
	require.NoError(t, err)
	require.Len(t, samples, 5)
	for i, sample := range samples {
		assert.Equal(t, fmt.Sprintf("customer-%03d", 6-i), sample.RequestID)
	}
}
