package model

import (
	"context"

	"gorm.io/gorm"
)

func FindArkCacheForCleanup(ctx context.Context, id string) (row ArkPrefixCache, err error) {
	err = DB.WithContext(ctx).Take(&row, "id = ?", id).Error
	return row, err
}

// Absence is enough to stop new references. It cannot backdate a deletion or
// characterize the storage charge between sending DELETE and observing absence.
func ObserveArkCacheAbsent(ctx context.Context, id string, now int64) error {
	result := DB.WithContext(ctx).Model(&ArkPrefixCache{}).Where("id = ? AND state = ?", id, ArkCacheDeleting).
		Updates(map[string]any{"state": ArkCacheAbsent, "absent_at": now, "delete_check_at": 0})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrArkCacheBusy
	}
	return nil
}

func arkCacheDeletionDue(query *gorm.DB, now int64) *gorm.DB {
	return query.Scopes(arkCacheExecutionScope).
		Where("state = ? AND (delete_check_at IS NULL OR (delete_check_at >= 0 AND delete_check_at <= ?))", ArkCacheDeleting, now)
}

func ArkCacheDeletionsDue(ctx context.Context, now int64) (rows []ArkPrefixCache, err error) {
	err = arkCacheDeletionDue(DB.WithContext(ctx).Model(&ArkPrefixCache{}), now).
		Order("delete_check_at, id").Limit(10).Find(&rows).Error
	return rows, err
}

func ClaimArkCacheDeletionCheck(ctx context.Context, id string, now int64) (bool, error) {
	result := arkCacheDeletionDue(DB.WithContext(ctx).Model(&ArkPrefixCache{}).Where("id = ?", id), now).
		UpdateColumn("delete_check_at", now+180)
	return result.RowsAffected == 1, result.Error
}

// StopExpiredArkCacheDeletionCheck ends polling without asserting when the
// provider removed the response or what the final billed lifetime was.
func StopExpiredArkCacheDeletionCheck(ctx context.Context, id string, now int64) error {
	return DB.WithContext(ctx).Model(&ArkPrefixCache{}).
		Where("id = ? AND state = ? AND expires_at > 0 AND expires_at <= ?", id, ArkCacheDeleting, now).
		UpdateColumn("delete_check_at", -1).Error
}

// ArkCacheOutstanding lets workers take over durable work even when this
// process starts without an admission configuration.
func ArkCacheOutstanding(ctx context.Context) bool {
	for _, condition := range []string{"next_action_at > 0", "inference_next_at > 0", "inference_log_due_at > 0"} {
		var rows []ArkPrefixCache
		err := DB.WithContext(ctx).Select("id").Scopes(arkCacheExecutionScope).Where(condition).Limit(1).Find(&rows).Error
		if err != nil || len(rows) != 0 {
			return true
		}
	}
	var deletions []ArkPrefixCache
	if err := DB.WithContext(ctx).Select("id").Scopes(arkCacheExecutionScope).
		Where("(state = ? AND (delete_check_at IS NULL OR delete_check_at >= 0)) OR (state = ? AND (session_round IS NULL OR session_round = 0))",
			ArkCacheDeleting, ArkCacheSending).Limit(1).Find(&deletions).Error; err != nil || len(deletions) != 0 {
		return true
	}
	var hours []ArkPrefixCacheHour
	return DB.WithContext(ctx).Select("id").Scopes(arkCacheHourExecutionScope).
		Where("settled_at > 0 AND delivered_at = 0").Limit(1).Find(&hours).Error != nil || len(hours) != 0
}
