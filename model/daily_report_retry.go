package model

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dailyReportRetry struct {
	Start          int64  `gorm:"primaryKey;autoIncrement:false"`
	Kind           string `gorm:"primaryKey;size:12"`
	CoverageIssue  bool
	CandidateRows  *int64
	FailureReason  string `gorm:"size:24"`
	ReleaseVersion int
	RetryAt        int64 `gorm:"index"`
}

const (
	DailyReportUsage   = "usage"
	DailyReportBilling = "billing"
)

func dailyReportReleaseVersion(kind string) int {
	if kind == DailyReportUsage {
		return usageReportMeterVersion
	}
	return CustomerOriginalReplayVersion
}

// Compare each parameter with the version column so PostgreSQL infers its numeric type.
const currentDailyReportRetry = "CASE WHEN kind = 'usage' THEN release_version = ? ELSE release_version = ? END"

func dailyReportRetryDates(ctx context.Context, kind string, now int64) (map[int64]bool, error) {
	var dates []int64
	query := usageReportMainDB(ctx).WithContext(ctx).Model(&dailyReportRetry{}).Where("retry_at > ? AND "+currentDailyReportRetry, now, usageReportMeterVersion, CustomerOriginalReplayVersion)
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	err := query.Pluck("start", &dates).Error
	out := make(map[int64]bool, len(dates))
	for _, start := range dates {
		out[start] = true
	}
	return out, err
}

func DeferDailyReportDate(ctx context.Context, start int64, kind string, failure error) error {
	return deferDailyReportDate(usageReportMainDB(ctx).WithContext(ctx), start, kind, failure, nil)
}

func deferDailyReportDate(db *gorm.DB, start int64, kind string, failure error, candidateRows *int64) error {
	delay := 10 * time.Minute
	reason := "failed"
	var timeout net.Error
	switch {
	case errors.Is(failure, ErrDailyReportResourcePressure):
		delay = time.Minute
		reason = "resources"
	case errors.Is(failure, errDailyReportCoverageDecreased):
		delay = 24 * time.Hour
		reason = "coverage"
	case errors.Is(failure, ErrDailyReportDeferred):
		reason = "related_day"
	case errors.Is(failure, context.DeadlineExceeded), errors.As(failure, &timeout) && timeout.Timeout():
		reason = "timeout"
	}
	coverageIssue := errors.Is(failure, errDailyReportCoverageDecreased)
	row := dailyReportRetry{Start: start, Kind: kind, RetryAt: common.GetTimestamp() + int64(delay.Seconds()), CoverageIssue: coverageIssue, ReleaseVersion: dailyReportReleaseVersion(kind), CandidateRows: candidateRows, FailureReason: reason}
	if errors.Is(failure, ErrDailyReportResourcePressure) {
		// A capacity decision must never replace a recorded coverage problem.
		return db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "start"}, {Name: "kind"}},
			DoUpdates: clause.Assignments(map[string]any{
				"retry_at":        gorm.Expr("CASE WHEN daily_report_retries.coverage_issue = ? THEN daily_report_retries.retry_at ELSE ? END", true, row.RetryAt),
				"release_version": gorm.Expr("CASE WHEN daily_report_retries.coverage_issue = ? THEN daily_report_retries.release_version ELSE ? END", true, row.ReleaseVersion),
				"failure_reason":  gorm.Expr("CASE WHEN daily_report_retries.coverage_issue = ? OR (daily_report_retries.release_version = ? AND daily_report_retries.failure_reason <> '') THEN daily_report_retries.failure_reason ELSE ? END", true, row.ReleaseVersion, row.FailureReason),
			}),
		}).Create(&row).Error
	}
	columns := []string{"retry_at", "release_version", "coverage_issue", "candidate_rows", "failure_reason"}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "start"}, {Name: "kind"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&row).Error
}

func dailyReportCoverageIssues(ctx context.Context, kind string, start, end int64) (map[int64]bool, error) {
	var days []int64
	err := usageReportMainDB(ctx).WithContext(ctx).Model(&dailyReportRetry{}).
		Where("kind = ? AND coverage_issue = ? AND release_version = ? AND start >= ? AND start < ?", kind, true, dailyReportReleaseVersion(kind), start, end).Pluck("start", &days).Error
	out := make(map[int64]bool, len(days))
	for _, day := range days {
		out[day] = true
	}
	return out, err
}

// IsDailyReportCoverageIssue identifies a publication rejection whose affected
// date and projection were persisted by the publisher.
func IsDailyReportCoverageIssue(err error) bool {
	return errors.Is(err, errDailyReportCoverageDecreased)
}
