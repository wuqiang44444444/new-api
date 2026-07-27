package model

import (
	"net/url"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

func TestGetUserBillingStatementWithClickHouse(t *testing.T) {
	dsn := os.Getenv("TEST_CLICKHOUSE_BILLING_DSN")
	if dsn == "" {
		t.Skip("TEST_CLICKHOUSE_BILLING_DSN is not configured")
	}
	parsedDSN, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "/billing_statement_test", parsedDSN.Path, "integration test must use an isolated billing_statement_test database")

	db, err := gorm.Open(clickhouse.Open(dsn), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	previousLogDB := LOG_DB
	previousLogDatabaseType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS logs").Error)
	t.Cleanup(func() {
		_ = db.Exec("DROP TABLE IF EXISTS logs").Error
		_ = sqlDB.Close()
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogDatabaseType)
	})
	require.NoError(t, migrateClickHouseLogDB())

	logs := []Log{
		{Id: 1, UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 11, TokenName: "task-key", ModelName: "model-a", Quota: 1000, UseTime: 2, Other: `{"is_task":true}`},
		{Id: 2, UserId: 7, CreatedAt: 1200, Type: LogTypeRefund, TokenId: 11, TokenName: "task-key", ModelName: "model-a", CompletionTokens: 250, Quota: 300, Other: `{"task_id":"refund-task"}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	items, summary, err := GetUserBillingStatement(7, 1000, 1300, 0, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.EqualValues(t, 1, items[0].Requests)
	assert.EqualValues(t, 250, items[0].CompletionTokens)
	assert.EqualValues(t, 1000, items[0].GrossQuota)
	assert.EqualValues(t, 300, items[0].RefundQuota)
	assert.EqualValues(t, 700, items[0].NetQuota)
	assert.InDelta(t, 2, items[0].AverageUseTimeSeconds, 0.0001)
	assert.EqualValues(t, 1, summary.Requests)
	assert.EqualValues(t, 250, summary.TotalTokens)
	assert.EqualValues(t, 700, summary.NetQuota)
}
