package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogCursorPreservesFilterAndSkipsNewerInserts(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	ctx := context.Background()
	filter := LogFilter{StartTimestamp: 100, EndTimestamp: 200}
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, CreatedAt: 150, Type: LogTypeConsume, RequestId: "first", Quota: 20},
		{UserId: 1, CreatedAt: 150, Type: LogTypeConsume, RequestId: "second", Quota: 30,
			Content: "Full charge explanation", Other: `{"cache_tokens":12,"model_ratio":2,"task_id":"job","is_model_mapped":true,"upstream_model_name":"private","admin_info":{"task_plugin":{"key":"seedance-link"}}}`},
		{UserId: 2, CreatedAt: 150, Type: LogTypeConsume, RequestId: "other", Quota: 999},
	}).Error)
	first, err := GetLogCursorPage(ctx, 1, filter, "start", 1, 1, 0, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	assert.Equal(t, "second", first.Items[0].RequestId)
	assert.Empty(t, first.Items[0].Content)
	assert.JSONEq(t, `{"cache_tokens":12,"task_id":"job"}`, first.Items[0].Other)
	require.Len(t, first.DetailRefs, 1)
	detail, err := GetLogCursorDetail(ctx, 1, first.DetailRefs[0])
	require.NoError(t, err)
	assert.Equal(t, "Full charge explanation", detail.Content)
	assert.Contains(t, detail.Other, "model_ratio")
	assert.NotContains(t, detail.Other, "upstream_model_name")
	_, err = GetLogCursorDetail(ctx, 2, first.DetailRefs[0])
	require.Error(t, err)
	require.NotEmpty(t, first.NextCursor)
	require.NoError(t, db.Create(&Log{UserId: 1, CreatedAt: 150, Type: LogTypeConsume, RequestId: "late", Quota: 10}).Error)
	second, err := GetLogCursorPage(ctx, 1, filter, first.NextCursor, 2, 1, 0, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Equal(t, "first", second.Items[0].RequestId)
	assert.Empty(t, second.NextCursor)
	_, err = GetLogCursorPage(ctx, 2, filter, first.NextCursor, 2, 1, 0, common.RoleCommonUser)
	require.Error(t, err)
	stats, err := GetLogRangeStatistics(ctx, 1, filter, 0)
	require.NoError(t, err)
	assert.EqualValues(t, 3, stats.Total)
	assert.Equal(t, 60, stats.Quota)
}

func TestBillingStatementFundGroupsKeepCrossDayRefundSeparate(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	rows := []Log{
		{UserId: 7, TokenId: 11, ChannelId: 8, ModelName: "video", Type: LogTypeConsume, Quota: 100, CreatedAt: start + 5, Other: `{"task_id":"task-a"}`},
		{UserId: 7, TokenId: 11, ChannelId: 8, ModelName: "video", Type: LogTypeRefund, Quota: 30, CreatedAt: start + 6, Other: `{"task_id":"task-a"}`},
		{UserId: 7, TokenId: 11, ChannelId: 8, ModelName: "video", Type: LogTypeRefund, Quota: 5, CreatedAt: start + billingReportDaySeconds, Other: `{"task_id":"task-a"}`},
	}
	require.NoError(t, db.Create(&rows).Error)
	ctx := context.Background()
	for _, day := range []int64{start, start + billingReportDaySeconds} {
		require.NoError(t, BuildBillingReportDay(ctx, day, "billing-test", "runner", nil, nil))
	}
	filter := BillingStatementLogFilter{UserId: 7, Start: start, End: start + 2*billingReportDaySeconds - 1, GroupFunds: true}
	ctx, _, err := ResolveBillingReport(ctx, filter.Start, filter.End, "")
	require.NoError(t, err)
	result, err := GetBillingStatementLogs(ctx, filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	require.NotNil(t, result.FundGroups)
	groups := *result.FundGroups
	require.Len(t, groups, 2)
	assert.EqualValues(t, -5, groups[0].NetQuota)
	assert.EqualValues(t, 70, groups[1].NetQuota)
	assert.EqualValues(t, 65, result.Quota)
	filter.GroupFunds = false
	filter.FundGroup = groups[1].Key
	events, err := GetBillingStatementLogs(ctx, filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.Len(t, events.Items, 2)
	assert.EqualValues(t, 70, events.Quota)
}

func TestQuotaDataFlushChunkCanBeRetriedAfterCommit(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.AutoMigrate(&QuotaData{}, &quotaDataFlushReceipt{}))
	ctx := context.Background()
	chunk := quotaDataFlushChunk{
		id:   "stable-chunk",
		rows: []*QuotaData{{UserID: 1, CreatedAt: 3600, Count: 2, Quota: 90, TokenUsed: 200}},
	}
	require.NoError(t, persistQuotaDataFlushChunk(ctx, db, chunk))
	require.NoError(t, persistQuotaDataFlushChunk(ctx, db, chunk))
	var got []QuotaData
	require.NoError(t, db.Find(&got).Error)
	require.Len(t, got, 1)
	assert.Equal(t, 2, got[0].Count)
	assert.Equal(t, 90, got[0].Quota)
	assert.Equal(t, 200, got[0].TokenUsed)
}
