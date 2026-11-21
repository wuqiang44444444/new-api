package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDailyReportSlowPublicationRechecksNewlyCommittedLogs(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := WithLowPriorityDailyReport(t.Context())
	log := Log{UserId: 7, ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 10}
	require.NoError(t, db.Create(&log).Error)
	buildCtx, err := WithDailyReportLogBoundary(StageDailyReport(ctx))
	require.NoError(t, err)
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	require.NoError(t, BuildBillingReportDay(buildCtx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, BuildUsageReportDay(buildCtx, date, "billing-test", "runner", nil))

	// A larger committed ID arrives while the candidate waits for capacity.
	log.Id, log.Quota = 0, 35
	require.NoError(t, db.Create(&log).Error)
	scopes := []DailyReportScope{{Start: start, Kind: DailyReportBilling}, {Start: start, Kind: DailyReportUsage}}
	require.ErrorIs(t, PublishScheduledDailyReport(buildCtx, scopes, "billing-test", "runner", start, nil), errDailyReportSourceChanged)
	var count int64
	require.NoError(t, db.Model(&billingReportHead{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&usageReportDay{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestDailyReportReadsExistingOpenDaySnapshotAtItsBuildBoundary(t *testing.T) {
	db, _, _ := setupBillingReportTest(t)
	now := time.Now().In(UsageAnalyticsZone())
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	log := Log{UserId: 7, ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 10}
	require.NoError(t, db.Create(&log).Error)
	ctx, err := WithDailyReportLogBoundary(StageDailyReport(WithLowPriorityDailyReport(t.Context())))
	require.NoError(t, err)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, BuildUsageReportDay(ctx, now.Format(time.DateOnly), "billing-test", "runner", nil))
	log.Id, log.Quota = 0, 35
	require.NoError(t, db.Create(&log).Error)

	scopes := []DailyReportScope{{Start: start, Kind: DailyReportBilling}, {Start: start, Kind: DailyReportUsage}}
	// A snapshot created before closed-day scheduling remains readable.
	require.NoError(t, PublishDailyReportStages(ctx, scopes, "billing-test", "runner"))
	_, manifest, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	require.Equal(t, 1, manifest.PublishedDays)
	assert.True(t, manifest.Estimated)
	var billing billingReportGeneration
	require.NoError(t, db.Where("start = ? AND published = ?", start, true).Take(&billing).Error)
	assert.EqualValues(t, 1, billing.Rows)
	assert.Less(t, billing.UpperLogID, int64(log.Id))
	var usage usageReportDay
	require.NoError(t, db.Where("start = ?", start).Take(&usage).Error)
	assert.Equal(t, billing.UpperLogID, usage.UpperLogID)
	assert.EqualValues(t, 1, usage.SourceRows)
}

func TestDailyReportBillingResumesCommittedDetailsAndSummaries(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	scope := DailyReportScope{Start: start, Kind: DailyReportBilling}
	ctx := StageDailyReport(WithLowPriorityDailyReport(t.Context()))
	logs := make([]Log, 29)
	for i := range logs {
		logs[i] = Log{UserId: 7, ModelName: "text", TokenId: 9, Type: LogTypeConsume, CreatedAt: start + int64(len(logs)-i), Quota: 10}
	}
	logs[0].Type, logs[0].Quota = LogTypeRefund, 5
	require.NoError(t, db.Create(&logs).Error)
	// Publish an independent reference so the resumed build must also pass
	// the exact previous-generation funding check.
	require.NoError(t, BuildBillingReportDay(t.Context(), start, "billing-test", "runner", nil, nil))
	var original billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&original).Error)

	limited := WithDailyReportWorkerBudget(ctx, nil, func(ctx context.Context, n int) error {
		var count int64
		require.NoError(t, db.WithContext(ctx).Model(&billingReportLine{}).Where("generation <> ?", original.Generation).Count(&count).Error)
		if count > 0 {
			return ErrDailyReportResourcePressure
		}
		return ctx.Err()
	})
	require.ErrorIs(t, BuildBillingReportDay(limited, start, "billing-test", "runner", nil, nil), ErrDailyReportResourcePressure)
	var draft dailyReportStage
	require.NoError(t, db.Where("start = ? AND kind = ?", start, DailyReportBilling).Take(&draft).Error)
	require.Positive(t, draft.BillingBuildRows)
	assert.Less(t, draft.BillingBuildRows, int64(len(logs)))
	require.ErrorIs(t, PublishDailyReportStages(ctx, []DailyReportScope{scope}, "billing-test", "runner"), errDailyReportSourceChanged)
	ready, err := GetDailyReportStage(ctx, scope, 0)
	require.NoError(t, err)
	require.False(t, ready)

	// A different worker takes over. Fail after writing the upstream
	// summary, then complete without rewriting it or any detail.
	require.NoError(t, db.Model(&SystemTaskLock{}).Where("type = ?", UsageReportTaskType).Update("locked_by", "runner-resumed").Error)
	limited = WithDailyReportWorkerBudget(ctx, nil, func(ctx context.Context, n int) error {
		var written int64
		require.NoError(t, db.WithContext(ctx).Model(&billingReportUpstreamDay{}).Where("generation = ?", draft.Generation).Count(&written).Error)
		if written > 0 {
			return ErrDailyReportResourcePressure
		}
		return ctx.Err()
	})
	require.ErrorIs(t, BuildBillingReportDay(limited, start, "billing-test", "runner-resumed", nil, nil), ErrDailyReportResourcePressure)
	var restaged dailyReportStage
	require.NoError(t, db.Where("start = ? AND kind = ?", start, DailyReportBilling).Take(&restaged).Error)
	require.Equal(t, draft.Generation, restaged.Generation)
	require.EqualValues(t, len(logs), restaged.BillingBuildRows)

	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail_duplicate_report_writes", func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_report_lines" || tx.Statement.Table == "billing_report_upstream_days" {
			tx.AddError(assert.AnError)
		}
	}))
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner-resumed", nil, nil))
	require.NoError(t, PublishDailyReportStages(ctx, []DailyReportScope{scope}, "billing-test", "runner-resumed"))
	var head billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
	require.Equal(t, draft.Generation, head.Generation)

	var oldLines, newLines []billingReportLine
	require.NoError(t, db.Where("generation = ?", original.Generation).Order("source_id").Find(&oldLines).Error)
	require.NoError(t, db.Where("generation = ?", head.Generation).Order("source_id").Find(&newLines).Error)
	require.Len(t, newLines, len(logs))
	for i := range oldLines {
		oldLines[i].Generation = head.Generation
	}
	assert.Equal(t, oldLines, newLines)
	manifestCtx, _, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	statement, err := GetBillingCustomerStatement(manifestCtx, 7, start, start+billingReportDaySeconds-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 275, statement.Summary.NetQuota)
}

