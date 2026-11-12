package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerOriginalReplayDiscoverySchedulesOldDatesAutomatically(t *testing.T) {
	db, today, _ := setupBillingReportTest(t)
	ctx := context.Background()
	old := today - 120*billingReportDaySeconds
	missing := old - 4*billingReportDaySeconds
	current := today - 2*billingReportDaySeconds
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, Type: LogTypeConsume, CreatedAt: old + 1},
		{UserId: 7, Type: LogTypeRefund, CreatedAt: missing + 1},
		{UserId: 7, Type: LogTypeConsume, CreatedAt: current + 1},
	}).Error)
	require.NoError(t, db.Create(&[]billingReportGeneration{
		{ID: "old", Start: old, Published: true, OriginalReplayVersion: CustomerOriginalReplayVersion - 1},
		{ID: "current", Start: current, Published: true, OriginalReplayVersion: CustomerOriginalReplayVersion},
	}).Error)
	require.NoError(t, db.Create(&billingReportSourceCursor{
		ID: billingReportReplayCursorID, OriginalReplayVersion: CustomerOriginalReplayVersion - 1,
	}).Error)
	require.NoError(t, db.Create(&[]billingReportHead{
		{Start: old, Generation: "old"},
		{Start: current, Generation: "current"},
	}).Error)
	more, err := DiscoverCustomerOriginalReplayDays(ctx, today, today)
	require.NoError(t, err)
	assert.True(t, more)
	var queued billingReportRebuild
	require.NoError(t, db.Take(&queued).Error)
	assert.Equal(t, old, queued.Start)
	assert.Equal(t, CustomerOriginalReplayVersion, queued.OriginalReplayVersion)

	// Discovery resumes only after the worker consumes the first queue item.
	more, err = DiscoverCustomerOriginalReplayDays(ctx, today, today)
	require.NoError(t, err)
	assert.True(t, more)
	var count int64
	require.NoError(t, db.Model(&billingReportRebuild{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, CompleteBillingReportRebuild(ctx, queued.Start, queued.RequestedAt))

	more, err = DiscoverCustomerOriginalReplayDays(ctx, today, today)
	require.NoError(t, err)
	assert.True(t, more)
	var older billingReportRebuild
	require.NoError(t, db.Where("start = ?", missing).Take(&older).Error)
	require.NoError(t, CompleteBillingReportRebuild(ctx, older.Start, older.RequestedAt))
	for range 2 {
		more, err = DiscoverCustomerOriginalReplayDays(ctx, today, today)
		require.NoError(t, err)
		assert.False(t, more)
	}
	require.NoError(t, db.Model(&billingReportRebuild{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestDailyReportSelectsCurrentRefreshBeforeAutomaticOriginalReplay(t *testing.T) {
	db, today, _ := setupBillingReportTest(t)
	ctx := context.Background()
	yesterday := today - billingReportDaySeconds
	old := today - 120*billingReportDaySeconds
	now := today + 4*3600
	require.NoError(t, db.Create(&Log{UserId: 7, Type: LogTypeConsume, CreatedAt: old + 1}).Error)
	_, err := DiscoverCustomerOriginalReplayDays(ctx, today, now)
	require.NoError(t, err)

	next, _, due, _, err := NextDailyReportDay(ctx, today, now)
	require.NoError(t, err)
	require.True(t, due)
	assert.Equal(t, yesterday, next)
	require.NoError(t, db.Create(&usageReportDay{Start: yesterday, AsOf: now}).Error)
	require.NoError(t, db.Create(&billingReportHead{Start: yesterday, AsOf: now}).Error)

	next, _, due, _, err = NextDailyReportDay(ctx, today, now)
	require.NoError(t, err)
	require.True(t, due)
	assert.Equal(t, today, next)
	require.NoError(t, db.Create(&usageReportDay{Start: today, AsOf: now}).Error)
	require.NoError(t, db.Create(&billingReportHead{Start: today, AsOf: now}).Error)

	next, revision, due, _, err := NextDailyReportDay(ctx, today, now)
	require.NoError(t, err)
	require.True(t, due)
	assert.Equal(t, old, next)
	assert.Positive(t, revision)
}
