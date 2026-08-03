package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetUserBillingStatementAggregatesSettlementLogs(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&Log{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})

	logs := []Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 11, TokenName: "old-key", ModelName: "model-a", PromptTokens: 100, CompletionTokens: 20, Quota: 1200, UseTime: 2},
		{UserId: 7, CreatedAt: 1200, Type: LogTypeConsume, TokenId: 11, TokenName: "current-key", ModelName: "model-a", PromptTokens: 200, CompletionTokens: 40, Quota: 2400, UseTime: 4, IsStream: true},
		{UserId: 7, CreatedAt: 1250, Type: LogTypeConsume, TokenId: 11, TokenName: "current-key", ModelName: "model-a", CompletionTokens: 10, Quota: 50, Other: `{"task_id":"async-1"}`},
		{UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, TokenId: 12, TokenName: "second-key", ModelName: "model-b", PromptTokens: 50, CompletionTokens: 10, Quota: 600, UseTime: 3, IsStream: true},
		{UserId: 7, CreatedAt: 1400, Type: LogTypeRefund, TokenId: 11, TokenName: "current-key", ModelName: "model-a", Quota: 100},
		{UserId: 8, CreatedAt: 1300, Type: LogTypeConsume, TokenId: 11, TokenName: "other-user-key", ModelName: "model-a", PromptTokens: 999, CompletionTokens: 999, Quota: 999},
		{UserId: 7, CreatedAt: 2000, Type: LogTypeConsume, TokenId: 11, TokenName: "current-key", ModelName: "model-a", PromptTokens: 999, CompletionTokens: 999, Quota: 999},
	}
	require.NoError(t, db.Create(&logs).Error)

	items, summary, err := GetUserBillingStatement(7, 1000, 1600, 0, "")
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, 11, items[0].TokenId)
	assert.Equal(t, "current-key", items[0].TokenName)
	assert.Equal(t, "model-a", items[0].ModelName)
	assert.EqualValues(t, 2, items[0].Requests)
	assert.EqualValues(t, 300, items[0].PromptTokens)
	assert.EqualValues(t, 70, items[0].CompletionTokens)
	assert.EqualValues(t, 370, items[0].TotalTokens)
	assert.EqualValues(t, 3650, items[0].GrossQuota)
	assert.EqualValues(t, 100, items[0].RefundQuota)
	assert.EqualValues(t, 3550, items[0].NetQuota)
	assert.InDelta(t, 3, items[0].AverageUseTimeSeconds, 0.0001)
	assert.EqualValues(t, 1, items[0].StreamRequests)

	assert.EqualValues(t, 3, summary.Requests)
	assert.EqualValues(t, 350, summary.PromptTokens)
	assert.EqualValues(t, 80, summary.CompletionTokens)
	assert.EqualValues(t, 430, summary.TotalTokens)
	assert.EqualValues(t, 4250, summary.GrossQuota)
	assert.EqualValues(t, 100, summary.RefundQuota)
	assert.EqualValues(t, 4150, summary.NetQuota)
	assert.InDelta(t, 3, summary.AverageUseTimeSeconds, 0.0001)
	assert.InDelta(t, 2.0/3.0, summary.StreamRatio, 0.0001)
	assert.InDelta(t, 0.3, summary.AverageRPM, 0.0001)
	assert.InDelta(t, 43, summary.AverageTPM, 0.0001)

	filteredItems, filteredSummary, err := GetUserBillingStatement(7, 1000, 1600, 12, "model-b")
	require.NoError(t, err)
	require.Len(t, filteredItems, 1)
	assert.Equal(t, "second-key", filteredItems[0].TokenName)
	assert.EqualValues(t, 1, filteredSummary.Requests)
	assert.EqualValues(t, 60, filteredSummary.TotalTokens)
	assert.EqualValues(t, 600, filteredSummary.GrossQuota)
	assert.Zero(t, filteredSummary.RefundQuota)
	assert.EqualValues(t, 600, filteredSummary.NetQuota)
}

