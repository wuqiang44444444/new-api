package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReportExecutionLeaseSerializesWorkersAndAllowsTakeover(t *testing.T) {
	_, _, _ = setupBillingReportTest(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Second)
	const ttl = 15 * time.Second
	allowed, err := RenewReportExecutionLease(ctx, "first-node", now, ttl)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = RenewReportExecutionLease(ctx, "second-node", now.Add(time.Second), ttl)
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = RenewReportExecutionLease(ctx, "first-node", now.Add(5*time.Second), ttl)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = RenewReportExecutionLease(ctx, "second-node", now.Add(ttl+time.Second), ttl)
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = RenewReportExecutionLease(ctx, "second-node", now.Add(ttl+6*time.Second), ttl)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = RenewReportExecutionLease(ctx, "first-node", now.Add(ttl+7*time.Second), ttl)
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestDailyReportBacklogDetectsAnOverlappingCleanup(t *testing.T) {
	db, selected, _ := setupBillingReportTest(t)
	older := selected - billingReportDaySeconds
	require.NoError(t, db.Create(&billingReportRebuild{Start: older, RequestedAt: time.Now().UnixNano()}).Error)
	cutoff := older + 1
	_, err := CreateSystemTask(SystemTaskTypeLogCleanup, map[string]any{"target_timestamp": cutoff}, nil)
	require.NoError(t, err)
	first, target, err := DailyReportSourceBacklog(t.Context(), selected)
	require.NoError(t, err)
	require.Equal(t, older, first)
	require.Equal(t, cutoff, target)
}
