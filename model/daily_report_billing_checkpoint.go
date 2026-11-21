package model

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const billingReportCheckpointVersion = CustomerOriginalReplayVersion<<8 | 1

type billingReportCheckpoint struct {
	stage          dailyReportStage
	digest         hash.Hash
	processed      int64
	reuseSummaries bool
	taskID         string
	runnerID       string
}

func beginBillingReportCheckpoint(ctx context.Context, db *gorm.DB, generation *billingReportGeneration, previous, dependencies, taskID, runnerID string, started time.Time) (*billingReportCheckpoint, error) {
	var stage dailyReportStage
	readCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
	err := db.WithContext(readCtx).Where("start = ? AND kind = ?", generation.Start, DailyReportBilling).Take(&stage).Error
	cancel()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	resumed := stage.unfinished() && stage.BillingBuildVersion == billingReportCheckpointVersion &&
		dailyReportStageVersionCurrent(stage) && stage.Previous == previous && stage.Dependencies == dependencies &&
		stage.Incomplete == generation.Incomplete && stage.UpperLogID <= generation.UpperLogID &&
		started.Unix()-stage.AsOf < 8*billingReportDaySeconds
	if resumed {
		generation.ID, generation.AsOf, generation.UpperLogID = stage.Generation, stage.AsOf, stage.UpperLogID
	} else {
		var issues int64
		stage = dailyReportStage{Start: generation.Start, Kind: DailyReportBilling,
			Generation: generation.ID, AsOf: generation.AsOf, UpperLogID: generation.UpperLogID,
			Previous: previous, Dependencies: dependencies, Incomplete: generation.Incomplete,
			IssueRows: &issues, IssueRowsGeneration: generation.ID, BillingBuildVersion: billingReportCheckpointVersion,
			CopyingGeneration: generation.ID, CopyingStartedAt: started.UnixNano()}
		if err := persistDailyReportStage(ctx, db, stage, taskID, runnerID, generation); err != nil {
			return nil, err
		}
	}
	digest := hmac.New(sha256.New, []byte(common.CryptoSecret))
	digest.Write([]byte("billing-report-committed-prefix:" + stage.Generation))
	return &billingReportCheckpoint{stage: stage, digest: digest, reuseSummaries: resumed && stage.BillingBuildSummaries, taskID: taskID, runnerID: runnerID}, nil
}

// Replay reconstructs in-memory accumulators with the same business rules.
// Compare the projected prefix before reusing it. The private task reference
// is hashed in memory before randomized encryption and is never stored here.
func (c *billingReportCheckpoint) observe(ctx context.Context, db *gorm.DB, row billingReportLine, rootTask string) (bool, error) {
	row.RootTaskRef = rootTask
	raw, err := common.Marshal(row)
	if err != nil {
		return false, err
	}
	c.digest.Write(raw)
	c.processed++
	if c.reuseSummaries && c.processed > c.stage.BillingBuildRows {
		return false, c.invalidate(ctx, db)
	}
	if c.processed == c.stage.BillingBuildRows && hex.EncodeToString(c.digest.Sum(nil)) != c.stage.BillingBuildDigest {
		return false, c.invalidate(ctx, db)
	}
	return c.processed <= c.stage.BillingBuildRows, nil
}

func (c *billingReportCheckpoint) commit(ctx context.Context, db *gorm.DB, rows []billingReportLine, issues int64, digest string) error {
	next := c.stage.BillingBuildRows + int64(len(rows))
	err := dailyReportStageTransaction(ctx, db, c.taskID, c.runnerID, func(tx *gorm.DB) error {
		check := tx.Model(&dailyReportStage{}).
			Where("generation = ? AND copying_generation = ? AND billing_build_rows = ?", c.stage.Generation, c.stage.Generation, c.stage.BillingBuildRows).
			Updates(map[string]any{"billing_build_rows": next, "billing_build_digest": digest, "issue_rows": issues})
		if check.Error != nil {
			return check.Error
		}
		if check.RowsAffected != 1 {
			return errDailyReportSourceChanged
		}
		return tx.Create(&rows).Error
	})
	if err == nil {
		c.stage.BillingBuildRows, c.stage.BillingBuildDigest = next, digest
		c.stage.IssueRows = &issues
	}
	return err
}

func (c *billingReportCheckpoint) invalidate(ctx context.Context, db *gorm.DB) error {
	if err := usageReportBeforeBatch(ctx); err != nil {
		return err
	}
	err := dailyReportStageTransaction(ctx, db, c.taskID, c.runnerID, func(tx *gorm.DB) error {
		return tx.Where("generation = ? AND copying_generation = ?", c.stage.Generation, c.stage.Generation).Delete(&dailyReportStage{}).Error
	})
	if err != nil {
		return err
	}
	return errDailyReportSourceChanged
}

func (c *billingReportCheckpoint) prepareSummaries(ctx context.Context, db *gorm.DB, coverage dailyReportCoverage) error {
	if c.processed != c.stage.BillingBuildRows ||
		c.stage.BillingBuildSummaries && (c.stage.SourceRows != coverage.Rows || c.stage.SourceRevision != coverage.Revision) {
		return c.invalidate(ctx, db)
	}
	if c.stage.BillingBuildSummaries {
		return nil
	}
	c.stage.BillingBuildSummaries = true
	c.stage.SourceRows, c.stage.SourceRevision = coverage.Rows, coverage.Revision
	return persistDailyReportStage(ctx, db, c.stage, c.taskID, c.runnerID, nil)
}

// A summary's indexed row is written after all its payload fragments. Reuse
// completed scopes. If only fragments survived the attempt, replace just those.
func (c *billingReportCheckpoint) reuseSummary(ctx context.Context, db *gorm.DB, table any, column, kind string, id int) (bool, error) {
	if !c.reuseSummaries {
		return false, nil
	}
	if err := usageReportBeforeBatch(ctx); err != nil {
		return false, err
	}
	readCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
	var header struct{ Generation string }
	err := db.WithContext(readCtx).Model(table).Select("generation").Where("generation = ? AND "+column+" = ?", c.stage.Generation, id).Take(&header).Error
	cancel()
	if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return err == nil, err
	}
	for {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return false, err
		}
		var parts []dailyReportPayloadPart
		readCtx, cancel := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
		err := db.WithContext(readCtx).Select("generation", "kind", "scope_id", "part").
			Where("generation = ? AND kind = ? AND scope_id = ?", c.stage.Generation, kind, id).Limit(8).Find(&parts).Error
		cancel()
		if err != nil || len(parts) == 0 {
			return false, err
		}
		if err := beforeDailyReportWrite(ctx, len(parts)); err != nil {
			return false, err
		}
		if err := dailyReportStageTransaction(ctx, db, c.taskID, c.runnerID, func(tx *gorm.DB) error {
			return tx.Delete(&parts).Error
		}); err != nil {
			return false, err
		}
	}
}
