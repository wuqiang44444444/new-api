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
	IssueRows             *int64
	IssueRowsGeneration   string `gorm:"size:32"`
	Incomplete            bool
	RankReady             bool

	HistoricalSource       string `gorm:"size:24"`
	HistoricalMeterVersion int
	CopyingGeneration      string `gorm:"size:32"`
	CopyingStartedAt       int64
	CopyingReleaseVersion  int
	HistoricalCopiesDone   bool
	BillingBuildVersion    int
	BillingBuildRows       int64
	BillingBuildDigest     string `gorm:"size:64"`
	BillingBuildSummaries  bool
}

func (stage dailyReportStage) copyingHistory() bool {
	return stage.unfinished() && stage.BillingBuildVersion == 0
}

func (stage dailyReportStage) unfinished() bool {
	return stage.Generation != "" && stage.CopyingGeneration == stage.Generation
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
	if err := loadDailyReportIssueRows(ctx, &stage); err != nil {
		return err
	}
	stage.CopyingGeneration = ""
	stage.CopyingStartedAt = 0
	stage.CopyingReleaseVersion = 0
	stage.HistoricalCopiesDone = false
	if err := persistDailyReportStage(ctx, db, stage, taskID, runnerID, nil); err != nil {
		return err
	}
	if dailyReportStageOnly(ctx) {
		return nil
	}
	return PublishDailyReportStages(ctx, []DailyReportScope{{Start: stage.Start, Kind: stage.Kind}}, taskID, runnerID)
}

func persistDailyReportStage(ctx context.Context, db *gorm.DB, stage dailyReportStage, taskID, runnerID string, initialize any) error {
	if err := usageReportBeforeBatch(ctx); err != nil {
		return err
	}
	if stage.unfinished() {
		// A worker without checkpoint support must reject an unfinished
		// stage even if it can take over the lease after a rollback.
		if stage.copyingHistory() {
			stage.CopyingReleaseVersion = dailyReportReleaseVersion(stage.Kind)
		}
		stage.OriginalReplayVersion, stage.UsageMeterVersion = 0, 0
	}
	return dailyReportStageTransaction(ctx, db, taskID, runnerID, func(tx *gorm.DB) error {
		if initialize != nil {
			if err := tx.Create(initialize).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "start"}, {Name: "kind"}}, UpdateAll: true,
		}).Create(&stage).Error
	})
}

func dailyReportStageTransaction(ctx context.Context, db *gorm.DB, taskID, runnerID string, write func(*gorm.DB) error) error {
	writeCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
	defer cancel()
	return db.WithContext(writeCtx).Transaction(func(tx *gorm.DB) error {
		var lease SystemTaskLock
		if err := lockForUpdate(tx).Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until >= ?",
			UsageReportTaskType, taskID, runnerID, common.GetTimestamp()).Take(&lease).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSystemTaskLockLost
			}
			return err
		}
		return write(tx)
	})
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
	keep := 2 * time.Hour
	if lowPriorityDailyReport(ctx) {
		// The paired projection may need several quiet windows to finish.
		keep = 48 * time.Hour
	}
	expired := time.Since(time.Unix(stage.AsOf, 0)) > keep
	superseded := requestedAt > 0 && stage.AsOf <= requestedAt/int64(time.Second)
	if stage.unfinished() {
		expired = common.GetTimestamp()-stage.AsOf >= 8*billingReportDaySeconds
		superseded = requestedAt > stage.CopyingStartedAt
	}
	if !dailyReportStageVersionCurrent(stage) || stage.AsOf < stage.Start+billingReportDaySeconds+dailyReportCloseDelay || expired || superseded {
		return false, db.Where("generation = ?", stage.Generation).Delete(&stage).Error
	}
	if stage.unfinished() {
		return false, nil
	}
	checks, err := readDailyReportStageLogChecks(ctx, []dailyReportStage{stage})
	if err == nil {
		err = checkDailyReportStageSource(stage, checks[0])
	}
	if err == nil {
		err = checkDailyReportStageDependencies(ctx, stage)
	}
	if errors.Is(err, errDailyReportCoverageDecreased) {
		// The publisher owns the decision to preserve a related date.
		return true, nil
	}
	if errors.Is(err, errDailyReportSourceChanged) {
		return false, db.Where("generation = ?", stage.Generation).Delete(&stage).Error
	}
	return err == nil, err
}

func dailyReportStageVersionCurrent(stage dailyReportStage) bool {
	if stage.unfinished() && stage.BillingBuildVersion != 0 {
		return stage.Kind == DailyReportBilling && stage.BillingBuildVersion == billingReportCheckpointVersion
	}
	if stage.unfinished() {
		return stage.CopyingReleaseVersion == dailyReportReleaseVersion(stage.Kind)
	}
	if stage.Kind == DailyReportUsage {
		return stage.UsageMeterVersion == usageReportMeterVersion
	}
	return stage.OriginalReplayVersion == CustomerOriginalReplayVersion
}

