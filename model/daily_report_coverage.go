package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type dailyReportCoverage struct {
	Rows     int64
	Revision string
}

const dailyReportTupleRevision = "CAST(hashtext(CAST(id AS text) || ':' || CAST(xmin AS text)) AS bigint)"

type dailyReportLogBoundaryKey struct{}

func WithDailyReportLogBoundary(ctx context.Context) (context.Context, error) {
	upper, err := dailyReportLogUpperBound(ctx)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, dailyReportLogBoundaryKey{}, upper), nil
}

func dailyReportLogUpperBound(ctx context.Context) (int64, error) {
	if upper, ok := ctx.Value(dailyReportLogBoundaryKey{}).(int64); ok {
		return upper, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var row struct{ ID int64 }
	err := usageReportLogDB(ctx).WithContext(ctx).Model(&Log{}).Select("id").Order("id DESC").Limit(1).Scan(&row).Error
	return row.ID, err
}

func dailyReportLogScanCoverage(ctx context.Context, count, revision int64) dailyReportCoverage {
	coverage := dailyReportCoverage{Rows: count}
	if usageReportLogDB(ctx).Dialector.Name() == "postgres" {
		coverage.Revision = strconv.FormatInt(revision, 10)
	}
	return coverage
}

// PostgreSQL's tuple version changes on UPDATE, even when the row count
// stays the same. Scope checks to the fixed ID boundary of the current day.
func readDailyReportCoverage(ctx context.Context, query *gorm.DB) (dailyReportCoverage, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var coverage dailyReportCoverage
	columns := "COUNT(*) AS rows"
	if query.Dialector.Name() == "postgres" {
		columns += ", CAST(COALESCE(SUM(" + dailyReportTupleRevision + "), 0) AS text) AS revision"
	}
	err := query.WithContext(ctx).Select(columns).Scan(&coverage).Error
	return coverage, err
}

func dailyReportLogQuery(ctx context.Context, start, end, upper int64, types []int) *gorm.DB {
	return usageReportLogDB(ctx).Model(&Log{}).
		Where("created_at >= ? AND created_at < ? AND id <= ? AND type IN ?", start, end, upper, types)
}

func dailyReportKnownLogCleanup(ctx context.Context, db *gorm.DB, start int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if ShouldTrackBillingStatementRevision() {
		var partial int64
		if err := db.WithContext(ctx).Model(&BillingStatementRetention{}).
			Where("period_start = ? AND status = ?", naturalMonthStartAt(start), BillingStatementRetentionPartial).
			Limit(1).Count(&partial).Error; err != nil {
			return false, err
		}
		if partial > 0 {
			return true, nil
		}
	}
	var cleanups []SystemTask
	err := db.WithContext(ctx).Select("payload").
		Where("type = ? AND status IN ?", SystemTaskTypeLogCleanup, []SystemTaskStatus{SystemTaskStatusRunning, SystemTaskStatusSucceeded, SystemTaskStatusFailed}).
		Order("id DESC").Limit(100).Find(&cleanups).Error
	if err != nil {
		return false, err
	}
	for _, task := range cleanups {
		var scope struct {
			Cutoff int64 `json:"target_timestamp"`
		}
		if common.UnmarshalJsonStr(task.Payload, &scope) == nil && scope.Cutoff > start {
			return true, nil
		}
	}
	return false, nil
}

var errDailyReportSourceChanged = errors.New("daily report source changed during build")
var errDailyReportCoverageDecreased = errors.New("daily report source coverage decreased")

// Dependencies are sampled with ordinary PostgreSQL reads. Filtering by
// the report's date avoids invalidating every report when unrelated traffic
// creates a task or changes a coefficient.
func dailyReportDependencies(ctx context.Context, start int64, kind string) (string, error) {
	db := usageReportMainDB(ctx)
	if db.Dialector.Name() != "postgres" {
		return "", nil
	}
	end := start + billingReportDaySeconds
	month := naturalMonthStartAt(start)
	discounts := db.Model(&ProviderChannelBillingDiscount{}).
		Where("period_start IN ?", []int64{month, previousBillingPeriodStart(month)})
	deliveries := db.Model(&TaskBillingDelivery{})
	var tasks *gorm.DB
	var queries []*gorm.DB
	if kind == DailyReportUsage {
		tasks = db.Model(&Task{}).Where("status IN ? AND finish_time >= ? AND finish_time < ?", TerminalTaskStatuses(), start, end)
		deliveries = deliveries.Where("task_row_id IN (?)", tasks.Session(&gorm.Session{}).Select("id"))
		batches := db.Model(&BatchJob{}).Where(`
			(upstream_status = 'completed' AND completed_at >= ? AND completed_at < ?) OR
			(upstream_status = 'failed' AND failed_at >= ? AND failed_at < ?) OR
			(upstream_status = 'cancelled' AND cancelled_at >= ? AND cancelled_at < ?) OR
			(upstream_status = 'expired' AND expires_at >= ? AND expires_at < ?)`,
			start, end, start, end, start, end, start, end)
		queries = append(queries, batches)
	} else {
		deliveries = deliveries.Where("created_at >= ? AND created_at < ?", start, end)
		tasks = db.Model(&Task{}).Where("id IN (?)", deliveries.Session(&gorm.Session{}).Select("task_row_id"))
	}
	queries = append(queries, tasks, deliveries, discounts)
	var revision string
	for _, query := range queries {
		coverage, err := readDailyReportCoverage(ctx, query.Session(&gorm.Session{}))
		if err != nil {
			return "", err
		}
		revision += fmt.Sprintf("%d:%s;", coverage.Rows, coverage.Revision)
	}
	return revision, nil
}
