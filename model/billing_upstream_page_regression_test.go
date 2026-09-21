package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpstreamWholePeriodInitializationIncludesUnvisitedChannels(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1, Username: "admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.FixedZone("Shanghai", 8*3600)).Unix()
	previous := previousBillingPeriodStart(start)
	url := "https://whole.example"
	// Cross both UI pages and the 100-channel initialization batch boundary.
	for id := 1; id <= 101; id++ {
		require.NoError(t, db.Create(&Channel{Id: id, Name: fmt.Sprintf("%03d", id), BaseURL: &url}).Error)
		require.NoError(t, db.Create(&ProviderChannelBillingDiscount{PeriodStart: previous, ChannelId: id, Discount: decimal.RequireFromString("0.8"), Version: 1}).Error)
		require.NoError(t, db.Create(&Log{ChannelId: id, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "m", Quota: 100, Other: `{"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":100}}`, TokenName: "模型测试", Content: "模型测试"}).Error)
	}
	counts, err := InitializeUpstreamPeriodDiscounts(context.Background(), start, start+100, 1)
	require.NoError(t, err)
	assert.Equal(t, 101, counts["created"])
	result, err := GetUpstreamSummaryPage(context.Background(), start, start+100, start, UpstreamSummaryPageFilter{Level: "groups", Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	require.NotNil(t, result.Groups[0].ReferenceAmount)
	assert.EqualValues(t, 8080, *result.Groups[0].ReferenceAmount)
	// Manual changes remain authoritative across repeated whole-period calls.
	record := ProviderChannelBillingDiscount{PeriodStart: start, ChannelId: 101, Discount: decimal.RequireFromString("0.5"), Reason: "manual"}
	require.NoError(t, SaveProviderChannelBillingDiscount(&record, 1, 1))
	counts, err = InitializeUpstreamPeriodDiscounts(context.Background(), start, start+100, 1)
	require.NoError(t, err)
	assert.Equal(t, 101, counts["exists"])
	result, err = GetUpstreamSummaryPage(context.Background(), start, start+100, start, UpstreamSummaryPageFilter{Level: "groups", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 8050, *result.Groups[0].ReferenceAmount)
}

func TestUpstreamOptionsAndChildPagesAvoidRepeatedBillingReads(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	url := "https://cached.example"
	require.NoError(t, db.Create(&Channel{Id: 1, Name: "original", BaseURL: &url}).Error)
	require.NoError(t, db.Create(&Log{ChannelId: 1, CreatedAt: 1100, Type: LogTypeConsume, ModelName: "m", Quota: 100, Other: `{"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":100}}`, TokenName: "模型测试", Content: "模型测试"}).Error)
	reads := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("count_billing_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" && strings.Contains(tx.Statement.SQL.String(), "completion_tokens") {
			reads++
		}
	}))
	t.Cleanup(func() { db.Callback().Query().Remove("count_billing_reads") })
	filter := UpstreamSummaryPageFilter{Level: "options", Search: "cached", Page: 1, PageSize: 10}
	options, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, options.Groups, 1)
	assert.Zero(t, reads, "option lookup must never load metering facts")
	filter.Level = "groups"
	_, err = GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	initial := reads
	assert.Positive(t, initial)
	filter.Level, filter.URLKey = "channels", url
	_, err = GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	filter.Level, filter.ChannelID = "models", 1
	result, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	assert.Equal(t, initial, reads, "child pages reuse the same metering projection")
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", 1).Update("name", "renamed").Error)
	filter.Level, filter.ChannelID = "channels", 0
	changed, err := GetUpstreamSummaryPage(context.Background(), 1000, 1500, 1000, filter)
	require.NoError(t, err)
	require.Len(t, changed.Channels, 1)
	assert.Equal(t, "renamed", changed.Channels[0].ChannelName)
	assert.Greater(t, reads, initial, "metadata changes invalidate cached accounting projections")
}

func TestUpstreamPeriodInitializationRejectsUnauthorizedAndCancelledCalls(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1, Username: "user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&Log{ChannelId: 1, CreatedAt: 1100, Type: LogTypeConsume}).Error)
	_, err := InitializeUpstreamPeriodDiscounts(context.Background(), 1000, 1500, 1)
	require.ErrorIs(t, err, ErrCustomerExportNotFound)
	var count int64
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Count(&count).Error)
	assert.Zero(t, count)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = InitializeUpstreamPeriodDiscounts(ctx, 1000, 1500, 1)
	require.ErrorIs(t, err, context.Canceled)
}