func checkDailyReportStageSource(stage dailyReportStage, logCheck dailyReportStageLogCheck) error {
	if logCheck.current.Rows != stage.SourceRows || logCheck.current.Revision != stage.SourceRevision {
		return errDailyReportSourceChanged
	}
	if logCheck.previousRows < logCheck.previous.rows {
		return errDailyReportCoverageDecreased
	}
	return nil
}

func checkDailyReportStageDependencies(ctx context.Context, stage dailyReportStage) error {
	dependencies, err := dailyReportDependencies(ctx, stage.Start, stage.Kind)
	if err != nil {
		return err
	}
	if dependencies != stage.Dependencies {
		return errDailyReportSourceChanged
	}
	return nil
}

// Compare against the previous boundary as well. New inserts above that
// boundary can otherwise hide deletion of older facts while totals look full.
func previousDailyReportBoundary(ctx context.Context, stage dailyReportStage) (dailyReportPreviousBoundary, error) {
	if stage.Previous == "" {
		return dailyReportPreviousBoundary{}, nil
	}
	db := usageReportMainDB(ctx).WithContext(ctx)
	var old struct {
		UpperLogID, SourceRows int64
		HistoricalSource       string
	}
	var err error
	if stage.Kind == DailyReportUsage {
		err = db.Model(&usageReportDay{}).Select("upper_log_id, source_rows, historical_source").
			Where("start = ? AND generation = ?", stage.Start, stage.Previous).Take(&old).Error
	} else {
		err = db.Model(&billingReportGeneration{}).Select("upper_log_id, rows AS source_rows, historical_source").
			Where("id = ?", stage.Previous).Take(&old).Error
	}
	if err != nil {
		return dailyReportPreviousBoundary{}, err
	}
	if stage.HistoricalSource != "" || old.SourceRows == 0 || old.UpperLogID == 0 ||
		(old.HistoricalSource == "" && (stage.SourceRows < old.SourceRows || stage.UpperLogID <= old.UpperLogID)) {
		return dailyReportPreviousBoundary{}, nil
	}
	return dailyReportPreviousBoundary{upper: old.UpperLogID, rows: old.SourceRows}, nil
}

// PublishDailyReportStages changes the complete set of public references in
// one short transaction. Source checks finish before taking the report lease.
func PublishDailyReportStages(ctx context.Context, scopes []DailyReportScope, taskID, runnerID string) error {
	return publishDailyReportStages(ctx, scopes, taskID, runnerID, 0, nil)
}

// PublishScheduledDailyReport preserves a related historical head on verified
// coverage loss, marking it for review in the same commit as the other heads.
func PublishScheduledDailyReport(ctx context.Context, scopes []DailyReportScope, taskID, runnerID string, start int64, revisions map[int64]int64) error {
	return publishDailyReportStages(ctx, scopes, taskID, runnerID, start, revisions)
}

