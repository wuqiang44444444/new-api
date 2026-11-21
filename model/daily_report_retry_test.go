package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportRetrySelectsCurrentVersions(t *testing.T) {
	db, today, _ := setupBillingReportTest(t)
	ctx := context.Background()
	now := today + 3600
	billingCurrent := today - 5*billingReportDaySeconds
	usageCurrent := today - 4*billingReportDaySeconds
	stale := today - 3*billingReportDaySeconds
	expired := today - 2*billingReportDaySeconds
	require.NoError(t, db.Create(&[]dailyReportRetry{
		{Start: billingCurrent, Kind: DailyReportBilling, ReleaseVersion: CustomerOriginalReplayVersion, RetryAt: now + 60},
		{Start: usageCurrent, Kind: DailyReportUsage, ReleaseVersion: usageReportMeterVersion, RetryAt: now + 60},
		{Start: stale, Kind: DailyReportUsage, ReleaseVersion: usageReportMeterVersion - 1, RetryAt: now + 60},
		{Start: stale, Kind: DailyReportBilling, ReleaseVersion: CustomerOriginalReplayVersion - 1, RetryAt: now + 60},
		{Start: expired, Kind: DailyReportBilling, ReleaseVersion: CustomerOriginalReplayVersion, RetryAt: now - 1},
	}).Error)
	active, err := dailyReportRetryDates(ctx, "", now)
	require.NoError(t, err)
	assert.Equal(t, map[int64]bool{billingCurrent: true, usageCurrent: true}, active)

	require.NoError(t, db.Create(&[]billingReportRebuild{
		{Start: billingCurrent, RequestedAt: 1},
		{Start: usageCurrent, RequestedAt: 2},
		{Start: stale, RequestedAt: 3},
	}).Error)
	pending, err := DiscoverCustomerOriginalReplayDays(ctx, today, now)
	require.NoError(t, err)
	assert.True(t, pending)
	start, requestedAt, due, _, err := NextDailyReportDay(ctx, today, now)
	require.NoError(t, err)
	assert.True(t, due)
	assert.Equal(t, stale, start)
	assert.EqualValues(t, 3, requestedAt)
}

func TestUnpublishedReportAutomaticallyRetriesAndClearsTheDiagnosticAfterPublication(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := t.Context()
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	end := start + billingReportDaySeconds
	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "text", Quota: 100}).Error)
	_, frozen, err := ResolveBillingReport(ctx, start, end-1, "")
	require.NoError(t, err)
	assert.Equal(t, []string{date}, frozen.MissingDays)
	assert.Zero(t, frozen.PublishedDays)

	for _, kind := range []string{DailyReportBilling, DailyReportUsage} {
		require.NoError(t, DeferDailyReportDate(ctx, start, kind, context.DeadlineExceeded))
	}
	require.NoError(t, DeferDailyReportDate(ctx, start, DailyReportBilling, ErrDailyReportResourcePressure))
	days, err := BillingReportDiagnosticDays(ctx, start, end)
	require.NoError(t, err)
	require.Len(t, days, 2)
	assert.Equal(t, DailyReportBilling, days[0].Kind)
	assert.Equal(t, date, days[0].Date)
	assert.Equal(t, "retrying", days[0].Status)
	assert.Equal(t, "timeout", days[0].FailureReason)
	assert.Greater(t, days[0].RetryAt, common.GetTimestamp())

	// Today is past the rolling first-discovery window for this failed day.
	// Keep one projection's cooldown active, then let both retries become due.
	today := start + 100*billingReportDaySeconds
	now := today + 3600
	require.NoError(t, db.Model(&dailyReportRetry{}).Where("start = ? AND kind = ?", start, DailyReportUsage).Update("retry_at", now+60).Error)
	selected, _, due, _, err := NextDailyReportDay(ctx, today, now)
	require.NoError(t, err)
	assert.True(t, due)
	assert.NotEqual(t, start, selected)
	selected, _, due, usageOnly, err := NextDailyReportDay(ctx, today, now+60)
	require.NoError(t, err)
	require.True(t, due)
	require.Equal(t, start, selected)
	assert.False(t, usageOnly)

	staged := StageDailyReport(ctx)
	require.NoError(t, BuildBillingReportDay(staged, start, "billing-test", "runner", nil, nil))
	require.NoError(t, BuildUsageReportDay(staged, date, "billing-test", "runner", nil))
	scopes := []DailyReportScope{{Start: start, Kind: DailyReportBilling}, {Start: start, Kind: DailyReportUsage}}
	require.NoError(t, PublishDailyReportStages(staged, scopes, "billing-test", "runner"))

	days, err = BillingReportDiagnosticDays(ctx, start, end)
	require.NoError(t, err)
	require.Len(t, days, 2)
	for _, day := range days {
		assert.NotEqual(t, "retrying", day.Status)
		assert.Empty(t, day.FailureReason)
		assert.Zero(t, day.RetryAt)
	}
	reportCtx, latest, err := ResolveBillingReport(ctx, start, end-1, "")
	require.NoError(t, err)
	assert.Empty(t, latest.MissingDays)
	assert.Equal(t, 1, latest.PublishedDays)
	statement, err := GetBillingCustomerStatement(reportCtx, 7, start, end-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, statement.Summary.NetQuota)
	_, original, err := ResolveBillingReport(ctx, start, end-1, frozen.Version)
	require.NoError(t, err)
	assert.Equal(t, frozen.MissingDays, original.MissingDays)
}
