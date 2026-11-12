package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishedCustomerUsageSplitsSameModelBySettledMeter(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.AutoMigrate(&usageReportDay{}, &usageReportDayRow{}, &SystemTaskLock{}, &SystemTask{}, &dailyReportStage{}, &dailyReportPayloadPart{}, &dailyReportRetry{}))
	ctx := context.Background()
	now := common.GetTimestamp()
	period, err := ResolveUsageAnalyticsPeriod("week", "2026-09-14", now)
	require.NoError(t, err)
	require.NoError(t, db.Create(&SystemTaskLock{Type: UsageReportTaskType, TaskID: "report", LockedBy: "runner", LockedUntil: now + 60}).Error)
	const expression = `param("quality") == "fixed" ? tier("image", fixed(0.04)) * image_count : tier("token", p + c)`
	makeLog := func(day int, id string, quota, prompt, output int, unit, tier string, count int) Log {
		t.Helper()
		fields := map[string]any{
			"billing_mode": "tiered_expr", "billing_unit": unit,
			"expr_b64":     base64.StdEncoding.EncodeToString([]byte(expression)),
			"matched_tier": tier, "image_count": count, "request_path": "/v1/images/generations",
		}
		if unit == "token" {
			fields["usage_semantic"] = "openai"
			fields["cache_tokens"] = 10
			fields["image_output"] = 25
		}
		other, err := common.Marshal(fields)
		require.NoError(t, err)
		return Log{UserId: 7, TokenId: 1, ModelName: "mixed", CreatedAt: period.Days[day].Start + 60,
			Type: LogTypeConsume, RequestId: id, Quota: quota, PromptTokens: prompt, CompletionTokens: output, Other: string(other)}
	}
	logs := []Log{
		makeLog(0, "token", 600, 40, 60, "token", "token", 1),
		makeLog(0, "images", 800, 200, 500, "request", "image", 2),
		makeLog(1, "more-images", 400, 500, 600, "request", "image", 1),
		makeLog(1, "missing-images", 300, 100, 200, "request", "image", 0),
		{UserId: 8, TokenId: 2, ModelName: "mixed", CreatedAt: period.Days[0].Start + 70, Type: LogTypeError,
			RequestId: "failed-other-customer"},
		{UserId: 7, TokenId: 1, ModelName: "mixed", CreatedAt: period.Days[1].Start + 70, Type: LogTypeError,
			RequestId: "failed", Other: `{"request_path":"/v1/images/generations"}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	for _, date := range []string{"2026-09-14", "2026-09-15"} {
		require.NoError(t, BuildUsageReportDay(ctx, date, "report", "runner", nil))
	}
	view, meta, err := GetPublishedAdminUsageCustomer(ctx, period, 7)
	require.NoError(t, err)
	frozen := WithUsageReportReferences(ctx, meta.References)
	query := UsageReportRankQuery{Kind: "model", Metric: "tokens", Page: 1, Size: 10}
	all, err := GetPublishedUsageRanks(frozen, period, query)
	require.NoError(t, err)
	require.Len(t, all.Items, 3)
	assert.Equal(t, BillingReconciliationModeToken, all.Items[0].BillingMode)
	require.Len(t, view.ModelRanks, 3)
	for _, row := range view.ModelRanks {
		require.NotNil(t, row.FailureCalls)
		switch row.BillingMode {
		case BillingReconciliationModeToken:
			assert.Zero(t, *row.FailureCalls)
			require.NotNil(t, row.TokenUsage)
			assert.EqualValues(t, 40, row.TokenUsage.InputTokens)
			assert.EqualValues(t, 60, row.TokenUsage.OutputTokens)
			assert.EqualValues(t, 10, row.TokenUsage.CacheReadTokens)
			assert.EqualValues(t, 0, row.TokenUsage.CacheWriteTokens)
			assert.EqualValues(t, 25, row.TokenUsage.TokenDetails["image_output"])
		case customerUsagePerImage:
			assert.Zero(t, *row.FailureCalls)
			assert.Nil(t, row.TokenUsage)
			assert.Zero(t, row.MissingTokens)
		case BillingReconciliationModeUnknown:
			assert.EqualValues(t, 1, *row.FailureCalls)
			assert.Nil(t, row.TokenUsage)
			assert.EqualValues(t, 1, row.MissingTokens)
		default:
			t.Fatalf("unexpected billing mode %q", row.BillingMode)
		}
	}
	assert.EqualValues(t, 5, view.Total.TotalCalls)
	assert.EqualValues(t, 2100, view.Total.NetQuota)
	assert.EqualValues(t, 100, view.Total.InputTokens+view.Total.OutputTokens)
	assert.EqualValues(t, 3, view.Total.ImageCount)
	assert.EqualValues(t, 1, view.Total.ImageCountMissingRows)
	require.Len(t, view.Keys, 1)
	require.Len(t, view.Keys[0].Models, 1)
	assert.EqualValues(t, view.Total.NetQuota, view.Keys[0].Models[0].Total.NetQuota)
	for _, row := range all.Items {
		assert.Equal(t, "mixed", row.Name)
		require.NotNil(t, row.Amount)
		require.NotNil(t, row.FailureCalls)
		switch row.BillingMode {
		case BillingReconciliationModeToken:
			assert.Zero(t, *row.FailureCalls)
			assert.EqualValues(t, 1, row.Calls)
			assert.EqualValues(t, 100, row.Tokens)
			assert.EqualValues(t, 600, *row.Amount)
			assert.Zero(t, row.NonToken.ImageCountMissingRows)
		case customerUsagePerImage:
			assert.Zero(t, *row.FailureCalls)
			assert.EqualValues(t, 3, row.Calls)
			assert.Zero(t, row.Tokens)
			assert.Zero(t, row.MissingTokens)
			assert.EqualValues(t, 1500, *row.Amount)
			assert.True(t, row.NonToken.ImageCountKnown)
			assert.EqualValues(t, 3, row.NonToken.ImageCount)
			assert.EqualValues(t, 1, row.NonToken.ImageCountMissingRows)
		case BillingReconciliationModeUnknown:
			assert.EqualValues(t, 2, row.Calls)
			assert.EqualValues(t, 2, *row.FailureCalls)
			assert.Zero(t, *row.Amount)
		default:
			t.Fatalf("unexpected billing mode %q", row.BillingMode)
		}
	}
	// Simulate a selected day published before the rank stored failure counts.
	require.Len(t, meta.References, 2)
	require.NoError(t, db.Model(&usageReportRank{}).
		Where("generation = ? AND kind = ?", meta.References[0].Generation, "model").
		Update("failure_calls", nil).Error)
	mixed, err := GetPublishedUsageRanks(frozen, period, query)
	require.NoError(t, err)
	assert.Equal(t, all, mixed)
}

func TestCustomerUsageFixedTierMeterUsesSettledBranch(t *testing.T) {
	const expression = `param("quality") == "fixed" ? tier("image", fixed(0.04)) * image_count : tier("flat", fixed(0.1))`
	assert.Equal(t, customerUsagePerImage, customerUsageFixedTierMeter(expression, "image"))
	assert.Equal(t, BillingReconciliationModePerCall, customerUsageFixedTierMeter(expression, "flat"))
	assert.Equal(t, BillingReconciliationModeUnknown, customerUsageFixedTierMeter(expression, "missing"))
	const ambiguous = `param("quality") == "fixed" ? tier("same", fixed(0.04)) * image_count : tier("same", fixed(0.1))`
	assert.Equal(t, BillingReconciliationModeUnknown, customerUsageFixedTierMeter(ambiguous, "same"))
}

func TestPublishedCustomerRankKeepsLegacyModelTotalsUnclassified(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.AutoMigrate(&usageReportDay{}, &usageReportDayRow{}, &SystemTaskLock{}, &SystemTask{}, &dailyReportStage{}, &dailyReportPayloadPart{}, &dailyReportRetry{}))
	ctx := context.Background()
	now := common.GetTimestamp()
	period, err := ResolveUsageAnalyticsPeriod("week", "2026-09-14", now)
	require.NoError(t, err)
	require.NoError(t, db.Create(&SystemTaskLock{Type: UsageReportTaskType, TaskID: "report", LockedBy: "runner", LockedUntil: now + 60}).Error)
	logs := []Log{
		{UserId: 7, ModelName: "mixed", Type: LogTypeConsume, CreatedAt: period.Days[0].Start + 60, Quota: 400, PromptTokens: 40, RequestId: "old"},
		{UserId: 7, ModelName: "mixed", Type: LogTypeConsume, CreatedAt: period.Days[1].Start + 60, Quota: 500, PromptTokens: 50, RequestId: "new", Other: `{"billing_unit":"token"}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	for _, date := range []string{"2026-09-14", "2026-09-15"} {
		require.NoError(t, BuildUsageReportDay(ctx, date, "report", "runner", nil))
	}
	var head usageReportDay
	require.NoError(t, db.First(&head, "start = ?", period.Days[0].Start).Error)
	require.NoError(t, db.Model(&usageReportRank{}).Where("generation = ? AND kind = ?", head.Generation, "model").Updates(map[string]any{
		"billing_mode": nil, "tokens": 40, "non_token_payload": `{"image_count_missing_rows":1}`,
	}).Error)
	rows, err := GetPublishedUsageRanks(ctx, period, UsageReportRankQuery{Kind: "model", Metric: "tokens", Page: 1, Size: 10})
	require.NoError(t, err)
	require.Len(t, rows.Items, 2)
	assert.Equal(t, BillingReconciliationModeToken, rows.Items[0].BillingMode)
	assert.EqualValues(t, 50, rows.Items[0].Tokens)
	old := rows.Items[1]
	assert.Equal(t, BillingReconciliationModeUnknown, old.BillingMode)
	assert.Zero(t, old.Tokens)
	assert.Zero(t, old.NonToken.ImageCountMissingRows)
	assert.EqualValues(t, 1, old.Calls)
	require.NotNil(t, old.Amount)
	assert.EqualValues(t, 400, *old.Amount)
}
