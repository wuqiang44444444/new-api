package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDailyReportPublicationOwnsExactFundingCheck(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := t.Context()
	original := Log{UserId: 7, TokenId: 11, ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 100}
	require.NoError(t, db.Create(&original).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	scope := DailyReportScope{Start: start, Kind: DailyReportBilling}
	require.NoError(t, BuildBillingReportDay(StageDailyReport(ctx), start, "billing-test", "runner", nil, nil))
	var staged dailyReportStage
	require.NoError(t, db.Where("start = ? AND kind = ?", start, scope.Kind).Take(&staged).Error)

	// A candidate with valid source witnesses but altered funding must not
	// replace the published version. Reuse leaves immutable checks to publish.
	require.NoError(t, db.Model(&billingReportLine{}).Where("generation = ? AND source_id = ?", staged.Generation, original.Id).
		Update("customer_charge", 99).Error)
	ready, err := GetDailyReportStage(ctx, scope, 0)
	require.NoError(t, err)
	require.True(t, ready)
	require.ErrorIs(t, PublishDailyReportStages(ctx, []DailyReportScope{scope}, "billing-test", "runner"), errDailyReportCoverageDecreased)
	var head billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
	assert.Equal(t, staged.Previous, head.Generation)

	var retry dailyReportRetry
	require.NoError(t, db.Where("start = ? AND kind = ?", start, scope.Kind).Take(&retry).Error)
	require.True(t, retry.CoverageIssue)
	require.NoError(t, DeferDailyReportDate(ctx, start, scope.Kind, ErrDailyReportResourcePressure))
	var retained dailyReportRetry
	require.NoError(t, db.Where("start = ? AND kind = ?", start, scope.Kind).Take(&retained).Error)
	assert.Equal(t, retry, retained)
}

func TestDailyReportRechecksPostgresSourceAfterExactFundingCheck(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	if db.Dialector.Name() != "postgres" {
		t.Skip("tuple revisions require PostgreSQL")
	}
	ctx := t.Context()
	log := Log{UserId: 7, ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 100}
	require.NoError(t, db.Create(&log).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	var prior billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&prior).Error)
	require.NoError(t, BuildBillingReportDay(StageDailyReport(ctx), start, "billing-test", "runner", nil, nil))
	corrected := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:late_source_correction", func(tx *gorm.DB) {
		if !corrected && tx.Statement.Table == "billing_report_lines" {
			corrected = true
			require.NoError(t, db.Model(&Log{}).Where("id = ?", log.Id).Update("quota", 110).Error)
		}
	}))
	scope := DailyReportScope{Start: start, Kind: DailyReportBilling}
	require.ErrorIs(t, PublishDailyReportStages(ctx, []DailyReportScope{scope}, "billing-test", "runner"), errDailyReportSourceChanged)
	assert.True(t, corrected)
	var head billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
	assert.Equal(t, prior.Generation, head.Generation)
}

