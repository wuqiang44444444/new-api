package model

import (
	"context"
	"strings"

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
	types := []int{LogTypeConsume, LogTypeRefund}
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
		types = usageScanLogTypes(usageViewOptions{WantCustomer: true, WantUpstream: true})
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
	query := dailyReportLogQuery(ctx, stage.Start, stage.Start+billingReportDaySeconds, upper, types)
	var first int64
	err = query.Session(&gorm.Session{}).WithContext(ctx).Select("id").Limit(1).Scan(&first).Error
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
			err := db.Model(&billingReportLine{}).Where("generation = ?", billing.ID).
				Select("COUNT(*) AS rows, COALESCE(MAX(source_id), 0) AS upper").Scan(&boundary).Error
			if err != nil {
				return false, err
			}
			priorRows, priorUpper = boundary.Rows, boundary.Upper
		}
		if priorRows > 0 && priorUpper > 0 {
			priorCoverage, err := readDailyReportCoverage(ctx, query.Session(&gorm.Session{}).Where("id <= ?", priorUpper))
			if err != nil || priorCoverage.Rows >= priorRows {
				return false, err
			}
		} else if usage.HistoricalSource != dailyReportSourceSummary && billing.HistoricalSource != dailyReportSourceSummary {
			return false, nil
		}
		coverage, err = readDailyReportCoverage(ctx, query)
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
	err = db.Transaction(func(tx *gorm.DB) error {
		if scope.Kind == DailyReportUsage {
			hasSummary, err := historicalUsageHasSummary(ctx, tx, stage.Previous)
			if err != nil {
				return err
			}
			if hasSummary {
				stage.HistoricalSource = dailyReportSourceSummary
			}
			// Pending refunds remain readable even when this day has no usage evidence.
			if _, err := copyDailyReportProjection(tx, &usageReportDayRow{}, stage, true); err != nil {
				return err
			}
			if _, err := copyDailyReportProjection(tx, &dailyReportPayloadPart{}, stage, false); err != nil {
				return err
			}
			return tx.Create(&usageReportDayRow{Generation: stage.Generation, UserID: -1, CreatedAt: common.GetTimestamp()}).Error
		}
		details, err := copyDailyReportProjection(tx, &billingReportLine{}, stage, false)
		if err != nil {
			return err
		}
		customer, err := copyDailyReportProjection(tx, &billingReportCustomerDay{}, stage, false)
		if err != nil {
			return err
		}
		upstream, err := copyDailyReportProjection(tx, &billingReportUpstreamDay{}, stage, false)
		if err != nil {
			return err
		}
		if details > 0 {
			stage.HistoricalSource = dailyReportSourceDetails
		} else if customer > 0 || upstream > 0 {
			stage.HistoricalSource = dailyReportSourceSummary
		}
		if _, err := copyDailyReportProjection(tx, &dailyReportPayloadPart{}, stage, false); err != nil {
			return err
		}
		if details > 0 {
			if err := rebuildMissingHistoricalBillingCustomers(ctx, tx, stage.Generation); err != nil {
				return err
			}
		}
		return tx.Create(&billingReportGeneration{ID: stage.Generation, Start: stage.Start,
			OriginalReplayVersion: stage.OriginalReplayVersion, AsOf: stage.AsOf, UpperLogID: upper,
			CreatedAt: common.GetTimestamp(), Incomplete: true, HistoricalSource: stage.HistoricalSource}).Error
	})
	if err != nil {
		return false, err
	}
	return true, saveDailyReportStage(ctx, db, stage, taskID, runnerID)
}

func historicalUsageHasSummary(ctx context.Context, tx *gorm.DB, generation string) (bool, error) {
	// Older generations may have only detail leaves. A customer row can also
	// contain just pending refunds, and the global row may contain only discounts.
	for after := -1; ; {
		var rows []usageReportDayRow
		if err := tx.Select("user_id", "overview", "payload").Where("generation = ? AND user_id > ?", generation, after).
			Order("user_id").Limit(16).Find(&rows).Error; err != nil {
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
			raw, err := dailyReportPayload(ctx, tx, generation, reportUsagePayload, row.UserID, row.Payload)
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

// Copy existing immutable evidence without unmarshaling it or deriving facts
// from current configuration. The schema and predicate are code-owned.
func copyDailyReportProjection(tx *gorm.DB, projection any, stage dailyReportStage, excludeUsageMarker bool) (int64, error) {
	statement := &gorm.Statement{DB: tx}
	if err := statement.Parse(projection); err != nil {
		return 0, err
	}
	columns := make([]string, 0, len(statement.Schema.DBNames))
	values := make([]string, 0, len(statement.Schema.DBNames))
	for _, name := range statement.Schema.DBNames {
		quoted := statement.Quote(name)
		columns = append(columns, quoted)
		if name == "generation" {
			quoted = "?"
		}
		values = append(values, quoted)
	}
	table := statement.Quote(statement.Schema.Table)
	query := "INSERT INTO " + table + " (" + strings.Join(columns, ", ") + ") SELECT " +
		strings.Join(values, ", ") + " FROM " + table + " WHERE generation = ?"
	if excludeUsageMarker {
		query += " AND user_id <> -1"
	}
	result := tx.Exec(query, stage.Generation, stage.Previous)
	return result.RowsAffected, result.Error
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
