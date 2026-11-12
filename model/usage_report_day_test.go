package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishedUsageReportKeepsOriginalExportGeneration(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.AutoMigrate(&usageReportDay{}, &usageReportDayRow{}, &SystemTaskLock{}, &SystemTask{}, &dailyReportStage{}, &dailyReportPayloadPart{}, &dailyReportRetry{}))
	ctx := context.Background()
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", common.GetTimestamp())
	require.NoError(t, err)
	require.NoError(t, db.Create(&SystemTaskLock{Type: UsageReportTaskType, TaskID: "report", LockedBy: "runner", LockedUntil: time.Now().Add(time.Minute).Unix()}).Error)
	logs := []Log{
		{UserId: 7, CreatedAt: period.StartTimestamp + 10, Type: LogTypeConsume, ModelName: "playground", Quota: 100, RequestId: "customer-seven"},
		{UserId: 8, CreatedAt: period.StartTimestamp + 20, Type: LogTypeConsume, ModelName: "playground", Quota: 80, RequestId: "customer-eight"},
	}
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, BuildUsageReportDay(ctx, period.Date, "report", "runner", nil))
	view, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, view.Total.NetQuota)
	assert.EqualValues(t, 1, view.Total.TotalCalls)
	require.Len(t, meta.References, 1)
	overview, _, err := GetPublishedUsageOverview(ctx, period, "")
	require.NoError(t, err)
	assert.EqualValues(t, 180, overview.Total.NetQuota)
	assert.EqualValues(t, 2, overview.Total.TotalCalls)
	other, _, err := GetPublishedUsageCustomer(ctx, period, 8, "")
	require.NoError(t, err)
	assert.EqualValues(t, 80, other.Total.NetQuota)

	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: period.StartTimestamp + 30, Type: LogTypeConsume, ModelName: "playground", Quota: 50, RequestId: "arrived-late"}).Error)
	require.NoError(t, BuildUsageReportDay(ctx, period.Date, "report", "runner", nil))
	fresh, updated, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 150, fresh.Total.NetQuota)
	assert.NotEqual(t, meta.References, updated.References)
	frozen, _, err := GetPublishedUsageCustomer(WithUsageReportReferences(ctx, meta.References), period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, frozen.Total.NetQuota)
}

func TestPublishedUsageModelTotalsCombineCustomersAcrossNaturalMonth(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.AutoMigrate(&usageReportDay{}, &usageReportDayRow{}, &SystemTaskLock{}, &SystemTask{}, &dailyReportStage{}, &dailyReportPayloadPart{}, &dailyReportRetry{}))
	ctx := context.Background()
	now := common.GetTimestamp()
	month, err := ResolveUsageAnalyticsPeriod("month", "2026-09-16", now)
	require.NoError(t, err)
	require.NoError(t, db.Create(&SystemTaskLock{Type: UsageReportTaskType, TaskID: "report", LockedBy: "runner", LockedUntil: now + 60}).Error)
	logs := []Log{
		{UserId: 7, CreatedAt: month.StartTimestamp + 10, Type: LogTypeConsume, ModelName: "shared", Quota: 100, RequestId: "model-first", Other: `{"completed_image_count":3,"statement_snapshot":{"billing_mode":"per_second","usage_units":{"duration":"second"}},"usage_facts":{"duration":0.1}}`},
		{UserId: 8, CreatedAt: month.EndTimestamp - 20, Type: LogTypeConsume, ModelName: "shared", Quota: 80, RequestId: "model-last", Other: `{"statement_snapshot":{"billing_mode":"per_second","usage_units":{"duration":"second"}},"usage_facts":{"duration":0.2}}`},
		{UserId: 8, CreatedAt: month.EndTimestamp - 10, Type: LogTypeConsume, ModelName: "second", Quota: 60, RequestId: "model-second"},
		{UserId: 8, CreatedAt: month.EndTimestamp + 1, Type: LogTypeConsume, ModelName: "outside", Quota: 700, RequestId: "model-outside"},
	}
	require.NoError(t, db.Create(&logs).Error)
	for _, day := range []string{"2026-09-01", "2026-09-30", "2026-10-01"} {
		require.NoError(t, BuildUsageReportDay(ctx, day, "report", "runner", nil))
	}
	overview, meta, err := GetPublishedUsageOverview(ctx, month, "")
	require.NoError(t, err)
	require.Len(t, meta.References, 2)
	frozen := WithUsageReportReferences(ctx, meta.References)
	totals, err := GetPublishedUsageModelTotals(frozen, month)
	require.NoError(t, err)
	require.Len(t, totals, 2)
	assert.Equal(t, "shared", totals[1].ModelName)
	assert.EqualValues(t, 2, totals[1].Total.TotalCalls)
	assert.EqualValues(t, 180, totals[1].Total.NetQuota)
	assert.EqualValues(t, overview.Total.NetQuota, totals[0].Total.NetQuota+totals[1].Total.NetQuota)
	first, err := GetPublishedUsageRanks(frozen, month, UsageReportRankQuery{Kind: "model", Metric: "amount", Page: 1, Size: 1})
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	assert.True(t, first.HasMore)
	assert.EqualValues(t, 2, first.Total)
	assert.Equal(t, "shared", first.Items[0].Name)
	require.NotNil(t, first.Items[0].Amount)
	assert.EqualValues(t, 180, *first.Items[0].Amount)
	require.NotNil(t, first.Items[0].NonToken)
	assert.Equal(t, BillingReconciliationModePerSecond, first.Items[0].BillingMode)
	assert.Zero(t, first.Items[0].NonToken.ImageCount)
	assert.False(t, first.Items[0].NonToken.ImageCountKnown)
	require.NotNil(t, first.Items[0].NonToken.Seconds)
	assert.Equal(t, "0.3", first.Items[0].NonToken.Seconds.String())
	assert.Equal(t, overview.Total.Seconds, first.Items[0].NonToken.Seconds)
	second, err := GetPublishedUsageRanks(frozen, month, UsageReportRankQuery{Kind: "model", Metric: "amount", Page: 2, Size: 1})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.False(t, second.HasMore)
	assert.EqualValues(t, 2, second.Total)
	assert.Equal(t, "second", second.Items[0].Name)

	// A generation published before the model projection gives the same totals.
	require.NoError(t, db.Where("generation = ? AND user_id = ?", meta.References[0].Generation, usageReportModelScope).Delete(&usageReportDayRow{}).Error)
	historical, err := GetPublishedUsageModelTotals(frozen, month)
	require.NoError(t, err)
	assert.Equal(t, totals, historical)
	// An older rank row without unit fields still uses its own frozen generation.
	require.NoError(t, db.Model(&usageReportRank{}).Where("generation = ?", meta.References[0].Generation).Update("non_token_payload", nil).Error)
	mixed, err := GetPublishedUsageRanks(frozen, month, UsageReportRankQuery{Kind: "model", Metric: "amount", Page: 1, Size: 1})
	require.NoError(t, err)
	assert.Equal(t, first, mixed)
}

