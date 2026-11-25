package model

import (
	"context"

	"gorm.io/gorm"
)

// ArkCacheUnknownAfterSeconds leaves time for the HTTP call and its receipt.
const ArkCacheUnknownAfterSeconds = 180

func ReconcileStaleArkPrefixSends(ctx context.Context, now int64) error {
	stale := func(query *gorm.DB) *gorm.DB {
		return query.Where("state = ? AND (session_round IS NULL OR session_round = 0) AND sent_at <= ?", ArkCacheSending, now-ArkCacheUnknownAfterSeconds)
	}
	var ids []string
	if err := stale(DB.WithContext(ctx).Model(&ArkPrefixCache{})).Order("sent_at, id").Limit(50).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return err
	}
	// Use a fresh statement so the SELECT's clauses cannot reach UPDATE.
	return stale(DB.WithContext(ctx).Model(&ArkPrefixCache{})).Where("id IN ?", ids).Updates(map[string]any{
		"state": ArkCacheUnknown,
		"next_action_at": gorm.Expr(`CASE WHEN held_quota > 0 AND (next_action_at IS NULL OR next_action_at <= 0)
			THEN CASE WHEN funds_deadline > ? THEN funds_deadline ELSE ? END ELSE next_action_at END`, now, now),
	}).Error
}

// The original transactions roll back on failure. Persist retry times
// separately so a conflicting row cannot monopolize a bounded scan.
const arkCacheRetryDelay = 120

func DeferArkPrefixCacheSettlement(ctx context.Context, id string, now int64) error {
	return DB.WithContext(ctx).Model(&ArkPrefixCache{}).
		Where("id = ? AND next_action_at > 0 AND next_action_at <= ?", id, now).
		UpdateColumn("next_action_at", now+arkCacheRetryDelay).Error
}

func DeferArkPrefixCacheLog(ctx context.Context, id int64, now int64) error {
	return DB.WithContext(ctx).Model(&ArkPrefixCacheHour{}).
		Where("id = ? AND settled_at > 0 AND delivered_at = 0 AND (next_retry_at IS NULL OR next_retry_at <= ?)", id, now).
		UpdateColumn("next_retry_at", now+arkCacheRetryDelay).Error
}

func DeferArkSessionInference(ctx context.Context, id string, now int64) error {
	return DB.WithContext(ctx).Model(&ArkPrefixCache{}).
		Where("id = ? AND inference_next_at > 0 AND inference_next_at <= ?", id, now).
		UpdateColumn("inference_next_at", now+arkCacheRetryDelay).Error
}

func DeferArkSessionLog(ctx context.Context, id string, now int64) error {
	return DB.WithContext(ctx).Model(&ArkPrefixCache{}).
		Where("id = ? AND inference_log_due_at > 0 AND inference_log_due_at <= ?", id, now).
		UpdateColumn("inference_log_due_at", now+arkCacheRetryDelay).Error
}
