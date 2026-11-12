package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DailyReportScope struct {
	Start int64
	Kind  string
}

type dailyReportStage struct {
	OriginalReplayVersion int
	UsageMeterVersion     int
	Start                 int64  `gorm:"primaryKey;autoIncrement:false"`
	Kind                  string `gorm:"primaryKey;size:12"`
	Generation            string `gorm:"size:32;index"`
	Previous              string `gorm:"size:32"`
	AsOf                  int64
	UpperLogID            int64
	SourceRows            int64
	SourceRevision        string
	Dependencies          string
	Incomplete            bool
	RankReady             bool
}

type dailyReportStageKey struct{}

func StageDailyReport(ctx context.Context) context.Context {
	return context.WithValue(ctx, dailyReportStageKey{}, true)
}

func dailyReportStageOnly(ctx context.Context) bool {
	staged, _ := ctx.Value(dailyReportStageKey{}).(bool)
	return staged
}

// Save a completed projection independently of its publication. The shared
// worker can resume the other projections without rescanning this one.
func saveDailyReportStage(ctx context.Context, db *gorm.DB, stage dailyReportStage, taskID, runnerID string) error {
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lease SystemTaskLock
		if err := lockForUpdate(tx).Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until >= ?",
			UsageReportTaskType, taskID, runnerID, common.GetTimestamp()).Take(&lease).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSystemTaskLockLost
			}
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "start"}, {Name: "kind"}}, UpdateAll: true,
		}).Create(&stage).Error
	}); err != nil {
		return err
	}
	if dailyReportStageOnly(ctx) {
		return nil
	}
	return PublishDailyReportStages(ctx, []DailyReportScope{{Start: stage.Start, Kind: stage.Kind}}, taskID, runnerID)
}

