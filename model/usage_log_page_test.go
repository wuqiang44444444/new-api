package model

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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
)

func TestDeferredUsageLogTokenRejectsLegacyAndReportsExpiry(t *testing.T) {
	setupBillingReconciliationTestDB(t)
	ctx := context.Background()
	filter := LogFilter{StartTimestamp: 100, EndTimestamp: 200}
	page, err := GetUsageLogPage(ctx, 1, filter, "start", 1, 20, 0, common.RoleCommonUser, UsageLogTotalDeferred)
	require.NoError(t, err)
	_, err = GetNumberedUsageLogPage(ctx, 1, filter, page.PageSnapshot, 1, 20, 0, common.RoleCommonUser)
	require.Error(t, err, "a deferred token must not decode as a legacy zero total")

	raw, err := common.Marshal(struct {
		UserID int
		LogFilter
	}{1, filter})
	require.NoError(t, err)
	hash := sha256.Sum256(raw)
	scope := "deferred:usage-log-page:" + base64.RawURLEncoding.EncodeToString(hash[:])
	plain, err := common.Marshal(usageLogPageSnapshot{Start: 100, End: 200, Until: 1, TotalPending: true})
	require.NoError(t, err)
	expired, err := common.EncryptShortLivedSecretForScope(scope, string(plain))
	require.NoError(t, err)
	_, err = GetUsageLogPage(ctx, 1, filter, expired, 1, 20, 0, common.RoleCommonUser, UsageLogTotalOnly)
	require.ErrorIs(t, err, ErrUsageLogSnapshotExpired)
}

func TestUsageLogStatisticsKeepResolvedWindow(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, CreatedAt: 10, RequestId: "target", Type: LogTypeConsume, Quota: 5},
		{UserId: 1, CreatedAt: 20_000, RequestId: "target", Type: LogTypeConsume, Quota: 10},
		{UserId: 1, CreatedAt: 30_000, RequestId: "target", Type: LogTypeConsume, Quota: 20},
	}).Error)
	stats, err := GetLogRangeStatistics(context.Background(), 0, LogFilter{RequestId: "target", EndTimestamp: 20_001}, 0, true)
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.Total)
	assert.Equal(t, 15, stats.Quota)
}

func TestUsageLogDeferredTotalCountsOnlyOnDemand(t *testing.T) {
	t.Run("SQLite", func(t *testing.T) {
		verifyUsageLogDeferredTotal(t, setupBillingReconciliationTestDB(t))
	})
	for _, database := range []struct {
		name string
		env  string
		open func(string) gorm.Dialector
	}{
		{"PostgreSQL", "TEST_USAGE_LOG_POSTGRES_DSN", postgres.Open},
		{"MySQL", "TEST_USAGE_LOG_MYSQL_DSN", mysql.Open},
	} {
		t.Run(database.name, func(t *testing.T) {
			dsn := os.Getenv(database.env)
			if dsn == "" {
				t.Skip("configure a dedicated empty database for integration testing")
			}
			db, err := gorm.Open(database.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			previous := LOG_DB
			t.Cleanup(func() {
				LOG_DB = previous
				_ = sqlDB.Close()
			})
			require.False(t, db.Migrator().HasTable(&Log{}), "test database must be empty")
			require.NoError(t, db.AutoMigrate(&Log{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&Log{})) })
			LOG_DB = db
			verifyUsageLogDeferredTotal(t, db)
		})
	}
}

