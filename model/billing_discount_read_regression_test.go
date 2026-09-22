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
	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: period.StartTimestamp + 10, Type: LogTypeConsume, ChannelId: 21, ModelName: "video", Quota: 1000, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`}).Error)
	summary, err := GetProviderBillingURLSummary(month, period.EndTimestamp, month, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	require.NotNil(t, summary.Groups[0].ReferenceAmount)
	assert.EqualValues(t, 800, *summary.Groups[0].ReferenceAmount)
	view, err := GetUsageUpstreamView(context.Background(), period)
	require.NoError(t, err)
	require.NotNil(t, view.Total.ReferenceAmount)
	assert.EqualValues(t, 800, *view.Total.ReferenceAmount)
	snapshot, err := freezeUpstreamExportChannels(context.Background(), month, []int{21, 22, 23}, false)
	require.NoError(t, err)
	assert.Equal(t, "0.8", snapshot[21].Discount.Value.String())
	assert.Nil(t, snapshot[22].Discount)
	assert.Equal(t, "1", snapshot[23].Discount.Value.String())
	read, err := GetProviderChannelBillingDiscounts(month, []int{21, 22, 23})
	require.NoError(t, err)
	assert.Equal(t, "0.8", read[21].Discount.String())
	assert.NotContains(t, read, 22)
	assert.Equal(t, "1", read[23].Discount.String())
	var count int64
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Where("period_start = ?", month).Count(&count).Error)
	assert.EqualValues(t, 1, count, "reads must not initialize deleted channels")
}
