package model

import (
	"github.com/QuantumNous/new-api/setting"
	"gorm.io/gorm"
)

// The confirmation caller holds the source-user lock. The same lock protects
// creation of the pending monetary facts, until their source logs are visible.
func verifyArkCacheStatementSource(tx *gorm.DB, userID int, start, end int64) error {
	var rows []ArkPrefixCache
	if err := tx.Scopes(arkCacheExecutionScope).Select("id").Where(`user_id = ? AND
		((created_at >= ? AND created_at < ? AND inference_log_due_at > 0)
		OR (meter_version IN ? AND funds_closed_at = 0 AND expires_at > ? AND
			((next_hour > 0 AND next_hour < ?) OR (state = ? AND created_at > 0 AND created_at < ?))))`,
		userID, start, end, []string{ArkSessionLocalMeterVersion, setting.ArkSessionPlatformStorageMeter}, start, end, ArkCacheInferencePending, end).
		Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != 0 {
		return ErrBillingStatementSourceIncomplete
	}
	var hours []ArkPrefixCacheHour
	if err := tx.Model(&ArkPrefixCacheHour{}).Select("ark_prefix_cache_hours.id").
		Joins("JOIN ark_prefix_caches ON ark_prefix_caches.id = ark_prefix_cache_hours.cache_id").
		Scopes(arkCacheExecutionScope).
		Where("ark_prefix_caches.user_id = ? AND hour_start >= ? AND hour_start < ? AND settled_at > 0 AND delivered_at = 0",
			userID, start, end).Limit(1).Find(&hours).Error; err != nil {
		return err
	}
	if len(hours) != 0 {
		return ErrBillingStatementSourceIncomplete
	}
	return nil
}