func TestDailyReportJointPublishRechecksUsageAfterBillingDependencies(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	if db.Dialector.Name() != "postgres" && db.Dialector.Name() != "sqlite" {
		t.Skip("PostgreSQL exercises the shared log snapshot")
	}
	ctx := StageDailyReport(t.Context())
	scope := []DailyReportScope{{Start: start, Kind: DailyReportBilling}, {Start: start, Kind: DailyReportUsage}}
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	failed := Log{UserId: 7, CreatedAt: start + 2, Type: LogTypeError, RequestId: "changed"}
	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: start + 1, Type: LogTypeConsume, Quota: 10}).Error)
	require.NoError(t, db.Create(&failed).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))

	if db.Dialector.Name() == "postgres" {
		batches := 0
		yield := WithDailyReportValidationGate(ctx, func(context.Context) error {
			batches++
			if batches > 1 {
				return ErrDailyReportResourcePressure
			}
			return nil
		})
		require.ErrorIs(t, PublishDailyReportStages(yield, scope, "billing-test", "runner"), ErrDailyReportResourcePressure)
		changed := false
		require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:usage_change_during_dependencies", func(tx *gorm.DB) {
			if !changed && tx.Statement.Table == "tasks" && strings.Contains(tx.Statement.SQL.String(), "task_billing_deliveries") {
				changed = true
				require.NoError(t, db.Model(&Log{}).Where("id = ?", failed.Id).Update("type", LogTypeSystem).Error)
			}
		}))
		require.ErrorIs(t, PublishDailyReportStages(ctx, scope, "billing-test", "runner"), errDailyReportSourceChanged)
		require.True(t, changed)
	} else {
		// If capacity disappears after the first scope's source check, neither
		// scope can publish from a partial validation attempt.
		batches := 0
		yield := WithDailyReportValidationGate(ctx, func(context.Context) error {
			batches++
			if batches == 2 {
				return ErrDailyReportResourcePressure
			}
			return nil
		})
		require.ErrorIs(t, PublishDailyReportStages(yield, scope, "billing-test", "runner"), ErrDailyReportResourcePressure)
		require.NoError(t, db.Model(&Log{}).Where("id = ?", failed.Id).Update("type", LogTypeSystem).Error)
		require.ErrorIs(t, PublishDailyReportStages(ctx, scope, "billing-test", "runner"), errDailyReportSourceChanged)
	}
	var count int64
	require.NoError(t, db.Model(&billingReportHead{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&usageReportDay{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&dailyReportStage{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestDailyReportHistoricalCopyYieldsBeforePublishingAndPreservesEvidence(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	const prior = "frozen-billing"
	const lineCount = 105
	ctx := t.Context()
	require.NoError(t, db.Create(&billingReportGeneration{ID: prior, Start: start, AsOf: start + 2*billingReportDaySeconds,
		OriginalReplayVersion: CustomerOriginalReplayVersion - 1, Incomplete: true, Published: true, Rows: lineCount, UpperLogID: lineCount}).Error)
	require.NoError(t, db.Create(&billingReportHead{Start: start, Generation: prior}).Error)
	lines := make([]billingReportLine, 0, lineCount)
	for id := int64(1); id <= lineCount; id++ {
		lines = append(lines, billingReportLine{Generation: prior, SourceID: id, UserID: 7, LogType: LogTypeConsume,
			CustomerCharge: id, CreatedAt: start + id})
	}
	lines[0].ReplayIssue, lines[0].PreviousOriginalMissing = "replay", true
	lines[len(lines)-1].PriceIssue = "price"
	require.NoError(t, db.CreateInBatches(&lines, 50).Error)
	parts := []dailyReportPayloadPart{
		{Generation: prior, Kind: reportBillingCustomerPayload, ScopeID: 6, Part: 0, Payload: "["},
		{Generation: prior, Kind: reportBillingCustomerPayload, ScopeID: 6, Part: 1, Payload: "1"},
		{Generation: prior, Kind: reportBillingCustomerPayload, ScopeID: 6, Part: 2, Payload: ","},
		{Generation: prior, Kind: reportBillingCustomerPayload, ScopeID: 6, Part: 3, Payload: "2"},
		{Generation: prior, Kind: reportBillingCustomerPayload, ScopeID: 6, Part: 4, Payload: "]"},
		{Generation: prior, Kind: reportBillingUpstreamPayload, ScopeID: 8, Part: 0, Payload: "{}"},
	}
	require.NoError(t, db.Create(&parts).Error)
	scope := DailyReportScope{Start: start, Kind: DailyReportBilling}
	limited := WithDailyReportWorkerBudget(ctx, nil, func(ctx context.Context, rows int) error {
		var committed int64
		require.NoError(t, db.WithContext(ctx).Model(&billingReportLine{}).Where("generation <> ?", prior).Count(&committed).Error)
		if committed > 0 {
			assert.Less(t, committed, int64(lineCount), "a partial candidate can stop between commits")
			return ErrDailyReportResourcePressure
		}
		return ctx.Err()
	})
	done, err := MigrateHistoricalDailyReport(limited, scope, "billing-test", "runner")
	require.ErrorIs(t, err, ErrDailyReportResourcePressure)
	require.False(t, done)
	var held billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&held).Error)
	require.Equal(t, prior, held.Generation)
	var checkpoint dailyReportStage
	require.NoError(t, db.Where("start = ? AND kind = ?", start, scope.Kind).Take(&checkpoint).Error)
	require.True(t, checkpoint.copyingHistory())
	require.NotEqual(t, CustomerOriginalReplayVersion, checkpoint.OriginalReplayVersion,
		"a worker without checkpoint support must also reject an unfinished generation")
	ready, err := GetDailyReportStage(ctx, scope, checkpoint.CopyingStartedAt-1)
	require.NoError(t, err)
	require.False(t, ready)
	require.ErrorIs(t, PublishDailyReportStages(ctx, []DailyReportScope{scope}, "billing-test", "runner"), errDailyReportSourceChanged)
	now := time.Now().Unix()
	require.NoError(t, db.Model(&billingReportGeneration{}).Where("id = ?", checkpoint.Generation).
		Update("created_at", now-2*dailyReportAbandonedGraceSeconds).Error)
	abandoned := billingReportGeneration{ID: "abandoned-copy", Start: start, CreatedAt: now - 2*dailyReportAbandonedGraceSeconds}
	require.NoError(t, db.Create(&abandoned).Error)
	pruned, err := PruneBillingReportGenerations(ctx, now)
	require.NoError(t, err)
	require.True(t, pruned)
	var count int64
	require.NoError(t, db.Model(&billingReportGeneration{}).Where("id = ?", abandoned.ID).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, db.Model(&billingReportGeneration{}).Where("id = ?", checkpoint.Generation).Count(&count).Error)
	require.EqualValues(t, 1, count)

	limited = WithDailyReportWorkerBudget(ctx, nil, func(ctx context.Context, rows int) error {
		var fragments int64
		require.NoError(t, db.WithContext(ctx).Model(&dailyReportPayloadPart{}).
			Where("generation = ?", checkpoint.Generation).Count(&fragments).Error)
		if fragments > 0 {
			assert.Less(t, fragments, int64(len(parts)))
			return ErrDailyReportResourcePressure
		}
		return ctx.Err()
	})
	done, err = MigrateHistoricalDailyReport(limited, scope, "billing-test", "runner")
	require.ErrorIs(t, err, ErrDailyReportResourcePressure)
	require.False(t, done)

	done, err = MigrateHistoricalDailyReport(ctx, scope, "billing-test", "runner")
	require.NoError(t, err)
	require.True(t, done)
	var published billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&published).Error)
	require.Equal(t, checkpoint.Generation, published.Generation, "a pressure retry publishes the same partial generation")
	var sum struct{ Rows, Charges int64 }
	require.NoError(t, db.Model(&billingReportLine{}).Where("generation = ?", published.Generation).
		Select("COUNT(*) AS rows, SUM(customer_charge) AS charges").Scan(&sum).Error)
	assert.EqualValues(t, lineCount, sum.Rows)
	assert.EqualValues(t, lineCount*(lineCount+1)/2, sum.Charges)
	var generation billingReportGeneration
	require.NoError(t, db.Where("id = ?", published.Generation).Take(&generation).Error)
	assert.EqualValues(t, 2, generation.IssueRows)
	payload, err := dailyReportPayload(ctx, db, published.Generation, reportBillingCustomerPayload, 6, "")
	require.NoError(t, err)
	assert.Equal(t, "[1,2]", payload)
	payload, err = dailyReportPayload(ctx, db, published.Generation, reportBillingUpstreamPayload, 8, "")
	require.NoError(t, err)
	assert.Equal(t, "{}", payload)
}

func TestDailyReportFinalErrorLookupsShareDeadlineAndDiscardPartialEvidence(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := t.Context()
	log := Log{Type: LogTypeError, CreatedAt: start, UserId: 7, ChannelId: 11, RequestId: "request", UpstreamRequestId: "sent"}
	require.NoError(t, db.Create(&log).Error)
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	period, err := ResolveUsageAnalyticsPeriod(UsageAnalyticsPeriodDay, date, start+billingReportDaySeconds)
	require.NoError(t, err)
	agg := newUsageAggregation(period, usageViewOptions{WantCustomer: true, WantUpstream: true})
	agg.errorRows = []usageLogRow{{id: int64(log.Id), fact: billingReconciliationLog{UserId: 7, ChannelId: 11, CreatedAt: start, RequestId: log.RequestId}}}
	var firstContext context.Context
	observe := func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		switch {
		case strings.Contains(sql, "AS last_id"):
			_, ok := tx.Statement.Context.Deadline()
			require.True(t, ok)
			firstContext = tx.Statement.Context
		case strings.Contains(sql, "upstream_request_id"):
			require.Same(t, firstContext, tx.Statement.Context)
			tx.AddError(context.DeadlineExceeded)
		}
	}
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:final_error_deadline", observe))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:final_error_deadline", observe))
	require.ErrorIs(t, agg.applyFinalErrors(ctx), context.DeadlineExceeded)
	assert.Empty(t, agg.customer)
	assert.Empty(t, agg.upstream)
}

func TestDailyReportDatabaseSamplerReadsForegroundCountersAndPostgresWAL(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := t.Context()
	sampler, err := DailyReportDatabaseSampler(ctx)
	require.NoError(t, err)
	before, err := sampler(ctx)
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.NoError(t, db.Create(&Log{CreatedAt: start, Type: LogTypeConsume, Quota: 70}).Error)
	after, err := sampler(ctx)
	require.NoError(t, err)
	require.Len(t, after, 1)
	conn, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, conn.Stats().MaxOpenConnections, after[0].Pool.MaxOpenConnections)
	if db.Dialector.Name() == "postgres" {
		assert.Greater(t, after[0].WALBytes, before[0].WALBytes)
	} else {
		assert.Zero(t, after[0].WALBytes)
	}
}