func TestDailyReportBillingDiscardsChangedCommittedPrefix(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := StageDailyReport(WithLowPriorityDailyReport(t.Context()))
	logs := make([]Log, 26)
	for i := range logs {
		logs[i] = Log{UserId: 7, ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 10}
	}
	require.NoError(t, db.Create(&logs).Error)
	admitted := false
	limited := WithDailyReportWorkerBudget(ctx, nil, func(ctx context.Context, n int) error {
		if admitted {
			return ErrDailyReportResourcePressure
		}
		admitted = true
		return ctx.Err()
	})
	require.ErrorIs(t, BuildBillingReportDay(limited, start, "billing-test", "runner", nil, nil), ErrDailyReportResourcePressure)
	require.NoError(t, db.Model(&Log{}).Where("id = ?", logs[0].Id).Update("quota", 35).Error)
	require.ErrorIs(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil), errDailyReportSourceChanged)
	var count int64
	require.NoError(t, db.Model(&dailyReportStage{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&billingReportHead{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, BuildBillingReportDay(t.Context(), start, "billing-test", "runner", nil, nil))
	manifestCtx, _, err := ResolveBillingReport(t.Context(), start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	statement, err := GetBillingCustomerStatement(manifestCtx, 7, start, start+billingReportDaySeconds-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 285, statement.Summary.NetQuota)
}