func publishDailyReportStages(ctx context.Context, scopes []DailyReportScope, taskID, runnerID string, primaryStart int64, revisions map[int64]int64) error {
	db := usageReportMainDB(ctx).WithContext(ctx)
	stages := make([]dailyReportStage, 0, len(scopes))
	coverageLost := make(map[DailyReportScope]bool)
	canPreserve := func(stage dailyReportStage, err error) bool {
		return primaryStart > 0 && stage.Start < primaryStart && stage.Previous != "" &&
			errors.Is(err, errDailyReportCoverageDecreased)
	}
	for _, scope := range scopes {
		var stage dailyReportStage
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		readCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
		err := db.WithContext(readCtx).Where("start = ? AND kind = ?", scope.Start, scope.Kind).Take(&stage).Error
		cancel()
		if err != nil {
			return err
		}
		stages = append(stages, stage)
	}
	originalChecks := make([]error, len(stages))
	for i := range stages {
		stage := &stages[i]
		if stage.unfinished() || !dailyReportStageVersionCurrent(*stage) {
			return errDailyReportSourceChanged
		}
		// Stage reuse only establishes that the candidate is worth retaining.
		// Finish the immutable checks before sampling volatile source coverage.
		if stage.Kind == DailyReportBilling && stage.Previous != "" {
			originalChecks[i] = verifyCustomerOriginalCoverage(ctx, stage.Previous, stage.Generation)
			if err := originalChecks[i]; err != nil && !errors.Is(err, errDailyReportCoverageDecreased) {
				return err
			}
		}
		if originalChecks[i] == nil {
			if err := loadDailyReportIssueRows(ctx, stage); err != nil {
				return err
			}
		}
	}
	// The mutable verification phase never waits for capacity. If any batch
	// yields, the next publication attempt takes fresh witnesses for all dates.
	ctx = context.WithValue(ctx, dailyReportFinalValidationKey{}, true)
	if primaryStart != 0 && lowPriorityDailyReport(ctx) {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return err
		}
		upper, err := readDailyReportLogUpperBound(ctx)
		if err != nil {
			return err
		}
		// Scheduled scopes are closed dates. Check arrivals while a candidate was parked.
		ctx = context.WithValue(ctx, dailyReportLogBoundaryKey{}, upper)
	}
	dependencyChecks := make([]error, len(stages))
	for i, stage := range stages {
		dependencyChecks[i] = checkDailyReportStageDependencies(ctx, stage)
		if err := dependencyChecks[i]; err != nil && !errors.Is(err, errDailyReportSourceChanged) {
			return err
		}
	}
	checks, err := readDailyReportStageLogChecks(ctx, stages)
	if err != nil {
		return err
	}
	for i, stage := range stages {
		err := checkDailyReportStageSource(stage, checks[i])
		if err == nil {
			err = dependencyChecks[i]
		}
		if err == nil {
			err = originalChecks[i]
		}
		if err != nil {
			if !canPreserve(stage, err) {
				return recordDailyReportCoverageIssue(ctx, stage, err)
			}
			coverageLost[DailyReportScope{Start: stage.Start, Kind: stage.Kind}] = true
		}
	}
	if err := usageReportBeforeBatch(ctx); err != nil {
		return err
	}
	var failed dailyReportStage
	err = executeDailyReportBatch(ctx, func(ctx context.Context) error {
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return db.WithContext(publishCtx).Transaction(func(tx *gorm.DB) error {
			var lease SystemTaskLock
			if err := lockForUpdate(tx).Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until >= ?",
				UsageReportTaskType, taskID, runnerID, common.GetTimestamp()).Take(&lease).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrSystemTaskLockLost
				}
				return err
			}
			published := make(map[int64]int, len(revisions))
			for _, stage := range stages {
				removed := tx.Where("generation = ?", stage.Generation).Delete(&stage)
				if removed.Error != nil {
					return removed.Error
				}
				if removed.RowsAffected != 1 {
					return errDailyReportSourceChanged
				}
				err := errDailyReportCoverageDecreased
				if !coverageLost[DailyReportScope{Start: stage.Start, Kind: stage.Kind}] {
					err = publishDailyReportStage(tx, stage)
				}
				if canPreserve(stage, err) {
					if err := deferDailyReportDate(tx, stage.Start, stage.Kind, err, &stage.SourceRows); err != nil {
						return err
					}
					continue
				}
				if err != nil {
					failed = stage
					return err
				}
				if err := tx.Where("start = ? AND kind = ?", stage.Start, stage.Kind).Delete(&dailyReportRetry{}).Error; err != nil {
					return err
				}
				if revision := revisions[stage.Start]; revision > 0 {
					published[stage.Start]++
					if published[stage.Start] == 2 {
						if err := tx.Where("start = ? AND requested_at = ?", stage.Start, revision).Delete(&billingReportRebuild{}).Error; err != nil {
							return err
						}
					}
				}
			}
			return nil
		})
	})
	return recordDailyReportCoverageIssue(ctx, failed, err)
}

func recordDailyReportCoverageIssue(ctx context.Context, stage dailyReportStage, err error) error {
	if !errors.Is(err, errDailyReportCoverageDecreased) {
		return err
	}
	retryCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer stop()
	return errors.Join(err, deferDailyReportDate(usageReportMainDB(retryCtx).WithContext(retryCtx), stage.Start, stage.Kind, err, &stage.SourceRows))
}

