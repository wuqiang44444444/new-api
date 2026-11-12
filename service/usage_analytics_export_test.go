package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CSV schema is positional: header width must stay in sync with the row
// builders. Layout = 2 (row type, date) + 11 group columns + 28 metric
// columns (including the test-priced rows column) + 8 scope columns. This
// guard exists because a previous version of the row builder emitted two
// fewer cells than the header promised.
func TestUsageAnalyticsExportSchemaAlignment(t *testing.T) {
	metrics := usageAnalyticsMetricsRow(model.UsageAnalyticsMetrics{}, "0.8", customerExportScopeColumns{}, usageAnalyticsExportFieldVersion)
	assert.Len(t, metrics, 28)
	for _, language := range []string{"en", "zh"} {
		header := usageAnalyticsExportHeader(language)
		assert.Len(t, header, 50, "header layout: 2 + 11 group + 28 metrics + 8 scope + customer discounts")
	}
}

func TestUsageAnalyticsExportDistinguishesRecordedZeroAndMissingMeters(t *testing.T) {
	seconds := decimal.RequireFromString("29.25")
	recorded := usageAnalyticsMetricsRow(model.UsageAnalyticsMetrics{
		ImageCountKnown: true, ImageCountMissingRows: 1,
		Seconds: &seconds, SecondsMissingRows: 2,
	}, "", customerExportScopeColumns{}, usageAnalyticsExportFieldVersion)
	assert.Equal(t, "0", recorded[9])
	assert.Equal(t, "29.25", recorded[10])
	assert.Equal(t, "images_unrecorded:1;seconds_unrecorded:2", recorded[23])

	missing := usageAnalyticsMetricsRow(model.UsageAnalyticsMetrics{
		ImageCountMissingRows: 1, SecondsMissingRows: 2,
	}, "", customerExportScopeColumns{}, usageAnalyticsExportFieldVersion)
	assert.Empty(t, missing[9])
	assert.Empty(t, missing[10])
	assert.Equal(t, "images_unrecorded:1;seconds_unrecorded:2", missing[23])

	notApplicable := usageAnalyticsMetricsRow(model.UsageAnalyticsMetrics{}, "", customerExportScopeColumns{}, usageAnalyticsExportFieldVersion)
	assert.Empty(t, notApplicable[9])
	assert.Empty(t, notApplicable[10])
	assert.Empty(t, notApplicable[23])

	legacy := usageAnalyticsMetricsRow(model.UsageAnalyticsMetrics{}, "", customerExportScopeColumns{}, legacyUsageAnalyticsExportFieldVersion)
	assert.Equal(t, "0", legacy[9])
}

func TestUsageAnalyticsPeriodFromFiltersRejectsArbitraryRanges(t *testing.T) {
	base := int64(1789488000) // 2026-09-16 00:00 +08:00, Wednesday
	for _, span := range []int64{2 * 86400, 6 * 86400, 8 * 86400, 31 * 86400} {
		_, err := usageAnalyticsPeriodFromFilters(model.CustomerExportFilters{
			StartTimestamp: base,
			EndTimestamp:   base + span,
		})
		assert.Error(t, err, "span %d must be rejected", span)
	}
	// A seven-day range that does not start on Monday is not a natural week.
	_, err := usageAnalyticsPeriodFromFilters(model.CustomerExportFilters{
		StartTimestamp: base,
		EndTimestamp:   base + 7*86400,
	})
	assert.Error(t, err)
}

