package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAnalyticsScopedUpstreamExportMatchesPublishedTotals(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ProviderChannelBillingDiscount{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 42).Update("role", 100).Error)
	const originalURL, nextURL = "https://shared.example.test", "https://moved.example.test"
	baseURL := originalURL
	require.NoError(t, model.DB.Create(&[]model.Channel{
		{Id: 971, Name: "Shared name", BaseURL: &baseURL},
		{Id: 972, Name: "Shared name", BaseURL: &baseURL},
	}).Error)
	period, err := model.ResolveUsageAnalyticsPeriod("week", "2026-09-14", 1789920000)
	require.NoError(t, err)
	const evidence = `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: 971, CreatedAt: period.Days[0].Start + 1, Type: model.LogTypeConsume, Quota: 100, ModelName: "priced", Other: evidence},
		{UserId: 42, ChannelId: 971, CreatedAt: period.Days[0].Start + 2, Type: model.LogTypeConsume, Quota: 40, ModelName: "unpriced"},
		{UserId: 42, ChannelId: 972, CreatedAt: period.Days[0].Start + 3, Type: model.LogTypeConsume, Quota: 60, ModelName: "other channel", Other: evidence},
		{UserId: 42, ChannelId: 971, CreatedAt: period.Days[1].Start + 1, Type: model.LogTypeConsume, Quota: 200, ModelName: "new address", Other: evidence},
	}).Error)
	ctx := context.Background()
	publishUsageExportTestDays(t, period.Days[0].Date)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 971).Update("base_url", nextURL).Error)
	require.NoError(t, model.BuildUsageReportDay(ctx, period.Days[1].Date, "export-fixture", "runner", nil))

	byURL, meta, err := model.GetPublishedUsageUpstream(ctx, period, model.UsageUpstreamScope{URLKey: originalURL})
	require.NoError(t, err)
	require.Len(t, byURL.Channels, 2, "same URL and name still represent separate channels")
	assert.EqualValues(t, 3, byURL.Total.TotalCalls)
	channel := 971
	frozen := model.WithUsageReportReferences(ctx, meta.References)
	detail, _, err := model.GetPublishedUsageUpstream(frozen, period, model.UsageUpstreamScope{URLKey: originalURL, ChannelID: &channel})
	require.NoError(t, err)
	assert.Equal(t, byURL.Channels[0].Total, detail.Total)
	assert.EqualValues(t, 2, detail.Total.TotalCalls)
	assert.EqualValues(t, 2, detail.DayTotals[0].TotalCalls)
	assert.Zero(t, detail.DayTotals[1].TotalCalls)
	assert.Nil(t, detail.Total.ReferenceAmount)
	assert.True(t, detail.Total.ReferencePending)
	require.NotNil(t, detail.Total.KnownReferenceAmount)
	assert.EqualValues(t, 100, *detail.Total.KnownReferenceAmount)
	rank, err := model.GetPublishedUsageRanks(frozen, period, model.UsageReportRankQuery{
		Kind: "channel", Metric: "amount", URLKey: originalURL, Page: 1, Size: 10,
	})
	require.NoError(t, err)
	require.Len(t, rank.Items, 2)
	assert.Equal(t, channel, rank.Items[0].ChannelID)
	assert.Equal(t, detail.Total.KnownReferenceAmount, rank.Items[0].Amount)
	assert.Positive(t, rank.Items[0].PendingAmount)
	acrossURLs, _, err := model.GetPublishedUsageUpstream(frozen, period, model.UsageUpstreamScope{ChannelID: &channel})
	require.NoError(t, err)
	require.Len(t, acrossURLs.UrlGroups, 2, "channel history retains both frozen addresses")
	assert.EqualValues(t, 3, acrossURLs.Total.TotalCalls)

	job, err := SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{
		ReportDays: meta.References, Period: period.Period, Date: period.Date, View: "upstream",
		Upstream: model.UsageUpstreamScope{URLKey: originalURL, ChannelID: &channel},
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, meta.References, filters.UsageReportDays)
	assert.Equal(t, originalURL, filters.UsageURLKey)
	require.NotNil(t, filters.ChannelId)
	assert.Equal(t, channel, *filters.ChannelId)
	rows, _, err := usageAnalyticsExportRows(ctx, job, "upstream", period, filters,
		customerExportScopeColumns{QuotaPerUnit: 100, CurrencyRate: 1, Currency: "USD"})
	require.NoError(t, err)
	require.Len(t, rows, 17, "two selected model rows with seven daily buckets each plus the scoped grand total")
	csv := rows[len(rows)-1]
	assert.Equal(t, "grand_total", csv[0])
	assert.Equal(t, originalURL, csv[7])
	assert.Equal(t, "971", csv[8])
	assert.Equal(t, "2", csv[13])
	assert.Empty(t, csv[28], "the complete reference amount stays blank")
	assert.Equal(t, "1.00000000", csv[len(csv)-1], "the known subtotal occupies its own column")
}

func TestUsageAnalyticsUpstreamMonthlyExportUsesShanghaiCalendar(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ProviderChannelBillingDiscount{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 42).Update("role", 100).Error)
	const url = "https://monthly.example.test"
	baseURL := url
	const channelID = 975
	require.NoError(t, model.DB.Create(&model.Channel{Id: channelID, Name: "Monthly upstream", BaseURL: &baseURL}).Error)
	now := time.Date(2026, time.October, 2, 0, 0, 0, 0, time.UTC).Unix()
	period, err := model.ResolveUsageAnalyticsPeriod("month", "2026-09-25", now)
	require.NoError(t, err)
	const evidence = `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{UserId: 42, ChannelId: channelID, CreatedAt: period.StartTimestamp - 1, Type: model.LogTypeConsume, Quota: 900, ModelName: "chat", Other: evidence},
		{UserId: 42, ChannelId: channelID, CreatedAt: period.StartTimestamp, Type: model.LogTypeConsume, Quota: 25, ModelName: "chat", Other: evidence},
		{UserId: 42, ChannelId: channelID, CreatedAt: period.EndTimestamp - 1, Type: model.LogTypeConsume, Quota: 75, ModelName: "chat", Other: evidence},
		{UserId: 42, ChannelId: channelID, CreatedAt: period.EndTimestamp, Type: model.LogTypeConsume, Quota: 900, ModelName: "chat", Other: evidence},
	}).Error)
	publishUsageExportTestDays(t, "2026-08-31", "2026-09-01", "2026-09-30", "2026-10-01")
	ctx := context.Background()
	channel := channelID
	scope := model.UsageUpstreamScope{URLKey: url, ChannelID: &channel}
	view, meta, err := model.GetPublishedUsageUpstream(ctx, period, scope)
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-09-01", "2026-09-30"}, meta.PublishedDays)
	require.Len(t, view.DayTotals, 30)
	assert.EqualValues(t, 1, view.DayTotals[0].TotalCalls)
	assert.EqualValues(t, 1, view.DayTotals[29].TotalCalls)
	assert.EqualValues(t, 2, view.Total.TotalCalls)
	require.NotNil(t, view.Total.ReferenceAmount)
	assert.EqualValues(t, 100, *view.Total.ReferenceAmount)

	job, err := SubmitUsageAnalyticsExport(42, UsageAnalyticsExportRequest{
		ReportDays: meta.References, Period: "month", Date: "2026-09-25", View: "upstream", Upstream: scope,
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, meta.References, filters.UsageReportDays)
	csvPeriod, err := usageAnalyticsPeriodFromFilters(filters)
	require.NoError(t, err)
	assert.Equal(t, "month", csvPeriod.Period)
	assert.Equal(t, "2026-09-01", csvPeriod.Date)
	rows, _, err := usageAnalyticsExportRows(ctx, job, "upstream", csvPeriod, filters,
		customerExportScopeColumns{QuotaPerUnit: 100, CurrencyRate: 1, Currency: "USD"})
	require.NoError(t, err)
	require.Len(t, rows, 32)
	assert.Equal(t, []string{"daily", "2026-09-01"}, rows[0][:2])
	assert.Equal(t, []string{"not_published", "2026-09-02"}, rows[1][:2])
	assert.Equal(t, []string{"daily", "2026-09-30"}, rows[29][:2])
	assert.Equal(t, "grand_total", rows[31][0])
	assert.Equal(t, "2", rows[31][13])
	assert.Equal(t, "1.00000000", rows[31][28])
}
