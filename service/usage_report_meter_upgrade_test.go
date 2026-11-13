package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportWorkerAutomaticallyRebuildsOldUsageMeterVersions(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TaskBillingDelivery{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	prepareDailyReportExportTest(t)
	ctx := context.Background()
	now := time.Now().In(model.UsageAnalyticsZone())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	old := today.AddDate(0, 0, -120)
	task := &model.SystemTask{TaskID: "export-fixture"}
	const runner = "runner"
	require.NoError(t, db.Create(&model.Log{
		UserId: 7, ModelName: "image-model", Type: model.LogTypeConsume, RequestId: "historic",
		CreatedAt: old.Unix() + 1, Quota: 300, PromptTokens: 25, CompletionTokens: 100,
		Other: `{"billing_unit":"token","request_path":"/v1/images/generations"}`,
	}).Error)
	for _, date := range []time.Time{old, today.AddDate(0, 0, -1), today} {
		require.NoError(t, model.BuildUsageReportDay(ctx, date.Format(time.DateOnly), task.TaskID, runner, nil))
		require.NoError(t, model.BuildBillingReportDay(ctx, date.Unix(), task.TaskID, runner, nil, nil))
	}
	period, err := model.ResolveUsageAnalyticsPeriod("day", old.Format(time.DateOnly), now.Unix())
	require.NoError(t, err)
	refs, err := model.FreezeUsageReportReferences(ctx, period, nil)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	var previousBilling string
	require.NoError(t, db.Table("billing_report_heads").Select("generation").Where("start = ?", old.Unix()).Scan(&previousBilling).Error)
	require.NotEmpty(t, previousBilling)
	// A database from before the migration has a NULL version and can also
	// contain a candidate staged by the previous process.
	require.NoError(t, db.Table("usage_report_days").Where("start = ?", old.Unix()).Update("meter_version", nil).Error)
	require.NoError(t, db.Table("usage_report_day_rows").Where("generation = ? AND user_id = -1", refs[0].Generation).
		Update("payload", fmt.Sprintf(`{"Start":%d,"Generation":%q}`, old.Unix(), refs[0].Generation)).Error)
	require.NoError(t, model.BuildUsageReportDay(model.StageDailyReport(ctx), old.Format(time.DateOnly), task.TaskID, runner, nil))
	var staleCandidate string
	require.NoError(t, db.Table("daily_report_stages").Select("generation").Where("start = ? AND kind = ?", old.Unix(), model.DailyReportUsage).Scan(&staleCandidate).Error)
	require.NotEmpty(t, staleCandidate)
	require.NoError(t, db.Table("daily_report_stages").Where("start = ? AND kind = ?", old.Unix(), model.DailyReportUsage).Update("usage_meter_version", nil).Error)

	customer, rebuilding, err := model.GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.Equal(t, refs, rebuilding.References)
	assert.Equal(t, []string{period.Date}, rebuilding.RebuildingDays)
	assert.EqualValues(t, 1, customer.Total.TotalCalls)
	assert.EqualValues(t, 300, customer.Total.NetQuota)
	rankQuery := model.UsageReportRankQuery{Kind: "model", Metric: "tokens", Page: 1, Size: 10}
	before, err := model.GetPublishedUsageRanks(ctx, period, rankQuery)
	require.NoError(t, err)
	require.Len(t, before.Items, 1)
	assert.EqualValues(t, 1, before.Items[0].Calls)
	assert.Equal(t, rebuilding.RebuildingDays, before.RebuildingDays)
	_, _, err = model.GetPublishedUsageCustomer(model.WithUsageReportReferences(ctx, refs), period, 7, "")
	require.NoError(t, err)
	_, err = model.FreezeUsageReportExport(ctx, period, refs)
	require.NoError(t, err)
	require.NoError(t, db.Table("daily_report_retries").Create(map[string]any{
		"start": old.Unix(), "kind": model.DailyReportUsage, "coverage_issue": true, "retry_at": now.Unix() + 60,
	}).Error)
	_, unavailable, err := model.GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.Empty(t, unavailable.RebuildingDays)
	assert.Equal(t, []string{period.Date}, unavailable.SourceUnavailableDays)
	require.NoError(t, db.Exec("DELETE FROM daily_report_retries WHERE start = ?", old.Unix()).Error)

	require.NoError(t, runDailyReport(ctx, task, runner, func(ctx context.Context) error { return ctx.Err() }, false))

	updated, err := model.FreezeUsageReportReferences(ctx, period, nil)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.NotEqual(t, refs[0].Generation, updated[0].Generation)
	assert.NotEqual(t, staleCandidate, updated[0].Generation)
	var billing string
	require.NoError(t, db.Table("billing_report_heads").Select("generation").Where("start = ?", old.Unix()).Scan(&billing).Error)
	assert.Equal(t, previousBilling, billing)
	ranks, err := model.GetPublishedUsageRanks(ctx, period, rankQuery)
	require.NoError(t, err)
	assert.Empty(t, ranks.RebuildingDays)
	require.Len(t, ranks.Items, 1)
	assert.Equal(t, "token", ranks.Items[0].BillingMode)
	assert.EqualValues(t, 125, ranks.Items[0].Tokens)
	assert.Zero(t, ranks.Items[0].NonToken.ImageCountMissingRows)

	_, _, _, usageOnly, err := model.NextDailyReportDay(ctx, today.Unix(), now.Unix())
	require.NoError(t, err)
	assert.False(t, usageOnly, "successful publication makes the upgrade ineligible on the next pass")
}
