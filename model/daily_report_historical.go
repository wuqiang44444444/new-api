package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	dailyReportSourceDetails = "frozen_details"
	dailyReportSourceSummary = "frozen_summary"
	dailyReportSourceMissing = "unavailable"
)

// MigrateHistoricalDailyReport preserves the strongest frozen evidence when
// an old date has insufficient primary logs. The stage tracks the current
// scan separately from that original coverage so restoration can be rechecked.
func MigrateHistoricalDailyReport(ctx context.Context, scope DailyReportScope, taskID, runnerID string) (bool, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	stage := dailyReportStage{
		Start: scope.Start, Kind: scope.Kind, Generation: common.GetUUID(),
		OriginalReplayVersion: CustomerOriginalReplayVersion, UsageMeterVersion: usageReportMeterVersion,
		Incomplete: true, HistoricalSource: dailyReportSourceMissing,
	}
	var billing billingReportGeneration
	var usage usageReportDay
	if scope.Kind == DailyReportUsage {
		if err := db.Where("start = ?", scope.Start).Limit(1).Find(&usage).Error; err != nil {
			return false, err
		}
		if usage.Generation == "" || usage.MeterVersion > usageReportMeterVersion ||
			(usage.MeterVersion == usageReportMeterVersion && usage.HistoricalSource == "") {
			return false, nil
		}
		stage.Previous, stage.AsOf, stage.HistoricalMeterVersion = usage.Generation, usage.AsOf, usage.MeterVersion
		if usage.HistoricalSource != "" {
			stage.HistoricalMeterVersion = usage.HistoricalMeterVersion
		}
	} else {
		if err := db.Table("billing_report_generations AS g").Select("g.*").
			Joins("JOIN billing_report_heads AS h ON h.generation = g.id").
			Where("h.start = ? AND (COALESCE(g.original_replay_version, 0) < ? OR (g.original_replay_version = ? AND g.historical_source <> ''))", scope.Start, CustomerOriginalReplayVersion, CustomerOriginalReplayVersion).
			Limit(1).Find(&billing).Error; err != nil {
			return false, err
		}
		if billing.ID == "" {
			return false, nil
		}
		stage.Previous, stage.AsOf = billing.ID, billing.AsOf
	}
	if stage.AsOf < stage.Start+billingReportDaySeconds {
		return false, nil
	}
	upper, err := dailyReportLogUpperBound(ctx)
	if err != nil {
		return false, err
	}
	query := dailyReportLogQuery(ctx, stage.Start, stage.Start+billingReportDaySeconds, upper, dailyReportLogTypes(scope.Kind))
	var first int64
	if err := usageReportBeforeBatch(ctx); err != nil {
		return false, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = query.WithContext(probeCtx).Select("id").Order("created_at, id").Limit(1).Scan(&first).Error
	cancel()
	if err != nil {
		return false, err
	}
	coverage := dailyReportLogScanCoverage(ctx, 0, 0)
	if first != 0 {
		// An incomplete restoration is weaker than an existing frozen report.
		// Count within its original boundary so new logs cannot mask missing rows.
		priorRows, priorUpper := max(billing.Rows, usage.SourceRows), max(billing.UpperLogID, usage.UpperLogID)
		if billing.OriginalReplayVersion == CustomerOriginalReplayVersion || usage.MeterVersion == usageReportMeterVersion {
			return false, nil
		}
		if billing.HistoricalSource == dailyReportSourceDetails && priorRows == 0 {
			var boundary struct{ Rows, Upper int64 }
			if err := usageReportBeforeBatch(ctx); err != nil {
				return false, err
			}
			readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := db.WithContext(readCtx).Model(&billingReportLine{}).Where("generation = ?", billing.ID).
				Select("COUNT(*) AS rows, COALESCE(MAX(source_id), 0) AS upper").Scan(&boundary).Error
			cancel()
			if err != nil {
				return false, err
			}
			priorRows, priorUpper = boundary.Rows, boundary.Upper
		}
		if priorRows > 0 && priorUpper > 0 {
			priorCoverage, err := readDailyReportDateCoverage(ctx, stage.Start, stage.Kind, min(upper, priorUpper))
			if err != nil || priorCoverage.Rows >= priorRows {
				return false, err
			}
		} else if usage.HistoricalSource != dailyReportSourceSummary && billing.HistoricalSource != dailyReportSourceSummary {
			return false, nil
		}
		coverage, err = readDailyReportDateCoverage(ctx, stage.Start, stage.Kind, upper)
		if err != nil {
			return false, err
		}
	}
	// An empty complete day with a zero source boundary can be regenerated
	// normally. Known cleanup or a previous nonempty day requires evidence.
	cleaned, err := dailyReportKnownLogCleanup(ctx, db, scope.Start)
	if err != nil || (!cleaned && !billing.Incomplete && !usage.Incomplete && billing.Rows == 0 && usage.SourceRows == 0 &&
		billing.HistoricalSource == "" && usage.HistoricalSource == "") {
		return false, err
	}
	// This is the current verification time. Frozen coverage and metering
	// provenance remain on the report independently of the build time.
	stage.AsOf = common.GetTimestamp()
	stage.UpperLogID, stage.SourceRows, stage.SourceRevision = upper, coverage.Rows, coverage.Revision
	stage.Dependencies, err = dailyReportDependencies(ctx, scope.Start, scope.Kind)
	if err != nil {
		return false, err
	}
	// The old generation stays pinned by the public head. A change of head
	// or release version invalidates the checkpoint, including its cursors.
	var checkpoint dailyReportStage
	err = db.Where("start = ? AND kind = ?", scope.Start, scope.Kind).Take(&checkpoint).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	resume := checkpoint.copyingHistory() && dailyReportStageVersionCurrent(checkpoint) &&
		checkpoint.Previous == stage.Previous && checkpoint.HistoricalMeterVersion == stage.HistoricalMeterVersion &&
		stage.AsOf-checkpoint.AsOf < 8*billingReportDaySeconds
	var initialize any
	if resume {
		stage.Generation, stage.CopyingStartedAt = checkpoint.Generation, checkpoint.CopyingStartedAt
		stage.HistoricalSource, stage.HistoricalCopiesDone = checkpoint.HistoricalSource, checkpoint.HistoricalCopiesDone
		stage.IssueRows, stage.IssueRowsGeneration = checkpoint.IssueRows, checkpoint.IssueRowsGeneration
	} else {
		stage.CopyingStartedAt = time.Now().UnixNano()
		if scope.Kind == DailyReportUsage {
			hasSummary, err := historicalUsageHasSummary(ctx, db, stage.Previous)
			if err != nil {
				return false, err
			}
			if hasSummary {
				stage.HistoricalSource = dailyReportSourceSummary
			}
			initialize = &usageReportDayRow{Generation: stage.Generation, UserID: -1, CreatedAt: stage.AsOf}
		} else {
			initialize = &billingReportGeneration{ID: stage.Generation, Start: stage.Start,
				OriginalReplayVersion: CustomerOriginalReplayVersion, AsOf: stage.AsOf, UpperLogID: upper,
				CreatedAt: stage.AsOf, Incomplete: true, HistoricalSource: stage.HistoricalSource}
		}
	}
	stage.CopyingGeneration = stage.Generation
	if err := persistDailyReportStage(ctx, db, stage, taskID, runnerID, initialize); err != nil {
		return false, err
	}
	if !stage.HistoricalCopiesDone {
		if err := copyHistoricalDailyReport(ctx, db, &stage); err != nil {
			return false, err
		}
		stage.HistoricalCopiesDone = true
		if err := persistDailyReportStage(ctx, db, stage, taskID, runnerID, nil); err != nil {
			return false, err
		}
	}
	if scope.Kind == DailyReportBilling && stage.HistoricalSource == dailyReportSourceDetails {
		if err := rebuildMissingHistoricalBillingCustomers(ctx, db, stage.Generation); err != nil {
			return false, err
		}
	}
	return true, saveDailyReportStage(ctx, db, stage, taskID, runnerID)
}

func copyHistoricalDailyReport(ctx context.Context, db *gorm.DB, stage *dailyReportStage) error {
	if stage.Kind == DailyReportUsage {
		// Pending refunds remain readable even when this day has no usage evidence.
		if _, err := copyDailyReportProjection(ctx, db, &usageReportDayRow{}, *stage, 8); err != nil {
			return err
		}
	} else {
		details, err := copyDailyReportProjection(ctx, db, &billingReportLine{}, *stage, 100)
		if err != nil {
			return err
		}
		if details.resumed {
			details.issues, err = countDailyReportDiagnosticRows(ctx, stage.Generation)
			if err != nil {
				return err
			}
		}
		stage.IssueRows, stage.IssueRowsGeneration = &details.issues, stage.Generation
		customers, err := copyDailyReportProjection(ctx, db, &billingReportCustomerDay{}, *stage, 8)
		if err != nil {
			return err
		}
		upstream, err := copyDailyReportProjection(ctx, db, &billingReportUpstreamDay{}, *stage, 8)
		if err != nil {
			return err
		}
		if details.hasRows {
			stage.HistoricalSource = dailyReportSourceDetails
		} else if customers.hasRows || upstream.hasRows {
			stage.HistoricalSource = dailyReportSourceSummary
		}
	}
	_, err := copyDailyReportProjection(ctx, db, &dailyReportPayloadPart{}, *stage, 4)
	return err
}

func historicalUsageHasSummary(ctx context.Context, db *gorm.DB, generation string) (bool, error) {
	// Older generations may have only detail leaves. A customer row can also
	// contain just pending refunds, and the global row may contain only discounts.
	for after := -1; ; {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return false, err
		}
		var rows []usageReportDayRow
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.WithContext(readCtx).Select("user_id", "overview", "payload").Where("generation = ? AND user_id > ?", generation, after).
			Order("user_id").Limit(16).Find(&rows).Error
		cancel()
		if err != nil {
			return false, err
		}
		for _, row := range rows {
			after = row.UserID
			if row.UserID > 0 && row.Overview != "" {
				var overview struct{ Metrics *UsageAnalyticsMetrics }
				if err := common.UnmarshalJsonStr(string(row.Overview), &overview); err != nil {
					return false, err
				}
				if overview.Metrics != nil {
					return true, nil
				}
			}
			readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			raw, err := dailyReportPayload(readCtx, db, generation, reportUsagePayload, row.UserID, row.Payload)
			cancel()
			if err != nil {
				return false, err
			}
			if raw == "" {
				continue
			}
			var data usageReportDayData
			if err := common.UnmarshalJsonStr(raw, &data); err != nil {
				return false, err
			}
			if (row.UserID > 0 && len(data.Customers) > 0) || (row.UserID == 0 && len(data.Upstream) > 0) {
				return true, nil
			}
		}
		if len(rows) < 16 {
			return false, nil
		}
	}
}

