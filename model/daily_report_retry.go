package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type dailyReportRetry struct {
	Start         int64  `gorm:"primaryKey;autoIncrement:false"`
	Kind          string `gorm:"primaryKey;size:12"`
	CoverageIssue bool
	RetryAt       int64 `gorm:"index"`
}

const (
	DailyReportUsage   = "usage"
	DailyReportBilling = "billing"
)

func dailyReportRetryDates(ctx context.Context, kind string, now int64) (map[int64]bool, error) {
	var dates []int64
	query := usageReportMainDB(ctx).WithContext(ctx).Model(&dailyReportRetry{}).Where("retry_at > ?", now)
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
	return deferDailyReportDate(usageReportMainDB(ctx).WithContext(ctx), start, kind, failure)
}

func deferDailyReportDate(db *gorm.DB, start int64, kind string, failure error) error {
	delay := 10 * time.Minute
	if errors.Is(failure, errDailyReportCoverageDecreased) {
		delay = 24 * time.Hour
	}
	coverageIssue := errors.Is(failure, errDailyReportCoverageDecreased)
	row := dailyReportRetry{Start: start, Kind: kind, RetryAt: common.GetTimestamp() + int64(delay.Seconds()), CoverageIssue: coverageIssue}
	columns := []string{"retry_at"}
	if coverageIssue {
		columns = append(columns, "coverage_issue")
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "start"}, {Name: "kind"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&row).Error
}

func dailyReportCoverageIssues(ctx context.Context, kind string, start, end int64) (map[int64]bool, error) {
	var days []int64
	err := usageReportMainDB(ctx).WithContext(ctx).Model(&dailyReportRetry{}).
		Where("kind = ? AND coverage_issue = ? AND start >= ? AND start < ?", kind, true, start, end).Pluck("start", &days).Error
	out := make(map[int64]bool, len(days))
	for _, day := range days {
		out[day] = true
	}
	return out, err
}
