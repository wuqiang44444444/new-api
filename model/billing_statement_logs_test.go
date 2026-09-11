package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBillingStatementLogsPreserveScopeAndNetAcrossPages(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1, Username: "renamed"}).Error)
	logs := []Log{
		{UserId: 1, Username: "old-name", Quota: 100, TokenName: "playground-default"},
		{UserId: 1, Username: "", Quota: 200, TokenName: "playground-default"},
		{UserId: 1, Quota: 40, Type: LogTypeRefund},
		{UserId: 1, Quota: 9000, TokenName: "模型测试", Content: "模型测试"},
		{UserId: 1, Quota: 700, TokenId: 90},
		{UserId: 2, Quota: 800},
		{UserId: 1, Quota: 900, Other: `{"model_price":1,"group_ratio":1}`},
		{UserId: 1, Quota: 1000, ModelName: "another-model"},
	}
	for i := range logs {
		logs[i].CreatedAt = 1100 + int64(i)
		if logs[i].Type == 0 {
			logs[i].Type = LogTypeConsume
		}
		if logs[i].ModelName == "" {
			logs[i].ModelName = "model"
		}
		if logs[i].Other == "" {
			logs[i].Other = `{"model_ratio":1,"group_ratio":1,"admin_info":{"diagnostic":"private"}}`
		}
	}
	require.NoError(t, db.Create(&logs).Error)
	zero := 0
	filter := BillingStatementLogFilter{UserId: 1, Start: 1000, End: 1500, TokenId: &zero, ModelName: "model", BillingMode: "token"}
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		var seen int
		for page := 1; page <= 3; page++ {
			result, err := GetBillingStatementLogs(filter, page, 1, role)
			require.NoError(t, err)
			assert.EqualValues(t, 3, result.Total)
			assert.EqualValues(t, 260, result.Quota)
			require.Len(t, result.Items, 1)
			assert.Equal(t, 1, result.Items[0].UserId)
			assert.Zero(t, result.Items[0].TokenId)
			assert.NotEqual(t, "模型测试", result.Items[0].TokenName)
			if role == common.RoleCommonUser {
				assert.NotContains(t, result.Items[0].Other, "private")
			}
			seen++
		}
		assert.Equal(t, 3, seen)
	}
	statement, err := GetBillingCustomerStatement(1, 1000, 1500, "api_key", 0, "model", "token")
	require.NoError(t, err)
	for _, group := range statement.Groups {
		if group.Id == 0 {
			assert.EqualValues(t, 260, group.Usage.NetQuota)
			encoded, err := common.Marshal(group.Models[0].DetailFilter)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"token_id":0`)
		}
	}
	filter.TokenId = nil
	result, err := GetBillingStatementLogs(filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, 4, result.Total)
	assert.EqualValues(t, 960, result.Quota)
	filter.TokenId = &zero
	filter.BillingMode = "per_call"
	result, err = GetBillingStatementLogs(filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, 1, result.Total)
	assert.EqualValues(t, 900, result.Quota)
}

func TestHistoricalChannelTestCacheUnknownIsAReadOnlyProjection(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "provider"}).Error)
	logs := []Log{
		{Other: `{"model_ratio":1,"group_ratio":1}`},
		{Other: `{"model_ratio":1,"group_ratio":1,"cache_write_tokens":0}`},
		{Other: `{"model_ratio":1,"group_ratio":1,"cache_creation_tokens":12}`},
		{TokenId: 9, Other: `{"model_ratio":1,"group_ratio":1}`},
		{Other: `{"model_price":1,"group_ratio":1}`},
	}
	for i := range logs {
		logs[i].UserId = 1
		logs[i].Type = LogTypeConsume
		logs[i].CreatedAt = 1100
		logs[i].ChannelId = 21
		logs[i].ModelName = "model"
		logs[i].TokenName = "模型测试"
		logs[i].Content = "模型测试"
	}
	require.NoError(t, db.Create(&logs).Error)
	summary, err := GetProviderBillingSummary(1000, 1500, 1000, 21, "", "", 1)
	require.NoError(t, err)
	require.Len(t, summary.Channels, 1)
	assert.EqualValues(t, 5, summary.Channels[0].Usage.Requests)
	assert.EqualValues(t, 12, summary.Channels[0].Usage.CacheWriteTokens)
	require.NotNil(t, summary.DataQuality)
	assert.EqualValues(t, 1, summary.DataQuality.CacheWriteUnavailableRequests)
	assert.Equal(t, "partial", summary.Channels[0].DataQuality.Status)
	for _, item := range summary.Channels[0].Models {
		if item.BillingMode == BillingReconciliationModeToken {
			assert.EqualValues(t, 1, item.DataQuality.CacheWriteUnavailableRequests)
		} else {
			assert.Zero(t, item.DataQuality.CacheWriteUnavailableRequests)
		}
	}
	for _, format := range []func([]*Log){FormatRootLogs, FormatAdminLogs, func(logs []*Log) { formatUserLogs(logs, 0) }} {
		var projection []*Log
		require.NoError(t, db.Order("id").Find(&projection).Error)
		format(projection)
		assert.Contains(t, projection[0].Other, `"cache_write_unavailable":true`)
		for _, log := range projection[1:] {
			assert.NotContains(t, log.Other, "cache_write_unavailable")
		}
	}
	var persisted []Log
	require.NoError(t, db.Order("id").Find(&persisted).Error)
	require.Len(t, persisted, len(logs))
	for i := range logs {
		assert.Equal(t, logs[i].Other, persisted[i].Other)
	}
}