// Read only this customer's frozen refund notices from days withheld from usage totals.
func historicalPendingRefunds(ctx context.Context, userID int, generations []string) ([]PublishedUsagePendingRefund, error) {
	var refunds []PublishedUsagePendingRefund
	var rows []usageReportDayRow
	if err := DB.WithContext(ctx).Select("generation", "payload").
		Where("user_id = ? AND generation IN ?", userID, generations).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		raw, err := dailyReportPayload(ctx, DB, row.Generation, reportUsagePayload, userID, row.Payload)
		if err != nil {
			return nil, err
		}
		if raw == "" {
			continue
		}
		var data usageReportDayData
		if err := common.UnmarshalJsonStr(raw, &data); err != nil {
			return nil, err
		}
		refunds = append(refunds, data.PendingRefunds...)
	}
	return refunds, nil
}

func replacesHistoricalReportWithoutSource(stage dailyReportStage, previousSource string, previousRows, previousUpper int64) bool {
	if stage.HistoricalSource != "" || previousSource == "" {
		return false
	}
	if stage.SourceRows == 0 {
		return true
	}
	// Frozen details have line-level verification. A summary alone requires
	// a previously recorded source boundary to prove that coverage was restored.
	return previousSource == dailyReportSourceSummary && (previousRows == 0 || previousUpper == 0)
}
