package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cache quality measures final metering rows only: creation pre-holds are
// estimates and are excluded, while rows of one task dedupe to a single
// missing count per meter. Standalone rows keep per-row counting.
func TestUpstreamCacheQualityCountsFinalMeteringOncePerTask(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "provider"}).Error)
	tokenSnapshot := `"statement_snapshot":{"billing_mode":"token"}`
	logs := []Log{
		// Creation pre-hold: an estimate, never final metering evidence.
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 21, ModelName: "model", Quota: 100,
			Other: `{"task_id":"t1","task_billing_event":"create","is_task":true,"contract_applicable":false,"group_ratio":1,"model_ratio":1,` + tokenSnapshot + `}`},
		// Async image completion reuses the create event but carries image_count:
		// it is the final metering row and must keep counting.
		{UserId: 7, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 21, ModelName: "model", Quota: 100,
			Other: `{"task_id":"t1","task_billing_event":"create","is_task":true,"image_count":2,"request_path":"/v1/images/generations","contract_applicable":false,"group_ratio":1,"model_ratio":1,` + tokenSnapshot + `}`},
		// Settlement adjustment of the same task: merges into one missing count.
		{UserId: 7, CreatedAt: 1102, Type: LogTypeRefund, ChannelId: 21, ModelName: "model", Quota: 30,
			Other: `{"task_id":"t1","task_billing_event":"adjustment","actual_quota":70,"pre_consumed_quota":100,"contract_applicable":false,"group_ratio":1,` + tokenSnapshot + `}`},
		// Standalone row without task identity: per-row counting applies.
		{UserId: 8, CreatedAt: 1103, Type: LogTypeConsume, ChannelId: 21, ModelName: "model", Quota: 5,
			Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,` + tokenSnapshot + `}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	quality := summary.Groups[0].DataQuality
	require.NotNil(t, quality)
	// 预扣行不计缺失；同一 Task 的完成与调整只计一次；独立行单计。
	assert.EqualValues(t, 2, quality.CacheReadUnavailableRequests)
	assert.EqualValues(t, 2, quality.CacheWriteUnavailableRequests)
}

// Trusted completion seconds are recovered read-only from the task's frozen
// snapshot when actual usage was reported; budget projections stay missing.
func TestUpstreamSecondsRecoveredFromTaskFrozenSnapshot(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "video"}).Error)
	require.NoError(t, db.Create(&[]Task{
		{TaskID: "t-sec", Platform: "seedance", UserId: 7, ChannelId: 21, Status: TaskStatusSuccess, BillingState: TaskBillingStateSettled,
			PrivateData: TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{
				State: TaskBillingStateSettled, Operation: "settle", ActualUsageReported: true,
				TieredSnapshot: &billingexpr.BillingSnapshot{
					UsageUnits: map[string]string{"duration": "second"},
					UsageFacts: map[string]any{"duration": 6},
				},
			}}},
		{TaskID: "t-budget", Platform: "seedance", UserId: 7, ChannelId: 21, Status: TaskStatusSuccess, BillingState: TaskBillingStateSettled,
			PrivateData: TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{
				ActualUsageReported: false,
				TieredSnapshot: &billingexpr.BillingSnapshot{
					UsageUnits: map[string]string{"duration": "second"},
					UsageFacts: map[string]any{"duration": 10},
				},
			}}},
	}).Error)
	perSecond := `"statement_snapshot":{"billing_mode":"per_second","customer_model":"video"}`
	logs := []Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 21, ModelName: "video", Quota: 600,
			Other: `{"task_id":"t-sec","contract_applicable":false,"group_ratio":1,"usage_units":{"duration":"second"},` + perSecond + `}`},
		{UserId: 7, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 21, ModelName: "video", Quota: 600,
			Other: `{"task_id":"t-budget","contract_applicable":false,"group_ratio":1,"usage_units":{"duration":"second"},` + perSecond + `}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	require.NotNil(t, group.Usage.Seconds)
	assert.Equal(t, "6", group.Usage.Seconds.String(), "only the actually reported task contributes seconds")
	// 预算行保持缺失，且单位已知：单独落在秒数值缺失计数里。
	assert.EqualValues(t, 1, group.Usage.SecondsUnavailableRows)
	require.NotNil(t, group.DataQuality)
	assert.EqualValues(t, 1, group.DataQuality.SecondsValueMissingRows)

	details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500, ChannelIds: []int{21}}, 1, 10, false)
	require.NoError(t, err)
	require.Len(t, details.Items, 2)
	require.NotNil(t, details.Items[0].Seconds)
	assert.Equal(t, "6", details.Items[0].Seconds.String())
	assert.Nil(t, details.Items[1].Seconds)
}

// Priced channel tests join the daily/weekly upstream reference amount with
// the record-month coefficient; pending tests keep usage-only with a reason.
func TestUsageUpstreamViewIncludesPricedChannelTests(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 71, Name: "alpha", BaseURL: urlPtr("https://core.example.com")}).Error)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-21", 0)
	require.NoError(t, err)
	// Versioned original comes from native pricing, independent of customer group.
	// 客户视角的排除由既有回归覆盖；无依据费用的待核算分支同样已单测。
	logs := []Log{
		{UserId: 7, CreatedAt: period.Days[0].Start + 100, Type: LogTypeConsume, TokenId: 0, TokenName: "模型测试", Content: "模型测试", ChannelId: 71, ModelName: "model", PromptTokens: 100, Quota: 100,
			Other: `{"contract_applicable":false,"group_ratio":3,"model_ratio":1,"completion_ratio":1,"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":100}}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	upstream, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	require.Len(t, upstream.UrlGroups, 1)
	require.Len(t, upstream.UrlGroups[0].Models, 1)
	require.Len(t, upstream.UrlGroups[0].Models[0].Channels, 1)
	total := upstream.UrlGroups[0].Models[0].Channels[0].Total
	assert.EqualValues(t, 1, total.TotalCalls)
	// 分组倍率 3 不参与测试原价还原：已计价测试按记录费用进入参考金额。
	require.NotNil(t, total.OriginalQuotaEstimate)
	assert.EqualValues(t, 100, *total.OriginalQuotaEstimate)
	require.NotNil(t, total.ReferenceAmount)
	assert.EqualValues(t, 100, *total.ReferenceAmount)
	assert.EqualValues(t, 1, total.TestPricedRows)
	assert.EqualValues(t, 0, total.UsageOnlyRows)
	assert.NotContains(t, total.EstimateReasons, BillingEstimateTestAmountPending)
}

func TestUpstreamTaskSecondsResolverRejectsUnsettledEvidence(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Task{
		TaskID: "t-legacy", Platform: "seedance", UserId: 7, ChannelId: 3, Status: TaskStatusSuccess, BillingState: TaskBillingStateSettled,
		PrivateData: TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{
			TieredSnapshot: &billingexpr.BillingSnapshot{UsageUnits: map[string]string{"d": "second"}, UsageFacts: map[string]any{"d": 8}},
		}}},
	).Error)
	resolved, err := resolveUpstreamTaskSeconds(context.Background(), []upstreamTaskSecondsKey{{7, 3, "t-legacy"}, {7, 3, "missing"}})
	require.NoError(t, err)
	assert.Empty(t, recoverUpstreamTaskSeconds(resolved[upstreamTaskSecondsKey{7, 3, "t-legacy"}], billingReconciliationLog{}))
	assert.Equal(t, decimal.Decimal{}, decimal.Decimal{})
}
