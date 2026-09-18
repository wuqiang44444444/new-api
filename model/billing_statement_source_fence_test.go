package model

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSourceVerificationRejectsRevisionFailureAfterScan(t *testing.T) {
	for _, partial := range []bool{false, true} {
		name := "first failure"
		if partial {
			name = "already partial"
		}
		t.Run(name, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			require.NoError(t, db.AutoMigrate(&Task{}))
			ctx := context.Background()
			const start, end = int64(1785513600), int64(1788191999)
			log := Log{UserId: 91, TokenId: 40, CreatedAt: start + 1, Type: LogTypeConsume, Quota: 100, Other: `{"model_price":1}`}
			require.NoError(t, db.Create(&log).Error)
			if partial {
				require.NoError(t, upsertBillingStatementRetentionTx(db, 91, start, BillingStatementRetentionPartial, "earlier failure", nowSeconds()))
			}
			report, err := GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			require.Empty(t, report.Issues)
			input := BillingSourceVerification{Fingerprint: report.Fingerprint, BackupEvidence: "fixture backup", RetentionEvidence: "fixture retention"}
			finishedTasks, injected, rejectRevision := false, false, false
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject_source_revision_once", func(tx *gorm.DB) {
				if rejectRevision && tx.Statement.Table == "billing_statement_revisions" {
					rejectRevision = false
					tx.AddError(errors.New("transient revision failure"))
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Create().Remove("test:reject_source_revision_once") })
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:write_after_source_scan", func(tx *gorm.DB) {
				if tx.Statement.Table == "tasks" && tx.RowsAffected == 0 {
					finishedTasks = true
				}
				if !finishedTasks || injected || tx.Statement.Table != "billing_statement_maintenance" {
					return
				}
				// The final stamp's generation has been read. Commit a source change
				// whose revision fails before the verification transaction starts.
				injected, rejectRevision = true, true
				tx.AddError(db.Model(&log).Update("quota", 101).Error)
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove("test:write_after_source_scan") })
			err = RecordBillingSourceVerification(ctx, 91, start, end, input, 1)
			require.True(t, injected)
			assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
			var stored Log
			require.NoError(t, db.First(&stored, log.Id).Error)
			assert.Equal(t, 101, stored.Quota, "the committed financial fact must be retained")
			retention, err := GetBillingStatementRetention(ctx, 91, start)
			require.NoError(t, err)
			require.NotNil(t, retention)
			assert.Equal(t, BillingStatementRetentionPartial, retention.Status)
			var audits int64
			require.NoError(t, db.Model(&BillingStatementAudit{}).Where("action = ?", "verify_source_retention").Count(&audits).Error)
			assert.Zero(t, audits, "a rejected verification must not record success")
			fresh, err := GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			input.Fingerprint = fresh.Fingerprint
			require.NoError(t, RecordBillingSourceVerification(ctx, 91, start, end, input, 1), "a fresh scan may recover after the transient failure")
			require.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
		})
	}
}

func TestSourceRevisionFailureRollsBackWhenGenerationFenceCannotPersist(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	log := Log{UserId: 91, TokenId: 40, CreatedAt: 1785513601, Type: LogTypeConsume, Quota: 100}
	require.NoError(t, db.Create(&log).Error)
	var before BillingStatementMaintenance
	require.NoError(t, db.First(&before, billingStatementMaintenanceRowID).Error)
	revisionErr, fenceErr := errors.New("revision unavailable"), errors.New("generation unavailable")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject_revision", func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_statement_revisions" {
			tx.AddError(revisionErr)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:reject_revision") })
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:reject_generation", func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_statement_maintenance" {
			if values, ok := tx.Statement.Dest.(map[string]interface{}); ok {
				if _, changesGeneration := values["generation"]; changesGeneration {
					tx.AddError(fenceErr)
				}
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove("test:reject_generation") })
	assert.ErrorIs(t, db.Model(&log).Update("quota", 101).Error, fenceErr)
	var stored Log
	require.NoError(t, db.First(&stored, log.Id).Error)
	assert.Equal(t, 100, stored.Quota, "source must not commit without its revision or durable failure fence")
	var after BillingStatementMaintenance
	require.NoError(t, db.First(&after, billingStatementMaintenanceRowID).Error)
	assert.Equal(t, before.Generation, after.Generation)
}

func TestSourceVerificationRejectsLogWriteFailureAfterScan(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	require.NoError(t, upsertBillingStatementRetentionTx(db, 91, start, BillingStatementRetentionPartial, "earlier failure", nowSeconds()))
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	writeErr := errors.New("source write rejected")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject_missing_source", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			tx.AddError(writeErr)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:reject_missing_source") })
	finishedTasks, injected := false, false
	var observedErr error
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:missing_source_after_scan", func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" && tx.RowsAffected == 0 {
			finishedTasks = true
		}
		if finishedTasks && !injected && tx.Statement.Table == "billing_statement_maintenance" {
			injected = true
			observedErr = createLog(&Log{UserId: 91, TokenId: 40, Type: LogTypeConsume, Quota: 17, CreatedAt: start + 1})
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:missing_source_after_scan") })
	err = RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: report.Fingerprint, BackupEvidence: "fixture backup", RetentionEvidence: "fixture retention"}, 1)
	require.True(t, injected)
	assert.ErrorIs(t, observedErr, writeErr)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
	retention, err := GetBillingStatementRetention(ctx, 91, start)
	require.NoError(t, err)
	require.NotNil(t, retention)
	assert.Equal(t, BillingStatementRetentionPartial, retention.Status)
	var count int64
	require.NoError(t, db.Model(&Log{}).Count(&count).Error)
	assert.Zero(t, count, "the failed log write must not be replayed")
	require.NoError(t, db.Model(&BillingStatementAudit{}).Where("action = ?", "verify_source_retention").Count(&count).Error)
	assert.Zero(t, count, "stale verification must not append a success audit")
}
