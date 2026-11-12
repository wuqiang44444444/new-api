package model

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeletedChannelDiscountConsistentAcrossReadPaths(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
	require.NoError(t, err)
	month := usageMonthStart(period.StartTimestamp)
	previous := previousBillingPeriodStart(month)
	require.NoError(t, db.Create(&[]ProviderChannelBillingDiscount{
		{PeriodStart: previous, ChannelId: 21, Discount: decimal.RequireFromString("0.8"), Version: 1},
		{PeriodStart: previous, ChannelId: 22, Discount: decimal.RequireFromString("0.7"), Version: 1},
		{PeriodStart: month, ChannelId: 22, Discount: decimal.NewFromInt(1), PendingReason: "conflict", Version: 1},
	}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: period.StartTimestamp + 10, Type: LogTypeConsume, ChannelId: 21, ModelName: "video", Quota: 1000,
		Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"upstream_model_name":"video","cache_tokens":0,"cache_write_tokens":0}`}).Error)
	summary, err := GetProviderBillingURLSummary(month, period.EndTimestamp, month, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	require.NotNil(t, summary.Groups[0].ReferenceAmount)
	assert.EqualValues(t, 800, *summary.Groups[0].ReferenceAmount)
	view, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	require.NotNil(t, view.Total.ReferenceAmount)
	assert.EqualValues(t, 800, *view.Total.ReferenceAmount)
	read, err := GetProviderChannelBillingDiscounts(month, []int{21, 22, 23})
	require.NoError(t, err)
	assert.Equal(t, "0.8", read[21].Discount.String())
	assert.NotContains(t, read, 22)
	assert.Equal(t, "1", read[23].Discount.String())

	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: period.StartTimestamp + 11, Type: LogTypeConsume, ChannelId: 22, ModelName: "image", PromptTokens: 100, Quota: 1000,
		Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"upstream_model_name":"image","request_path":"/v1/images/edits","cache_tokens":0,"cache_write_tokens_reported":false}`}).Error)
	page, err := GetUpstreamSummaryPage(context.Background(), month, period.EndTimestamp, month, UpstreamSummaryPageFilter{Level: "url_channels", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.NotNil(t, page.Totals)
	require.NotNil(t, page.Totals.OriginalAmount)
	assert.EqualValues(t, 2000, *page.Totals.OriginalAmount)
	assert.Nil(t, page.Totals.ReferenceAmount)
	assert.Equal(t, 1, page.Totals.DiscountPendingChannels)
	require.NotNil(t, page.DataQuality.EvidenceCoverage)
	assert.Zero(t, page.DataQuality.EvidenceCoverage.AmountGapRows)
	assert.EqualValues(t, 1, page.DataQuality.EvidenceCoverage.UsageGapRows)

	for _, status := range []string{"complete", "partial"} {
		models, err := GetUpstreamSummaryPage(context.Background(), month, period.EndTimestamp, month, UpstreamSummaryPageFilter{Level: "url_models", ChannelID: 22, DataStatus: status, Page: 1, PageSize: 10})
		require.NoError(t, err)
		if status == "complete" {
			require.Len(t, models.ModelRows, 1)
			assert.Nil(t, models.ModelRows[0].ReferenceAmount)
		} else {
			assert.Empty(t, models.ModelRows)
		}
	}
	var count int64
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Where("period_start = ?", month).Count(&count).Error)
	assert.EqualValues(t, 1, count, "reads must not initialize deleted channels")
}