func publishDailyReportStage(tx *gorm.DB, stage dailyReportStage) error {
	if stage.Kind == DailyReportUsage {
		var current usageReportDay
		err := tx.Where("start = ?", stage.Start).Take(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if current.MeterVersion > stage.UsageMeterVersion || current.Generation != stage.Previous {
			return errDailyReportSourceChanged
		}
		if current.Generation != "" {
			if replacesHistoricalReportWithoutSource(stage, current.HistoricalSource, current.SourceRows, current.UpperLogID) {
				return errDailyReportCoverageDecreased
			}
			if stage.HistoricalSource == "" && (current.SourceRows > stage.SourceRows || (stage.Incomplete && !current.Incomplete)) {
				return errDailyReportCoverageDecreased
			}
			if err := tx.Model(&usageReportDayRow{}).Where("generation = ? AND user_id = -1", current.Generation).
				Update("retired_at", common.GetTimestamp()).Error; err != nil {
				return err
			}
		}
		head := usageReportDay{HistoricalSource: stage.HistoricalSource, HistoricalMeterVersion: stage.HistoricalMeterVersion, Start: stage.Start, Generation: stage.Generation, AsOf: stage.AsOf, SourceRows: stage.SourceRows, UpperLogID: stage.UpperLogID, Incomplete: stage.Incomplete, RankReady: stage.RankReady, MeterVersion: stage.UsageMeterVersion}
		if stage.HistoricalSource != "" && current.Generation != "" {
			head.SourceRows, head.UpperLogID = current.SourceRows, current.UpperLogID
			head.HistoricalCoverageComplete = current.HistoricalCoverageComplete ||
				(current.HistoricalSource == "" && !current.Incomplete && current.SourceRows > 0 && current.UpperLogID > 0)
		} else if current.HistoricalCoverageComplete {
			head.Incomplete = false // The source checks proved that the full prior boundary was restored.
		}
		metadata, err := common.Marshal(head)
		if err != nil {
			return err
		}
		stored := tx.Model(&usageReportDayRow{}).Where("generation = ? AND user_id = -1", stage.Generation).
			Update("payload", string(metadata))
		if stored.Error != nil {
			return stored.Error
		}
		if stored.RowsAffected != 1 {
			return errDailyReportSourceChanged
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "start"}},
			DoUpdates: clause.AssignmentColumns([]string{"generation", "as_of", "source_rows", "upper_log_id", "incomplete", "rank_ready", "meter_version", "historical_source", "historical_meter_version", "historical_coverage_complete"})}).Create(&head).Error
	}
	if stage.IssueRows == nil || stage.IssueRowsGeneration != stage.Generation {
		return errDailyReportSourceChanged
	}
	var current billingReportHead
	err := lockForUpdate(tx).Where("start = ?", stage.Start).Take(&current).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if current.Generation != stage.Previous {
		return errDailyReportSourceChanged
	}
	var old billingReportGeneration
	if current.Generation != "" {
		if err := tx.Where("id = ?", current.Generation).Take(&old).Error; err != nil {
			return err
		}
		if old.OriginalReplayVersion > stage.OriginalReplayVersion {
			return errDailyReportSourceChanged
		}
		if replacesHistoricalReportWithoutSource(stage, old.HistoricalSource, old.Rows, old.UpperLogID) {
			return errDailyReportCoverageDecreased
		}
		if stage.HistoricalSource == "" && (old.Rows > stage.SourceRows || (stage.Incomplete && !old.Incomplete)) {
			return errDailyReportCoverageDecreased
		}
		if err := tx.Model(&old).Update("retired_at", common.GetTimestamp()).Error; err != nil {
			return err
		}
	}
	rows, upper, incomplete, historicalCoverageComplete := stage.SourceRows, stage.UpperLogID, stage.Incomplete, false
	if stage.HistoricalSource != "" && old.ID != "" {
		rows, upper = old.Rows, old.UpperLogID
		historicalCoverageComplete = old.HistoricalCoverageComplete ||
			(old.HistoricalSource == "" && !old.Incomplete && old.Rows > 0 && old.UpperLogID > 0)
	} else if old.HistoricalCoverageComplete {
		incomplete = false
	}
	stored := tx.Model(&billingReportGeneration{}).Where("id = ?", stage.Generation).
		Updates(map[string]any{"published": true, "as_of": stage.AsOf, "rows": rows, "upper_log_id": upper, "incomplete": incomplete, "historical_source": stage.HistoricalSource,
			"historical_coverage_complete": historicalCoverageComplete, "issue_rows": *stage.IssueRows})
	if stored.Error != nil {
		return stored.Error
	}
	if stored.RowsAffected != 1 {
		return errDailyReportSourceChanged
	}
	head := billingReportHead{Start: stage.Start, Generation: stage.Generation, AsOf: stage.AsOf}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "start"}},
		DoUpdates: clause.AssignmentColumns([]string{"generation", "as_of"})}).Create(&head).Error
}

func MigrateDailyReports(db *gorm.DB) error {
	return db.AutoMigrate(&usageReportDay{}, &usageReportDayRow{}, &usageReportRank{}, &dailyReportRetry{}, &dailyReportExportReference{}, &dailyReportStage{}, &dailyReportPayloadPart{},
		&billingReportHead{}, &billingReportGeneration{}, &billingReportCustomerDay{}, &billingReportUpstreamDay{}, &billingReportLine{}, &billingReportRebuild{}, &billingReportSourceCursor{})
}
