package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type billingReportRebuild struct {
	Start                 int64 `gorm:"primaryKey;autoIncrement:false"`
	RequestedAt           int64
	OriginalReplayVersion int
}

// RequestBillingReportRebuild records the date, not a new worker. The normal
// scheduler owns pressure checks, source reads and publication for this scope.
func RequestBillingReportRebuild(ctx context.Context, date string) error {
	day, err := time.ParseInLocation(time.DateOnly, date, UsageAnalyticsZone())
	if err != nil || day.Unix() <= 0 || day.After(time.Now()) {
		return common.NewMessage("Invalid billing report date")
	}
	return upsertBillingReportRebuilds(DB.WithContext(ctx), []billingReportRebuild{{Start: day.Unix(), RequestedAt: time.Now().UnixNano()}})
}

func upsertBillingReportRebuilds(tx *gorm.DB, rows []billingReportRebuild) error {
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "start"}}, DoUpdates: clause.AssignmentColumns([]string{"requested_at", "original_replay_version"}),
	}).CreateInBatches(rows, 100).Error
}

func CompleteBillingReportRebuild(ctx context.Context, date, revision int64) error {
	return usageReportMainDB(ctx).WithContext(ctx).Where("start = ? AND requested_at = ?", date, revision).Delete(&billingReportRebuild{}).Error
}

func PruneBillingReportGenerations(ctx context.Context, now int64) (bool, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	cutoff := now - 8*billingReportDaySeconds
	var row billingReportGeneration
	err := db.Where("(retired_at > 0 AND retired_at < ?) OR (published = ? AND created_at < ? AND retired_at < ?)", cutoff, false, cutoff, now-60).
		Where("NOT EXISTS (SELECT 1 FROM daily_report_stages stage WHERE stage.generation = billing_report_generations.id AND stage.as_of >= ?)", cutoff).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Retire the reference before dropping any of its data, so a stale pinned
	// request fails closed instead of reading a partially cleaned generation.
	if row.Published {
		return false, db.Transaction(func(tx *gorm.DB) error {
			if err := lockForUpdate(tx).Where("id = ?", row.ID).Take(&row).Error; err != nil {
				return err
			}
			var active int64
			if err := activeDailyReportExports(tx, row.ID).Count(&active).Error; err != nil {
				return err
			}
			if active > 0 {
				return nil
			}
			return tx.Model(&row).Updates(map[string]any{"published": false, "retired_at": now}).Error
		})
	}
	var ids []int64
	if err := db.Model(&billingReportLine{}).Where("generation = ?", row.ID).Limit(500).Pluck("source_id", &ids).Error; err != nil {
		return false, err
	}
	if len(ids) > 0 {
		return true, db.Where("generation = ? AND source_id IN ?", row.ID, ids).Delete(&billingReportLine{}).Error
	}
	for _, table := range []struct {
		model any
		key   string
	}{
		{&billingReportCustomerDay{}, "user_id"}, {&billingReportUpstreamDay{}, "channel_id"},
	} {
		var ids []int
		if err := db.Model(table.model).Where("generation = ?", row.ID).Limit(200).Pluck(table.key, &ids).Error; err != nil {
			return false, err
		}
		if len(ids) > 0 {
			return true, db.Where("generation = ? AND "+table.key+" IN ?", row.ID, ids).Delete(table.model).Error
		}
	}
	if pruned, err := pruneDailyReportPayload(ctx, db, row.ID); err != nil || pruned {
		return pruned, err
	}
	return true, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("generation = ?", row.ID).Delete(&dailyReportExportReference{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

// A bounded append cursor discovers historical rows even when their created_at
// predates the periodic lookback. The small overlap covers out-of-order commits.
type billingReportSourceCursor struct {
	ID                    int `gorm:"primaryKey;autoIncrement:false"`
	UpperLogID            int64
	OriginalReplayVersion int
	ReplayBefore          int64
}

func DiscoverLateBillingReportDays(ctx context.Context, today int64) error {
	db := usageReportMainDB(ctx).WithContext(ctx)
	logDB := usageReportLogDB(ctx).WithContext(ctx)
	var upper int64
	if err := logDB.Model(&Log{}).Select("id").Order("id DESC").Limit(1).Scan(&upper).Error; err != nil {
		return err
	}
	cursor := billingReportSourceCursor{ID: 1}
	err := db.Where("id = ?", 1).Take(&cursor).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var previous int64
	if err == nil {
		previous = max(0, cursor.UpperLogID-1000)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&billingReportSourceCursor{ID: 1, UpperLogID: upper}).Error
	}
	var logs []struct{ ID, CreatedAt int64 }
	if err := logDB.Model(&Log{}).Select("id, created_at").
		Where("id > ? AND id <= ? AND created_at < ? AND type IN ?", previous, upper, today, []int{LogTypeConsume, LogTypeRefund}).
		Order("id ASC").Limit(2000).Find(&logs).Error; err != nil {
		return err
	}
	byDay := make(map[int64][]int64)
	for _, row := range logs {
		at := time.Unix(row.CreatedAt, 0).In(UsageAnalyticsZone())
		day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, at.Location()).Unix()
		byDay[day] = append(byDay[day], row.ID)
	}
	dirty := make([]billingReportRebuild, 0, len(byDay))
	for start, ids := range byDay {
		var head billingReportHead
		if err := db.Where("start = ?", start).Limit(1).Find(&head).Error; err != nil {
			return err
		}
		if head.Generation == "" {
			continue
		}
		var included int64
		if err := db.Model(&billingReportLine{}).Where("generation = ? AND source_id IN ?", head.Generation, ids).Count(&included).Error; err != nil {
			return err
		}
		if included < int64(len(ids)) {
			dirty = append(dirty, billingReportRebuild{Start: start, RequestedAt: time.Now().UnixNano()})
		}
	}
	if len(logs) == 2000 {
		upper = logs[len(logs)-1].ID
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if len(dirty) > 0 {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "start"}}, DoNothing: true}).CreateInBatches(dirty, 100).Error; err != nil {
				return err
			}
		}
		return tx.Model(&cursor).Update("upper_log_id", max(cursor.UpperLogID, upper)).Error
	})
}