func TestUsageAnalyticsExportRowsPreserveDailySchemaAndScope(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Token{}, &model.BatchJob{}, &model.TaskBillingDelivery{}, &model.ProviderChannelBillingDiscount{}))
	period, err := model.ResolveUsageAnalyticsPeriod("week", "2026-09-16", 1789920000)
	require.NoError(t, err)
	period.Days[6].Future = true
	require.NoError(t, model.DB.Create(&model.Token{Id: 991, UserId: 42, Name: "=unsafe"}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 42, TokenId: 991, ChannelId: 991, CreatedAt: period.StartTimestamp + 1, Type: model.LogTypeConsume, Quota: 100, ModelName: "chat", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"audio_input":0,"audio_output":9}`}).Error)
	filters := model.CustomerExportFilters{StartTimestamp: period.StartTimestamp, EndTimestamp: period.EndTimestamp, Timezone: model.UsageAnalyticsTimezone, Language: "en", QuotaPerUnit: 100, CurrencyRate: 1}
	scope := customerExportScopeColumns{QuotaPerUnit: 100, CurrencyRate: 1, Currency: "USD"}
	filters.UsageReportDays = freezeUsageExportTestDays(t, period, 1)
	for _, view := range []string{"customer", "upstream", "customers"} {
		t.Run(view, func(t *testing.T) {
			rows, _, err := usageAnalyticsExportRows(context.Background(), &model.CustomerExportJob{JobID: "usage-file", TargetUserId: 42}, view, period, filters, scope)
			require.NoError(t, err)
			require.Len(t, rows, 9, "seven days, leaf total and grand total")
			header := usageAnalyticsExportHeader("en")
			for _, row := range rows {
				require.Len(t, row, len(header))
			}
			cells := map[string]string{}
			for i, h := range header {
				cells[h] = rows[0][i]
			}
			assert.Equal(t, "1", cells["Calls"])
			assert.Equal(t, "100", cells["Net quota"])
			assert.Equal(t, "0", cells["Audio input tokens"])
			assert.Equal(t, "9", cells["Audio output tokens"])
			if view == "customer" {
				assert.Equal(t, "'=unsafe", cells["API Key"])
				assert.Empty(t, cells["Channel ID"])
				discounts := rows[7][len(header)-1]
				assert.Contains(t, discounts, "No contract discount applied")
				assert.Contains(t, discounts, "final factor: 1")
				assert.Empty(t, cells["Customer period discount combinations"], "period combinations do not masquerade as daily facts")
			}
			if view == "upstream" {
				assert.Equal(t, "991", cells["Channel ID"])
				assert.NotEmpty(t, cells["URL grouping"])
				assert.Empty(t, cells["Customer ID"])
			}
			assert.Equal(t, "grand_total", rows[8][0])
			assert.Equal(t, "not_started", rows[6][0])
			assert.Empty(t, rows[6][13])
		})
	}
}

func TestUsageAnalyticsRegenerationPreservesSnapshotAndAuthorization(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 42).Update("role", 100).Error)
	publishUsageExportTestDays(t, "2026-09-16")
	job, err := SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{Period: "day", Date: "2026-09-16", View: "upstream", Language: "en"})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	require.Len(t, filters.UsageReportDays, 1)
	require.NoError(t, model.DB.Model(&model.CustomerExportJob{}).Where("job_id = ?", job.JobID).Update("status", model.CustomerExportJobStatusFailed).Error)
	regenerated, err := ResubmitUsageAnalyticsExport(42, job.JobID, false)
	require.NoError(t, err)
	restored, err := regenerated.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, filters, restored)
	_, err = ResubmitUsageAnalyticsExport(43, job.JobID, false)
	require.Error(t, err)
	_, err = ResubmitUsageAnalyticsExport(42, job.JobID, true)
	require.Error(t, err)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 42).Update("role", 1).Error)
	_, err = ResubmitUsageAnalyticsExport(42, job.JobID, false)
	require.Error(t, err)
	_, err = SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{Period: "day", Date: "2026-09-16", View: "customers"})
	require.Error(t, err)
}

func TestUsageExportIncludesDeliveredVideoCustomerRefund(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Token{}, &model.BatchJob{}, &model.TaskBillingDelivery{}, &model.ProviderChannelBillingDiscount{}))
	require.NoError(t, model.DB.Create(&model.Token{Id: 991, UserId: 42, Name: "refund-export-key"}).Error)
	period, err := model.ResolveUsageAnalyticsPeriod("day", "2026-09-16", 1789920000)
	require.NoError(t, err)
	task := model.Task{TaskID: "export-refunded-video", UserId: 42, ChannelId: 991, Platform: "video", Status: model.TaskStatusSuccess,
		FinishTime: period.StartTimestamp + 10, BillingState: model.TaskBillingStateAwaitingUsage,
		Properties:  model.Properties{OriginModelName: "video"},
		PrivateData: model.TaskPrivateData{TokenId: 991, AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStateAwaitingUsage}},
		VideoRefund: model.VideoRefund{VideoRefundState: "refunded", VideoRefundCompletedAt: period.EndTimestamp + 10}}
	require.NoError(t, model.DB.Create(&task).Error)
	for _, event := range []model.TaskBillingDelivery{
		{TaskRowID: task.ID, Event: "create", AfterQuota: 100, CreatedAt: period.StartTimestamp - 10},
		{TaskRowID: task.ID, Event: "customer_refund", BeforeQuota: 100, CreatedAt: period.EndTimestamp + 10},
	} {
		require.NoError(t, model.DB.Create(&event).Error)
		require.NoError(t, model.DeliverTaskBillingLog(context.Background(), event.ID, BuildTaskBillingDeliveryLog))
	}
	filters := model.CustomerExportFilters{UsageReportDays: freezeUsageExportTestDays(t, period, 1)}
	for _, view := range []string{"customer", "customers", "upstream"} {
		t.Run(view, func(t *testing.T) {
			rows, _, err := usageAnalyticsExportRows(context.Background(), &model.CustomerExportJob{TargetUserId: 42}, view, period, filters, customerExportScopeColumns{QuotaPerUnit: 100, CurrencyRate: 1, Currency: "USD"})
			require.NoError(t, err)
			require.NotEmpty(t, rows)
			cells := map[string]string{}
			for i, h := range usageAnalyticsExportHeader("en") {
				cells[h] = rows[0][i]
			}
			assert.Equal(t, "1", cells["Calls"])
			assert.Equal(t, "100", cells["Gross quota"])
			assert.Equal(t, "100", cells["Refund quota"])
			assert.Equal(t, "0", cells["Net quota"])
		})
	}
}
