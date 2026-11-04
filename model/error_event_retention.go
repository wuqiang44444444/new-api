package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
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
