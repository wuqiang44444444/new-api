package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementsExcludeChannelTestsButKeepWalletAndProviderUsage(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]User{
		{Id: 1, Username: "customer", AffCode: "customer", Quota: 1000},
		{Id: 2, Username: "tests-only", AffCode: "tests-only", Quota: 2000},
	}).Error)
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "provider"}).Error)
	logs := []Log{
		{UserId: 1, Type: LogTypeConsume, TokenName: "playground-default", Quota: 100, PromptTokens: 10},
		{UserId: 1, Type: LogTypeConsume, TokenId: 9, TokenName: "模型测试", Content: "模型测试", Quota: 200, PromptTokens: 20},
		{UserId: 1, Type: LogTypeConsume, TokenName: "模型测试", Content: "模型测试", Quota: 9000, PromptTokens: 900},
		{UserId: 1, Type: LogTypeRefund, TokenId: 9, TokenName: "模型测试", Content: "模型测试", Quota: 50},
		{UserId: 2, Type: LogTypeConsume, TokenName: "模型测试", Content: "模型测试", Quota: 8000, PromptTokens: 800},
	}
	for i := range logs {
		logs[i].CreatedAt = int64(1100 + i)
		logs[i].ChannelId = 21
		logs[i].ModelName = "model"
		logs[i].Other = `{"group_ratio":1,"model_ratio":1,"cache_tokens":2,"cache_write_tokens":3}`
	}
	require.NoError(t, db.Create(&logs).Error)

	for _, dimension := range []string{"api_key", "channel"} {
		statement, err := GetBillingCustomerStatement(1, 1000, 1500, dimension, 0, "", "")
		require.NoError(t, err)
		assert.EqualValues(t, 2, statement.Summary.Requests)
		assert.EqualValues(t, 300, statement.Summary.GrossQuota)
		assert.EqualValues(t, 50, statement.Summary.RefundQuota)
		assert.EqualValues(t, 250, statement.Summary.NetQuota)
		assert.EqualValues(t, 1000, statement.CurrentBalance)
		if dimension == "api_key" {
			require.Len(t, statement.Groups, 2)
			for _, group := range statement.Groups {
				if group.Id == 0 {
					assert.Equal(t, "playground-default", group.Name)
					assert.EqualValues(t, 1, group.Usage.Requests)
					assert.EqualValues(t, 100, group.Usage.NetQuota)
				}
			}
		}
	}

	list, err := GetBillingCustomerStatementList(1000, 1500, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.Equal(t, 1, list.Items[0].UserId)
	assert.EqualValues(t, 1, list.Summary.CustomerCount)
	assert.EqualValues(t, 250, list.Summary.Usage.NetQuota)

	_, legacy, err := GetUserBillingStatement(1, 1000, 1500, 0, "")
	require.NoError(t, err)
	assert.EqualValues(t, 2, legacy.Requests)
	assert.EqualValues(t, 250, legacy.NetQuota)
	_, breakdown, err := GetUserBillingStatementBreakdown(1, 1000, 1500, 0, "")
	require.NoError(t, err)
	assert.EqualValues(t, 2, breakdown.Requests)
	assert.EqualValues(t, 300, breakdown.GrossQuota)
	require.NotNil(t, breakdown.Cache)
	assert.EqualValues(t, 6, breakdown.Cache.WriteTokens)

	provider, err := GetProviderBillingSummary(1000, 1500, 1000, 21, "", "", 1)
	require.NoError(t, err)
	require.Len(t, provider.Channels, 1)
	assert.EqualValues(t, 4, provider.Channels[0].Usage.Requests)
	assert.EqualValues(t, 1730, provider.Channels[0].Usage.InputTokens)
	assert.EqualValues(t, 12, provider.Channels[0].Usage.CacheWriteTokens)
	var count int64
	require.NoError(t, db.Model(&Log{}).Count(&count).Error)
	assert.EqualValues(t, len(logs), count, "projections must not rewrite or delete original logs")
}

func TestCustomerSettlementScopeKeepsUnmarkedFreeAndRefundRecords(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	logs := []Log{
		{Type: LogTypeConsume, TokenName: "模型测试", Content: "ordinary request"},
		{Type: LogTypeConsume, TokenName: "playground-default", Content: "模型测试"},
		{Type: LogTypeConsume},
		{Type: LogTypeRefund, TokenName: "模型测试", Content: "模型测试", Quota: 5},
	}
	require.NoError(t, db.Create(&logs).Error)
	// Nullable historical fields must not disappear through SQL's NOT(NULL).
	require.NoError(t, db.Model(&Log{}).Where("id = ?", logs[2].Id).Updates(map[string]any{"token_name": nil, "content": nil}).Error)
	var got []Log
	require.NoError(t, db.Scopes(customerSettlementLogs).Order("id").Find(&got).Error)
	require.Len(t, got, len(logs))
	for i := range logs {
		assert.Equal(t, logs[i].Id, got[i].Id)
	}
}
