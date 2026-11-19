package model

import (
	"context"
	"testing"

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
