package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

// DeleteOldErrorEventBatch only removes diagnostics, never consume/refund logs
// or request evidence. Selecting IDs avoids dialect-dependent DELETE LIMIT.
func DeleteOldErrorEventBatch(ctx context.Context, cutoff int64, limit int) (int64, error) {
	if cutoff <= 0 || limit < 1 || limit > 1000 {
		return 0, errors.New("invalid error event cleanup scope")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		var count int64
		if err := LOG_DB.WithContext(ctx).Model(&ErrorEvent{}).Where("created_at < ?", cutoff).Count(&count).Error; err != nil || count == 0 {
			return 0, err
		}
		err := LOG_DB.WithContext(ctx).Exec("ALTER TABLE error_events DELETE WHERE created_at < ? SETTINGS mutations_sync = 1", cutoff).Error
		if err != nil {
			return 0, err
		}
		return count, nil
	}
	var ids []int
	if err := LOG_DB.WithContext(ctx).Model(&ErrorEvent{}).Where("created_at < ?", cutoff).
		Order("created_at ASC, id ASC").Limit(limit).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return 0, err
	}
	result := LOG_DB.WithContext(ctx).Where("id IN ? AND created_at < ?", ids, cutoff).Delete(&ErrorEvent{})
	return result.RowsAffected, result.Error
}

// RecordErrorEventRetentionCutoff commits the coverage boundary before cleanup.
// The log store can be separate from the primary DB. Recording the intended
// deletion first makes a crash or partial delete conservative for backfill.
func RecordErrorEventRetentionCutoff(ctx context.Context, cutoff int64) error {
	if cutoff <= 0 {
		return errors.New("invalid error event retention cutoff")
	}
	db := DB.WithContext(ctx)
	schedule := ErrorReportSchedule{Scope: ErrorReportScheduleScopeGlobal}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&schedule).Error; err != nil {
		return err
	}
	return db.Model(&ErrorReportSchedule{}).
		Where("scope = ? AND error_retention_cutoff < ?", ErrorReportScheduleScopeGlobal, cutoff).
		UpdateColumn("error_retention_cutoff", cutoff).Error
}

func ReadErrorEventRetentionCutoff(ctx context.Context) (int64, error) {
	var schedule ErrorReportSchedule
	err := DB.WithContext(ctx).Select("error_retention_cutoff").
		Where("scope = ?", ErrorReportScheduleScopeGlobal).Find(&schedule).Error
	return schedule.ErrorRetentionCutoff, err
}
