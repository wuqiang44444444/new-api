package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func usageTestCells(t *testing.T, row []string, view, grain string) map[string]string {
	t.Helper()
	columns := usageCSVColumns(view, grain)
	require.Len(t, row, len(columns))
	values := make(map[string]string, len(columns))
	for i, column := range columns {
		values[column.key] = row[i]
	}
	return values
}

func TestUsageAnalyticsExportDistinguishesRecordedZeroAndMissingMeters(t *testing.T) {
	seconds := decimal.RequireFromString("29.25")
	zero, writes := int64(0), int64(32)
	recorded := usageCSVMetricValues(model.UsageAnalyticsMetrics{
		ImageCountKnown: true, ImageCountMissingRows: 1, Seconds: &seconds, SecondsMissingRows: 2,
		CacheWrites:       &model.BillingCacheWrites{FiveMinutes: &zero, OneHour: &writes},
		RowsMissingTokens: 1, // Input/output evidence alone does not determine cache tiers.
	}, customerExportScopeColumns{}, false)
	assert.Equal(t, "0", recorded["images"])
	assert.Equal(t, "29.25", recorded["seconds"])
	assert.Equal(t, "0", recorded["write_5m"])
	assert.Equal(t, "32", recorded["write_1h"])
	assert.Empty(t, recorded["input"])
	assert.Empty(t, recorded["write_12h"])
	assert.Contains(t, recorded["notes"], "2 records missing seconds")

	missing := usageCSVMetricValues(model.UsageAnalyticsMetrics{
		ImageCountMissingRows: 1, SecondsMissingRows: 2, CacheWriteTokens: 15,
	}, customerExportScopeColumns{}, false)
	assert.Empty(t, missing["images"])
	assert.Empty(t, missing["seconds"])
	assert.Empty(t, missing["write_5m"])
	assert.Contains(t, missing["notes"], "Unclassified cache write tokens: 15")

	notApplicable := usageCSVMetricValues(model.UsageAnalyticsMetrics{}, customerExportScopeColumns{}, false)
	assert.Empty(t, notApplicable["images"])
	assert.Empty(t, notApplicable["seconds"])
	assert.Empty(t, notApplicable["notes"])
}

func TestUsageCSVExplainsPricingOperandsWithoutReplacingCacheTiers(t *testing.T) {
	standard, twelveHours := int64(5), int64(12)
	row := usageCSVMetricValues(model.UsageAnalyticsMetrics{
		BillingTokensApplicable: true, CacheWriteTokens: 29, CacheWritePricingAdjustedRows: 1,
		CacheWrites: &model.BillingCacheWrites{Standard: &standard, TwelveHours: &twelveHours},
	}, customerExportScopeColumns{Language: "zh"}, true)
	assert.Equal(t, "5", row["write_standard"])
	assert.Equal(t, "12", row["write_12h"])
	assert.Contains(t, row["notes"], "已知缓存写入计价用量合计 29 Token")
	fixedPrice := usageCSVMetricValues(model.UsageAnalyticsMetrics{
		CacheWrites: &model.BillingCacheWrites{TwelveHours: &twelveHours},
	}, customerExportScopeColumns{Language: "zh"}, true)
	assert.Empty(t, fixedPrice["input"])
	assert.Equal(t, "12", fixedPrice["write_12h"])
}

