package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func prepareDailyReportExportTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.MigrateDailyReports(model.DB))
	require.NoError(t, model.DB.AutoMigrate(&model.BatchJob{}, &model.ProviderChannelBillingDiscount{},
		&model.ProviderChannelBillingConfig{}, &model.ProviderURLGroupDisplayName{}))
	for _, table := range []string{"daily_report_export_references", "usage_report_days", "usage_report_day_rows",
		"billing_report_heads", "billing_report_generations", "billing_report_customer_days",
		"billing_report_upstream_days", "billing_report_lines"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	require.NoError(t, model.DB.Where("type = ?", model.UsageReportTaskType).Delete(&model.SystemTaskLock{}).Error)
	require.NoError(t, model.DB.Create(&model.SystemTaskLock{Type: model.UsageReportTaskType,
		TaskID: "export-fixture", LockedBy: "runner", LockedUntil: common.GetTimestamp() + 60}).Error)
}

func publishUsageExportTestDays(t *testing.T, dates ...string) {
	t.Helper()
	prepareDailyReportExportTest(t)
	for _, date := range dates {
		require.NoError(t, model.BuildUsageReportDay(context.Background(), date, "export-fixture", "runner", nil))
	}
}

func publishBillingExportTestDay(t *testing.T, days ...int64) {
	t.Helper()
	prepareDailyReportExportTest(t)
	for _, start := range days {
		require.NoError(t, model.BuildBillingReportDay(context.Background(), start, "export-fixture", "runner", nil,
			func(log *model.Log, row *model.CustomerExportRow) {
				attachCustomerExportBillingExplanation(log, row, common.QuotaPerUnit)
			}))
	}
}

func freezeUsageExportTestDays(t *testing.T, period model.UsageAnalyticsPeriod, count int) []model.UsageReportDayReference {
	t.Helper()
	var dates []string
	for i := range count {
		dates = append(dates, time.Unix(period.Days[i].Start, 0).In(model.UsageAnalyticsZone()).Format(time.DateOnly))
	}
	publishUsageExportTestDays(t, dates...)
	refs, err := model.FreezeUsageReportReferences(context.Background(), period, nil)
	require.NoError(t, err)
	return refs
}
