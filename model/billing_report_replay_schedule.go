package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The existing source cursor table stores the append cursor at ID 1. The
// replay cursor advances over retained calendar days once per resolver version.
const billingReportReplayCursorID = 2

// DiscoverCustomerOriginalReplayDays runs under the report worker's existing
// lease. It schedules one historical day at a time and saves progress with the
// queue insert, so neither restart nor a failed publication loses the date.
// The result tells the local worker whether discovery can continue immediately.
func DiscoverCustomerOriginalReplayDays(ctx context.Context, today, now int64) (bool, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	var ready []int64
	if err := db.Model(&billingReportRebuild{}).
		Where("NOT EXISTS (SELECT 1 FROM daily_report_retries retry WHERE retry.start = billing_report_rebuilds.start AND retry.retry_at > ?)", now).
		Limit(1).Pluck("start", &ready).Error; err != nil || len(ready) > 0 {
		return len(ready) > 0, err
	}
	cursor := billingReportSourceCursor{ID: billingReportReplayCursorID}
	err := db.Where("id = ?", cursor.ID).Take(&cursor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cursor.OriginalReplayVersion, cursor.ReplayBefore = CustomerOriginalReplayVersion, today
		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&cursor)
		if result.Error != nil || result.RowsAffected != 1 {
			return false, result.Error
		}
	} else if err != nil {
		return false, err
	}
	if cursor.OriginalReplayVersion > CustomerOriginalReplayVersion {
		return false, nil
	}

	before := cursor.ReplayBefore
	if cursor.OriginalReplayVersion != CustomerOriginalReplayVersion {
		before = today
	}
	if before == 0 {
		return false, nil
	}
	var candidate int64
	logs := usageReportLogDB(ctx).WithContext(ctx)
	// Limit the discovery work independently of the size of the retention window.
	for range 4 {
		var at int64
		if err := logs.Model(&Log{}).Select("created_at").
			Where("created_at > 0 AND created_at < ? AND type IN ?", before, []int{LogTypeConsume, LogTypeRefund}).
			Order("created_at DESC").Limit(1).Scan(&at).Error; err != nil {
			return false, err
		}
		if at == 0 {
			before = 0
			break
		}
		date := time.Unix(at, 0).In(UsageAnalyticsZone())
		before = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location()).Unix()
		var publishedVersion int
		if err := db.Table("billing_report_heads AS h").
			Select("COALESCE(g.original_replay_version, 0)").
			Joins("JOIN billing_report_generations g ON g.id = h.generation").
			Where("h.start = ?", before).Scan(&publishedVersion).Error; err != nil {
			return false, err
		}
		if publishedVersion < CustomerOriginalReplayVersion {
			candidate = before
			break
		}
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&cursor).
			Where("original_replay_version = ? AND replay_before = ?", cursor.OriginalReplayVersion, cursor.ReplayBefore).
			Updates(map[string]any{"original_replay_version": CustomerOriginalReplayVersion, "replay_before": before})
		if result.Error != nil || result.RowsAffected != 1 || candidate == 0 {
			return result.Error
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&billingReportRebuild{Start: candidate, RequestedAt: time.Now().UnixNano(), OriginalReplayVersion: CustomerOriginalReplayVersion}).Error
	})
	return before != 0, err
}