func TestGetUserBillingStatementCountsAsyncTasksOnceAcrossSettlementAdjustments(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&Log{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})

	logs := []Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, TokenId: 11, TokenName: "task-key", ModelName: "model-refund", Quota: 1000, UseTime: 2, Other: `{"is_task":true}`},
		{UserId: 7, CreatedAt: 1200, Type: LogTypeRefund, TokenId: 11, TokenName: "task-key", ModelName: "model-refund", CompletionTokens: 250, Quota: 300, UseTime: 99, IsStream: true, Other: `{"task_id":"refund-task"}`},
		{UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, TokenId: 11, TokenName: "task-key", ModelName: "model-supplement", Quota: 1000, UseTime: 3, IsStream: true, Other: `{"is_task":true}`},
		{UserId: 7, CreatedAt: 1400, Type: LogTypeConsume, TokenId: 11, TokenName: "task-key", ModelName: "model-supplement", CompletionTokens: 400, Quota: 200, UseTime: 99, IsStream: true, Other: `{"task_id":"supplement-task"}`},
		{UserId: 7, CreatedAt: 900, Type: LogTypeConsume, TokenId: 11, TokenName: "task-key", ModelName: "model-cross-period", Quota: 500, UseTime: 4, Other: `{"is_task":true}`},
		{UserId: 7, CreatedAt: 1500, Type: LogTypeRefund, TokenId: 11, TokenName: "task-key", ModelName: "model-cross-period", CompletionTokens: 100, Quota: 500, Other: `{"task_id":"cross-period-task"}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	items, summary, err := GetUserBillingStatement(7, 1000, 1600, 0, "")
	require.NoError(t, err)
	require.Len(t, items, 3)

	itemsByModel := make(map[string]BillingStatementItem, len(items))
	for _, item := range items {
		itemsByModel[item.ModelName] = item
	}

	refundItem := itemsByModel["model-refund"]
	assert.EqualValues(t, 1, refundItem.Requests)
	assert.EqualValues(t, 250, refundItem.CompletionTokens)
	assert.EqualValues(t, 1000, refundItem.GrossQuota)
	assert.EqualValues(t, 300, refundItem.RefundQuota)
	assert.EqualValues(t, 700, refundItem.NetQuota)
	assert.InDelta(t, 2, refundItem.AverageUseTimeSeconds, 0.0001)
	assert.Zero(t, refundItem.StreamRequests)

	supplementItem := itemsByModel["model-supplement"]
	assert.EqualValues(t, 1, supplementItem.Requests)
	assert.EqualValues(t, 400, supplementItem.CompletionTokens)
	assert.EqualValues(t, 1200, supplementItem.GrossQuota)
	assert.Zero(t, supplementItem.RefundQuota)
	assert.EqualValues(t, 1200, supplementItem.NetQuota)
	assert.InDelta(t, 3, supplementItem.AverageUseTimeSeconds, 0.0001)
	assert.EqualValues(t, 1, supplementItem.StreamRequests)

	crossPeriodItem := itemsByModel["model-cross-period"]
	assert.Zero(t, crossPeriodItem.Requests)
	assert.EqualValues(t, 100, crossPeriodItem.CompletionTokens)
	assert.Zero(t, crossPeriodItem.GrossQuota)
	assert.EqualValues(t, 500, crossPeriodItem.RefundQuota)
	assert.Zero(t, crossPeriodItem.NetQuota)

	assert.EqualValues(t, 2, summary.Requests)
	assert.EqualValues(t, 750, summary.CompletionTokens)
	assert.EqualValues(t, 2200, summary.GrossQuota)
	assert.EqualValues(t, 800, summary.RefundQuota)
	assert.EqualValues(t, 1900, summary.NetQuota)
	assert.InDelta(t, 2.5, summary.AverageUseTimeSeconds, 0.0001)
	assert.InDelta(t, 0.5, summary.StreamRatio, 0.0001)
}
