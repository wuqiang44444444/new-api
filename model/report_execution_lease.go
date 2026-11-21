package model

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

const reportExecutionLeaseType = "report_database_budget"

// RenewReportExecutionLease coordinates report consumers sharing the task
// database. It uses existing background lease storage and never writes logs.
func RenewReportExecutionLease(ctx context.Context, owner string, now time.Time, ttl time.Duration) (bool, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	until := now.Add(ttl).Unix()
	result := db.Model(&SystemTaskLock{}).
		Where("type = ? AND (locked_by = ? OR locked_until < ?)", reportExecutionLeaseType, owner, now.Unix()).
		Updates(map[string]any{"locked_by": owner, "locked_until": until, "updated_at": now.Unix()})
	if result.Error != nil || result.RowsAffected != 0 {
		return result.RowsAffected != 0, result.Error
	}
	lease := SystemTaskLock{Type: reportExecutionLeaseType, TaskID: reportExecutionLeaseType,
		LockedBy: owner, LockedUntil: until, UpdatedAt: now.Unix()}
	result = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&lease)
	if result.Error != nil {
		return false, result.Error
	}
	// MySQL can report a matched no-op upsert as affected. Check the
	// holder before granting work on the insert/conflict path.
	var held int64
	err := db.Model(&SystemTaskLock{}).Where("type = ? AND locked_by = ? AND locked_until >= ?",
		reportExecutionLeaseType, owner, until).Count(&held).Error
	return held != 0, err
}