func verifyUsageLogDeferredTotal(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	filter := LogFilter{StartTimestamp: 100, EndTimestamp: 200}
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, CreatedAt: 150, RequestId: "first"},
		{UserId: 1, CreatedAt: 150, RequestId: "second"},
		{UserId: 1, CreatedAt: 150, RequestId: "third"},
		{UserId: 2, CreatedAt: 150, RequestId: "other user"},
	}).Error)
	countCalls := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:usage_log_counts", func(tx *gorm.DB) {
		if strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "count(*)") {
			countCalls++
		}
	}))

	first, err := GetUsageLogPage(ctx, 1, filter, "start", 1, 2, 0, common.RoleCommonUser, UsageLogTotalDeferred)
	require.NoError(t, err)
	assert.True(t, first.TotalPending)
	assert.True(t, first.HasMore)
	require.Len(t, first.Items, 2)
	assert.Equal(t, "third", first.Items[0].RequestId)
	assert.Zero(t, countCalls)

	require.NoError(t, db.Create(&Log{UserId: 1, CreatedAt: 150, RequestId: "late"}).Error)
	next, err := GetUsageLogPage(ctx, 1, filter, first.PageSnapshot, 2, 2, 0, common.RoleCommonUser, UsageLogTotalDeferred)
	require.NoError(t, err)
	assert.False(t, next.HasMore)
	require.Len(t, next.Items, 1)
	assert.Equal(t, "first", next.Items[0].RequestId)
	assert.Zero(t, countCalls)

	_, err = GetUsageLogPage(ctx, 2, filter, first.PageSnapshot, 1, 2, 0, common.RoleCommonUser, UsageLogTotalOnly)
	require.Error(t, err)
	counted, err := GetUsageLogPage(ctx, 1, filter, first.PageSnapshot, 1, 2, 0, common.RoleCommonUser, UsageLogTotalOnly)
	require.NoError(t, err)
	assert.Empty(t, counted.Items)
	assert.False(t, counted.TotalPending)
	assert.EqualValues(t, 3, counted.Total)
	assert.Equal(t, 1, countCalls)
	again, err := GetUsageLogPage(ctx, 1, filter, counted.PageSnapshot, 1, 2, 0, common.RoleCommonUser, UsageLogTotalOnly)
	require.NoError(t, err)
	assert.EqualValues(t, 3, again.Total)
	assert.Equal(t, 1, countCalls)

	require.NoError(t, db.Where("request_id = ?", "first").Delete(&Log{}).Error)
	last, err := GetUsageLogPage(ctx, 1, filter, counted.PageSnapshot, 1, 2, 0, common.RoleCommonUser, UsageLogLastPage)
	require.NoError(t, err)
	assert.Equal(t, 1, last.Page)
	assert.EqualValues(t, 2, last.Total)
	require.Len(t, last.Items, 2)
	assert.Equal(t, "third", last.Items[0].RequestId)
	assert.Equal(t, "second", last.Items[1].RequestId)
}

func TestUsageLogRequestIDSearchUsesAllowedRange(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	ctx := context.Background()
	now := time.Now().Unix()
	const days = int64(2 * 24 * 3600)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, CreatedAt: now - days/2, RequestId: "target", Type: LogTypeError},
		{UserId: 1, CreatedAt: now - days/2, RequestId: "target", Type: LogTypeConsume},
		{UserId: 1, CreatedAt: now - days*2, RequestId: "target"},
		{UserId: 2, CreatedAt: now - days/2, RequestId: "target"},
		{UserId: 1, CreatedAt: now - days/2, RequestId: "target-other"},
	}).Error)
	filter := LogFilter{RequestId: "target"}
	page, err := GetUsageLogPage(ctx, 1, filter, "start", 1, 20, days, common.RoleCommonUser, UsageLogTotalDeferred)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, LogTypeConsume, page.Items[0].Type)
	assert.Equal(t, LogTypeError, page.Items[1].Type)
	assert.EqualValues(t, 2, page.Total)
	assert.False(t, page.TotalPending)
	assert.False(t, page.HasMore)
	assert.Equal(t, days, page.RangeEnd-page.RangeStart)
	stats, err := GetLogRangeStatistics(ctx, 1, LogFilter{
		RequestId: filter.RequestId, StartTimestamp: page.RangeStart, EndTimestamp: page.RangeEnd,
	}, days, true)
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.Total)

	legacy, err := GetLogRangeStatistics(ctx, 1, filter, days, false)
	require.NoError(t, err)
	assert.Zero(t, legacy.Total)

	filter.StartTimestamp, filter.EndTimestamp = now-3600, now
	explicit, err := GetUsageLogPage(ctx, 1, filter, "start", 1, 20, days, common.RoleCommonUser, UsageLogTotalDeferred)
	require.NoError(t, err)
	assert.Empty(t, explicit.Items)
	assert.False(t, explicit.TotalPending)
	assert.Zero(t, explicit.Total)
}