func GetDailyReportStage(ctx context.Context, scope DailyReportScope, requestedAt int64) (bool, error) {
	db := usageReportMainDB(ctx).WithContext(ctx)
	var stage dailyReportStage
	err := db.Where("start = ? AND kind = ?", scope.Start, scope.Kind).Take(&stage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !dailyReportStageVersionCurrent(stage) ||
		time.Since(time.Unix(stage.AsOf, 0)) > 2*time.Hour ||
		(requestedAt > 0 && stage.AsOf <= requestedAt/int64(time.Second)) {
		return false, db.Delete(&stage).Error
	}
	err = checkDailyReportStage(ctx, stage)
	if errors.Is(err, errDailyReportCoverageDecreased) {
		// The publisher owns the decision to preserve a related date.
		return true, nil
	}
	if errors.Is(err, errDailyReportSourceChanged) {
		return false, db.Delete(&stage).Error
	}
	return err == nil, err
}

func dailyReportStageVersionCurrent(stage dailyReportStage) bool {
	return stage.OriginalReplayVersion == CustomerOriginalReplayVersion &&
		(stage.Kind != DailyReportUsage || stage.UsageMeterVersion == usageReportMeterVersion)
}

func checkDailyReportStage(ctx context.Context, stage dailyReportStage) error {
	if !dailyReportStageVersionCurrent(stage) {
		return errDailyReportSourceChanged
	}
	types := []int{LogTypeConsume, LogTypeRefund}
	if stage.Kind == DailyReportUsage {
		types = usageScanLogTypes(usageViewOptions{WantCustomer: true, WantUpstream: true})
	}
	upper := stage.UpperLogID
	if current, ok := ctx.Value(dailyReportLogBoundaryKey{}).(int64); ok {
		upper = current
	}
	query := dailyReportLogQuery(ctx, stage.Start, stage.Start+billingReportDaySeconds, upper, types)
	coverage, err := readDailyReportCoverage(ctx, query)
	if err != nil {
		return err
	}
	if coverage.Rows != stage.SourceRows || coverage.Revision != stage.SourceRevision {
		return errDailyReportSourceChanged
	}
	if err := verifyPreviousDailyReportCoverage(ctx, stage, types); err != nil {
		return err
	}
	dependencies, err := dailyReportDependencies(ctx, stage.Start, stage.Kind)
	if err != nil {
		return err
	}
	if dependencies != stage.Dependencies {
		return errDailyReportSourceChanged
	}
	if stage.Kind == DailyReportBilling && stage.Previous != "" {
		return verifyCustomerOriginalCoverage(ctx, stage.Previous, stage.Generation)
	}
	return nil
}

// Compare against the previous boundary as well. New inserts above that
// boundary can otherwise hide deletion of older facts while totals look full.
func verifyPreviousDailyReportCoverage(ctx context.Context, stage dailyReportStage, types []int) error {
	if stage.Previous == "" {
		return nil
	}
	db := usageReportMainDB(ctx).WithContext(ctx)
	var old struct{ UpperLogID, SourceRows int64 }
	var err error
	if stage.Kind == DailyReportUsage {
		err = db.Model(&usageReportDay{}).Select("upper_log_id, source_rows").
			Where("start = ? AND generation = ?", stage.Start, stage.Previous).Take(&old).Error
	} else {
		err = db.Model(&billingReportGeneration{}).Select("upper_log_id, rows AS source_rows").
			Where("id = ?", stage.Previous).Take(&old).Error
	}
	if err != nil {
		return err
	}
	if old.SourceRows == 0 || old.UpperLogID == 0 || stage.SourceRows < old.SourceRows || stage.UpperLogID <= old.UpperLogID {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var rows int64
	if err := dailyReportLogQuery(ctx, stage.Start, stage.Start+billingReportDaySeconds, old.UpperLogID, types).
		WithContext(checkCtx).Count(&rows).Error; err != nil {
		return err
	}
	if rows < old.SourceRows {
		return errDailyReportCoverageDecreased
	}
	return nil
}

// PublishDailyReportStages changes the complete set of public references in
// one short transaction. Source checks finish before taking the report lease.
func PublishDailyReportStages(ctx context.Context, scopes []DailyReportScope, taskID, runnerID string) error {
	return publishDailyReportStages(ctx, scopes, taskID, runnerID, 0)
}

// PublishScheduledDailyReport preserves a related historical head on verified
// coverage loss, marking it for review in the same commit as the other heads.
func PublishScheduledDailyReport(ctx context.Context, scopes []DailyReportScope, taskID, runnerID string, start int64) error {
	return publishDailyReportStages(ctx, scopes, taskID, runnerID, start)
}

func publishDailyReportStages(ctx context.Context, scopes []DailyReportScope, taskID, runnerID string, primaryStart int64) error {
	db := usageReportMainDB(ctx).WithContext(ctx)
	stages := make([]dailyReportStage, 0, len(scopes))
	coverageLost := make(map[DailyReportScope]bool)
	canPreserve := func(stage dailyReportStage, err error) bool {
		return primaryStart > 0 && stage.Start < primaryStart && stage.Previous != "" &&
			errors.Is(err, errDailyReportCoverageDecreased)
	}
	for _, scope := range scopes {
		var stage dailyReportStage
		if err := db.Where("start = ? AND kind = ?", scope.Start, scope.Kind).Take(&stage).Error; err != nil {
			return err
		}
		if err := checkDailyReportStage(ctx, stage); err != nil {
			if !canPreserve(stage, err) {
				return recordDailyReportCoverageIssue(ctx, stage, err)
			}
			coverageLost[scope] = true
		}
		stages = append(stages, stage)
	}
	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var failed dailyReportStage
	err := db.WithContext(publishCtx).Transaction(func(tx *gorm.DB) error {
		var lease SystemTaskLock
		if err := lockForUpdate(tx).Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until >= ?",
			UsageReportTaskType, taskID, runnerID, common.GetTimestamp()).Take(&lease).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSystemTaskLockLost
			}
			return err
		}
		for _, stage := range stages {
			err := errDailyReportCoverageDecreased
			if !coverageLost[DailyReportScope{Start: stage.Start, Kind: stage.Kind}] {
				err = publishDailyReportStage(tx, stage)
			}
			if canPreserve(stage, err) {
				if err := deferDailyReportDate(tx, stage.Start, stage.Kind, err); err != nil {
					return err
				}
				if err := tx.Delete(&stage).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				failed = stage
				return err
			}
			if err := tx.Delete(&stage).Error; err != nil {
				return err
			}
			if err := tx.Where("start = ? AND kind = ?", stage.Start, stage.Kind).Delete(&dailyReportRetry{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return recordDailyReportCoverageIssue(ctx, failed, err)
}

func recordDailyReportCoverageIssue(ctx context.Context, stage dailyReportStage, err error) error {
	if !errors.Is(err, errDailyReportCoverageDecreased) {
		return err
	}
	retryCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	return errors.Join(err, DeferDailyReportDate(retryCtx, stage.Start, stage.Kind, err))
}

func publishDailyReportStage(tx *gorm.DB, stage dailyReportStage) error {
	if stage.Kind == DailyReportUsage {
		var current usageReportDay
		err := tx.Where("start = ?", stage.Start).Take(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if current.Generation != stage.Previous {
			return errDailyReportSourceChanged
		}
		if current.Generation != "" {
			if current.SourceRows > stage.SourceRows || (stage.Incomplete && !current.Incomplete) {
				return errDailyReportCoverageDecreased
			}
			if err := tx.Model(&usageReportDayRow{}).Where("generation = ? AND user_id = -1", current.Generation).
				Update("retired_at", common.GetTimestamp()).Error; err != nil {
				return err
			}
		}
		head := usageReportDay{Start: stage.Start, Generation: stage.Generation, AsOf: stage.AsOf, SourceRows: stage.SourceRows, UpperLogID: stage.UpperLogID, Incomplete: stage.Incomplete, RankReady: stage.RankReady, MeterVersion: stage.UsageMeterVersion}
		metadata, err := common.Marshal(head)
		if err != nil {
			return err
		}
		if err := tx.Model(&usageReportDayRow{}).Where("generation = ? AND user_id = -1", stage.Generation).
			Update("payload", string(metadata)).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "start"}},
			DoUpdates: clause.AssignmentColumns([]string{"generation", "as_of", "source_rows", "upper_log_id", "incomplete", "rank_ready", "meter_version"})}).Create(&head).Error
	}
	var current billingReportHead
	err := lockForUpdate(tx).Where("start = ?", stage.Start).Take(&current).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if current.Generation != stage.Previous {
		return errDailyReportSourceChanged
	}
	if current.Generation != "" {
		var old billingReportGeneration
		if err := tx.Where("id = ?", current.Generation).Take(&old).Error; err != nil {
			return err
		}
		if old.Rows > stage.SourceRows || (stage.Incomplete && !old.Incomplete) {
			return errDailyReportCoverageDecreased
		}
		if err := tx.Model(&old).Update("retired_at", common.GetTimestamp()).Error; err != nil {
			return err
		}
	}
	if err := tx.Model(&billingReportGeneration{}).Where("id = ?", stage.Generation).
		Updates(map[string]any{"published": true, "rows": stage.SourceRows}).Error; err != nil {
		return err
	}
	head := billingReportHead{Start: stage.Start, Generation: stage.Generation, AsOf: stage.AsOf}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "start"}},
		DoUpdates: clause.AssignmentColumns([]string{"generation", "as_of"})}).Create(&head).Error
}

func MigrateDailyReports(db *gorm.DB) error {
	return db.AutoMigrate(&usageReportDay{}, &usageReportDayRow{}, &usageReportRank{}, &dailyReportRetry{}, &dailyReportExportReference{}, &dailyReportStage{}, &dailyReportPayloadPart{},
		&billingReportHead{}, &billingReportGeneration{}, &billingReportCustomerDay{}, &billingReportUpstreamDay{}, &billingReportLine{}, &billingReportRebuild{}, &billingReportSourceCursor{})
}