func TestPublishedUsageChannelRankFiltersAndPagesFrozenGroups(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&BatchJob{}))
	ctx := context.Background()
	zone := UsageAnalyticsZone()
	date := time.Unix(start, 0).In(zone).Format(time.DateOnly)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 21, Name: "small", BaseURL: common.GetPointer("https://rank.example")},
		{Id: 22, Name: "large", BaseURL: common.GetPointer("https://rank.example")},
		{Id: 23, Name: "outside", BaseURL: common.GetPointer("https://other.example")},
	}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, ChannelId: 21, ModelName: "text", RequestId: "small", Type: LogTypeConsume, CreatedAt: start + 1, PromptTokens: 4, Other: `{"image_count":9}`},
		{UserId: 7, ChannelId: 22, ModelName: "text", RequestId: "large", Type: LogTypeConsume, CreatedAt: start + 2, PromptTokens: 10, Other: `{"completed_image_count":0,"statement_snapshot":{"billing_mode":"per_second","usage_units":{"duration":"second"}}}`},
		{UserId: 7, ChannelId: 23, ModelName: "text", RequestId: "outside", Type: LogTypeConsume, CreatedAt: start + 3, PromptTokens: 100, Other: `{"completed_image_count":4}`},
	}).Error)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	period, err := ResolveUsageAnalyticsPeriod("day", date, common.GetTimestamp())
	require.NoError(t, err)
	refs, err := FreezeUsageReportReferences(ctx, period, nil)
	require.NoError(t, err)
	filter := UsageReportRankQuery{Kind: "channel", Metric: "tokens", URLKey: "https://rank.example", Page: 1, Size: 1}
	frozen := WithUsageReportReferences(ctx, refs)
	first, err := GetPublishedUsageRanks(frozen, period, filter)
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	assert.EqualValues(t, 22, first.Items[0].ChannelID)
	require.NotNil(t, first.Items[0].NonToken)
	assert.True(t, first.Items[0].NonToken.ImageCountKnown)
	assert.Zero(t, first.Items[0].NonToken.ImageCount)
	assert.Nil(t, first.Items[0].NonToken.Seconds)
	assert.EqualValues(t, 1, first.Items[0].NonToken.SecondsMissingRows)
	assert.True(t, first.HasMore)
	filter.Page++
	second, err := GetPublishedUsageRanks(frozen, period, filter)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.EqualValues(t, 21, second.Items[0].ChannelID)
	require.NotNil(t, second.Items[0].NonToken)
	assert.Zero(t, second.Items[0].NonToken.ImageCount)
	assert.False(t, second.Items[0].NonToken.ImageCountKnown)
	assert.EqualValues(t, 1, second.Items[0].NonToken.ImageCountMissingRows)
	assert.False(t, second.HasMore)
}

