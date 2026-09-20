package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderSummaryCountsSettlementUsageWithoutCustomerRefunds(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "provider"}).Error)
	logs := []Log{
		{CreatedAt: 900, Type: LogTypeConsume, Other: `{"task_id":"video-task","task_billing_event":"create","model_ratio":1,"group_ratio":1}`},
		{CreatedAt: 1100, Type: LogTypeRefund, CompletionTokens: 80, Other: `{"task_id":"video-task","actual_quota":40,"pre_consumed_quota":100,"model_ratio":1,"group_ratio":1}`},
		{CreatedAt: 1101, Type: LogTypeRefund, CompletionTokens: 999, Other: `{"task_id":"video-task","task_billing_event":"refund","model_ratio":1,"group_ratio":1}`},
		{CreatedAt: 1102, Type: LogTypeRefund, CompletionTokens: 999, Other: `{"model_ratio":1,"group_ratio":1}`},
	}
	for i := range logs {
		logs[i].ChannelId = 21
		logs[i].ModelName = "video"
		logs[i].UserId = 1
		logs[i].Quota = 60
	}
	require.NoError(t, db.Create(&logs).Error)
	for _, period := range []struct{ start, requests int64 }{{800, 1}, {1000, 0}} {
		summary, err := GetProviderBillingURLSummary(period.start, 1500, 1000, "")
		require.NoError(t, err)
		require.Len(t, summary.Groups, 1)
		assert.EqualValues(t, 80, summary.Groups[0].Usage.OutputTokens)
		assert.EqualValues(t, period.requests, summary.Groups[0].Usage.Requests)
	}
}