func TestUsageExportFreezesIncompleteDatesAndDistinguishesUpstreamFallback(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	ctx := context.Background()
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Token{}, &model.BatchJob{}, &model.TaskBillingDelivery{}, &model.ProviderChannelBillingDiscount{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 42).Update("role", 100).Error)
	period, err := model.ResolveUsageAnalyticsPeriod("week", "2026-09-16", 1789920000)
	require.NoError(t, err)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{UserId: 42, TokenId: 991, ChannelId: 991, CreatedAt: period.StartTimestamp + 1, Type: model.LogTypeConsume, Quota: 100,
			ModelName: "same", PromptTokens: 50, Other: `{"upstream_model_name":"same","group_ratio":1,"model_ratio":1}`},
		{UserId: 42, TokenId: 991, ChannelId: 991, CreatedAt: period.StartTimestamp + 2, Type: model.LogTypeConsume, Quota: 100,
			ModelName: "same", PromptTokens: 50, Other: `{"is_model_mapped":true,"group_ratio":1,"model_ratio":1}`},
	}).Error)
	refs := freezeUsageExportTestDays(t, period, 1)
	require.Len(t, refs, 1)
	head := model.DB.Table("usage_report_days").Where("generation = ?", refs[0].Generation)
	require.NoError(t, head.Update("incomplete", true).Error)
	job, err := SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{
		Period: "week", Date: "2026-09-16", View: "upstream", Grain: "period", Language: "zh", ReportDays: refs,
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, []string{period.Days[0].Date}, filters.UsageIncompleteDates)
	require.NoError(t, head.Update("incomplete", false).Error)

	scope := customerExportScopeColumns{Language: "zh", QuotaPerUnit: 100, CurrencyRate: 1, Currency: "USD"}
	for _, grain := range []string{"period", ""} {
		filters.UsageExportGrain = grain
		rows, _, err := usageAnalyticsExportRows(ctx, job, "upstream", period, filters, scope)
		require.NoError(t, err)
		fallbackRows, meteredRows := 0, 0
		for _, row := range rows {
			cells := usageTestCells(t, row, "upstream", grain)
			if cells["calls"] != "1" {
				continue
			}
			meteredRows++
			assert.Equal(t, "same", cells["model"])
			assert.Contains(t, cells["notes"], "覆盖不完整日期 "+period.Days[0].Date)
			if strings.Contains(cells["notes"], "上游模型未记录") {
				fallbackRows++
			}
		}
		assert.Equal(t, 2, meteredRows)
		assert.Equal(t, 1, fallbackRows)
	}
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
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: 42, TokenId: 991, ChannelId: 991, CreatedAt: period.StartTimestamp + 1, Type: model.LogTypeConsume, Quota: 100,
		ModelName: "chat", PromptTokens: 50, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"usage_semantic":"openai","cache_creation_tokens":17,"cache_creation_tokens_12h":12,"cache_creation_ratio":1.2}`,
	}).Error)
	filters := model.CustomerExportFilters{StartTimestamp: period.StartTimestamp, EndTimestamp: period.EndTimestamp}
	filters.UsageReportDays = freezeUsageExportTestDays(t, period, 1)
	scope := customerExportScopeColumns{QuotaPerUnit: 100, CurrencyRate: 1, Currency: "USD"}
	for _, view := range []string{"customer", "upstream", "customers"} {
		t.Run(view, func(t *testing.T) {
			job := &model.CustomerExportJob{JobID: "usage-file", TargetUserId: 42}
			rows, _, err := usageAnalyticsExportRows(context.Background(), job, view, period, filters, scope)
			require.NoError(t, err)
			require.Len(t, rows, 6, "daily file keeps elapsed dates and excludes total rows")
			first := usageTestCells(t, rows[0], view, "")
			assert.Equal(t, "1", first["calls"])
			assert.Equal(t, "5", first["write_standard"])
			assert.Equal(t, "12", first["write_12h"])
			assert.Contains(t, usageTestCells(t, rows[1], view, "")["notes"], "Not published")
			if view == "customer" {
				assert.Equal(t, "'=unsafe", first["token"])
				assert.Equal(t, "1.00000000", first["net_amount"])
				assert.NotContains(t, first, "channel_id")
			}
			if view == "upstream" {
				assert.Equal(t, "991", first["channel_id"])
				assert.NotEmpty(t, first["url"])
				assert.NotContains(t, first, "customer")
			}

			periodFilters := filters
			periodFilters.UsageExportGrain = "period"
			totals, _, err := usageAnalyticsExportRows(context.Background(), job, view, period, periodFilters, scope)
			require.NoError(t, err)
			require.Len(t, totals, 1)
			total := usageTestCells(t, totals[0], view, "period")
			assert.Equal(t, first["write_12h"], total["write_12h"])
			assert.Equal(t, first["calls"], total["calls"])
			assert.Equal(t, "Period", csvLabels("en", scope.Currency, usageCSVColumns(view, "period"))[0])
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
			cells := usageTestCells(t, rows[0], view, "")
			assert.Equal(t, "1", cells["calls"])
			if view != "upstream" {
				assert.Equal(t, "1.00000000", cells["gross_amount"])
				assert.Equal(t, "1.00000000", cells["refund_amount"])
				assert.Equal(t, "0.00000000", cells["net_amount"])
			}
		})
	}
}