func TestPublishedUsageReportShowsOnlyOutstandingRefundsOver24Hours(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.AutoMigrate(&usageReportDay{}, &usageReportDayRow{}, &SystemTaskLock{}, &SystemTask{}, &dailyReportStage{}, &dailyReportPayloadPart{}, &dailyReportRetry{}))
	ctx := context.Background()
	now := common.GetTimestamp()
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", now)
	require.NoError(t, err)
	require.NoError(t, db.Create(&SystemTaskLock{Type: UsageReportTaskType, TaskID: "report", LockedBy: "runner", LockedUntil: now + 60}).Error)
	base := Task{UserId: 7, ChannelId: 21, Platform: "video", Status: TaskStatusSuccess,
		FinishTime: period.StartTimestamp + 10, BillingState: TaskBillingStateSettled, Quota: 80,
		Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 11}}
	older, newer, completed := base, base, base
	older.TaskID, newer.TaskID, completed.TaskID = "overdue", "recent", "completed"
	older.VideoRefund = VideoRefund{VideoRefundState: "pending", VideoRefundRequestedAt: now - 86401}
	newer.VideoRefund = VideoRefund{VideoRefundState: "pending", VideoRefundRequestedAt: now - 86000}
	completed.VideoRefund = VideoRefund{VideoRefundState: "refunded", VideoRefundRequestedAt: now - 90000, VideoRefundCompletedAt: now - 1}
	require.NoError(t, db.Create(&[]Task{older, newer, completed}).Error)
	count, err := CountOverdueBillingRefunds(ctx, 7, period.StartTimestamp, period.EndTimestamp-1, now-1)
	require.NoError(t, err)
	assert.Zero(t, count, "exactly 24 hours is not overdue")
	count, err = CountOverdueBillingRefunds(ctx, 7, period.StartTimestamp, period.EndTimestamp-1, now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	require.NoError(t, BuildUsageReportDay(ctx, period.Date, "report", "runner", nil))
	_, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	require.Len(t, meta.PendingRefunds, 1)
	assert.Equal(t, "overdue", meta.PendingRefunds[0].TaskID)
	_, other, err := GetPublishedUsageCustomer(ctx, period, 8, "")
	require.NoError(t, err)
	assert.Empty(t, other.PendingRefunds)
}

func TestUsageReportHonorsShanghaiPublicationWindow(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, MigrateDailyReports(db))
	ctx := context.Background()
	today := time.Date(2026, 11, 11, 0, 0, 0, 0, UsageAnalyticsZone()).Unix()
	require.NoError(t, db.Create(&usageReportDay{Start: today - 86400, AsOf: today - 1, Generation: "provisional"}).Error)

	before, _, due, _, err := NextDailyReportDay(ctx, today, today+3*3600-1)
	require.NoError(t, err)
	assert.True(t, due)
	assert.Equal(t, today, before)
	after, _, due, _, err := NextDailyReportDay(ctx, today, today+3*3600)
	require.NoError(t, err)
	assert.True(t, due)
	assert.Equal(t, today-86400, after)
}

func TestUsageReportDoesNotStarveHistoryWhenTodayBuildRunsLong(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, MigrateDailyReports(db))
	ctx := context.Background()
	today := time.Date(2026, 11, 11, 0, 0, 0, 0, UsageAnalyticsZone()).Unix()
	require.NoError(t, db.Create(&[]usageReportDay{
		{Start: today - 86400, AsOf: today + 3*3600, MeterVersion: usageReportMeterVersion},
		{Start: today, AsOf: today + 4*3600, MeterVersion: usageReportMeterVersion},
	}).Error)
	require.NoError(t, db.Create(&[]billingReportHead{
		{Start: today - 86400, AsOf: today + 3*3600},
		{Start: today, AsOf: today + 4*3600},
	}).Error)
	now := today + 4*3600 + dailyReportRefreshSeconds + 1
	next, _, due, _, err := NextDailyReportDay(ctx, today, now)
	require.NoError(t, err)
	assert.True(t, due)
	assert.Equal(t, today-2*86400, next)
	require.NoError(t, db.Create(&usageReportDay{Start: next, AsOf: now, MeterVersion: usageReportMeterVersion}).Error)
	require.NoError(t, db.Create(&billingReportHead{Start: next, AsOf: now}).Error)
	next, _, due, _, err = NextDailyReportDay(ctx, today, now+1)
	require.NoError(t, err)
	assert.True(t, due)
	assert.Equal(t, today, next)
}
