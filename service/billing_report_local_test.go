package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportPublishesMultipleHistoricalDatesAcrossTicks(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TaskBillingDelivery{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	prepareDailyReportExportTest(t)
	ctx := context.Background()
	now := time.Now().In(model.UsageAnalyticsZone())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	first := today.AddDate(0, 0, -121).Unix()
	second := today.AddDate(0, 0, -120).Unix()
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 7, Type: model.LogTypeConsume, CreatedAt: first + 1, Quota: 30, ModelName: "first"},
		{UserId: 7, Type: model.LogTypeConsume, CreatedAt: second + 1, Quota: 40, ModelName: "second"},
	}).Error)

	for range 8 {
		require.NoError(t, runDailyReport(ctx, &model.SystemTask{TaskID: "export-fixture"}, "runner", func(ctx context.Context) error { return ctx.Err() }))
	}

	_, report, err := model.ResolveBillingReport(ctx, first, second+86399, "")
	require.NoError(t, err)
	assert.Equal(t, 2, report.PublishedDays)
}

func TestDailyReportUpgradesAllKnownHistoryWithExplicitMissingDates(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TaskBillingDelivery{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	prepareDailyReportExportTest(t)
	ctx := context.Background()
	now := time.Now().In(model.UsageAnalyticsZone())
	first := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -101)
	const dates = 98
	const runner = "runner"
	task := &model.SystemTask{TaskID: "export-fixture"}
	// The test calls the worker directly, without the scheduler's lease heartbeat.
	require.NoError(t, db.Model(&model.SystemTaskLock{}).Where("type = ? AND task_id = ?", model.UsageReportTaskType, task.TaskID).
		Update("locked_until", now.Add(10*time.Minute).Unix()).Error)
	for i := range dates {
		at := first.AddDate(0, 0, i)
		require.NoError(t, db.Create(&model.Log{
			UserId: 7, TokenId: 17, ChannelId: 11, ModelName: "fixture", Type: model.LogTypeConsume,
			CreatedAt: at.Unix() + 1, Quota: 300,
		}).Error)
		require.NoError(t, model.BuildUsageReportDay(ctx, at.Format(time.DateOnly), task.TaskID, runner, nil))
		require.NoError(t, model.BuildBillingReportDay(ctx, at.Unix(), task.TaskID, runner, nil, nil))
	}
	final := first.AddDate(0, 0, dates)
	require.NoError(t, db.Table("usage_report_days").Where("start >= ? AND start < ?", first.Unix(), final.Unix()).Update("meter_version", 0).Error)
	require.NoError(t, db.Table("billing_report_generations").Where("start >= ? AND start < ?", first.Unix(), final.Unix()).Update("original_replay_version", model.CustomerOriginalReplayVersion-1).Error)
	frozen := first.AddDate(0, 0, 25).Unix()
	missing := first.AddDate(0, 0, 50).Unix()
	require.NoError(t, db.Where("created_at IN ?", []int64{frozen + 1, missing + 1}).Delete(&model.Log{}).Error)
	for _, fixture := range []struct{ head, row string }{
		{"usage_report_days", "usage_report_day_rows"},
		{"billing_report_heads", "billing_report_lines"},
		{"billing_report_heads", "billing_report_customer_days"},
		{"billing_report_heads", "billing_report_upstream_days"},
	} {
		require.NoError(t, db.Table(fixture.row).Where("generation IN (?)", db.Table(fixture.head).Select("generation").Where("start = ?", missing)).Delete(nil).Error)
	}
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "history-fixture", Quota: 800}).Error)

	// Each tick processes at most one scope. A day can require separate
	// usage metering and original billing replay passes.
	for range 3 * dates {
		require.NoError(t, runDailyReport(ctx, task, runner, func(ctx context.Context) error { return ctx.Err() }))
	}

	for i := range dates {
		at := first.AddDate(0, 0, i)
		date := at.Format(time.DateOnly)
		start, end := at.Unix(), at.AddDate(0, 0, 1).Unix()-1
		period, err := model.ResolveUsageAnalyticsPeriod("day", date, now.Unix())
		require.NoError(t, err)
		view, usage, err := model.GetPublishedUsageCustomer(ctx, period, 7, "")
		require.NoError(t, err)
		bound, billing, err := model.ResolveBillingReport(ctx, start, end, "")
		require.NoError(t, err)
		assert.Empty(t, usage.RebuildingDays, date)
		assert.Empty(t, billing.RebuildingDays, date)
		if start == missing {
			assert.Equal(t, []string{date}, usage.SourceUnavailableDays)
			assert.Equal(t, []string{date}, billing.SourceUnavailableDays)
			assert.Empty(t, usage.References)
			assert.Zero(t, billing.PublishedDays)
			continue
		}
		assert.Equal(t, []string{date}, usage.PublishedDays)
		assert.Equal(t, 1, billing.PublishedDays)
		assert.EqualValues(t, 300, view.Total.NetQuota)
		statement, err := model.GetBillingCustomerStatement(bound, 7, start, end, "api_key", 17, "", "")
		require.NoError(t, err)
		assert.EqualValues(t, 300, statement.Summary.NetQuota)
		if start == frozen {
			assert.Equal(t, []string{date}, usage.IncompleteDays)
			assert.Equal(t, 1, billing.IncompleteDays)
		}
	}
	var customer model.User
	require.NoError(t, db.Select("quota").First(&customer, 7).Error)
	assert.Equal(t, 800, customer.Quota)
	var count int64
	require.NoError(t, db.Model(&model.Log{}).Count(&count).Error)
	assert.EqualValues(t, dates-2, count)
}
