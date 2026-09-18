package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BillingSourceVerification struct {
	Fingerprint       string `json:"fingerprint"`
	BackupEvidence    string `json:"backup_evidence"`
	RetentionEvidence string `json:"retention_evidence"`
}
type billingSourceAttestation struct {
	Version int                `json:"version"`
	UserID  int                `json:"user_id"`
	Start   int64              `json:"start"`
	Stamp   billingSourceStamp `json:"stamp"`
	BillingSourceVerification
	Revisions map[string]int64 `json:"revisions"`
}

func billingSourceReviewKey(user int, start int64) string {
	return fmt.Sprintf("source-review:%d:%d", user, start)
}

func ensureBillingSourceReviewLock(tx *gorm.DB, user int) error {
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}}, DoNothing: true}).Create(&BillingStatementRevision{Scope: fmt.Sprintf("ev:%d:0", user), Generation: 1}).Error
}

func RecordBillingSourceVerification(ctx context.Context, user int, start, end int64, input BillingSourceVerification, actor int) error {
	input.BackupEvidence = strings.TrimSpace(input.BackupEvidence)
	input.RetentionEvidence = strings.TrimSpace(input.RetentionEvidence)
	if actor <= 0 || input.BackupEvidence == "" || input.RetentionEvidence == "" || len(input.BackupEvidence) > 4000 || len(input.RetentionEvidence) > 4000 {
		return ErrBillingStatementSourceIncomplete
	}
	r, err := GetBillingSourceReview(ctx, user, start, end)
	if err != nil {
		return err
	}
	if input.Fingerprint != r.Fingerprint {
		return ErrBillingStatementVersionConflict
	}
	if r.Blockers > 0 || r.Pending > 0 {
		return ErrBillingStatementSourceIncomplete
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock order is maintenance -> user registration -> month -> retention.
		_, err := lockBillingStatementMaintenanceTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := ensureBillingSourceReviewLock(tx, user); err != nil {
			return err
		}
		if err := lockBillingSourceReview(ctx, tx, r); err != nil {
			return err
		}
		// Review decisions may have changed after the scan. Re-read them under
		// the maintenance/user locks shared with decision writes.
		if err := loadBillingSourceReviewNotes(ctx, tx, r); err != nil {
			return err
		}
		if r.Blockers > 0 || r.Pending > 0 {
			return ErrBillingStatementSourceIncomplete
		}

		if err := IncrementBillingStatementRevisionTx(tx, fmt.Sprintf("cm:%d:%d", user, start)); err != nil {
			return err
		}
		stamp := r.stamp
		registered := make([]BillingStatementRevision, 0, len(r.revisions))
		for scope := range r.revisions {
			registered = append(registered, BillingStatementRevision{Scope: scope, Generation: 1})
		}
		sort.Slice(registered, func(i, j int) bool { return registered[i].Scope < registered[j].Scope })
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}}, DoNothing: true}).CreateInBatches(registered, 100).Error; err != nil {
			return err
		}
		// The month revision was just incremented to invalidate old drafts.
		r.revisions[fmt.Sprintf("cm:%d:%d", user, start)]++
		raw, err := common.Marshal(billingSourceAttestation{Version: BillingSourceDecisionVersion, UserID: user, Start: start, Stamp: stamp, Revisions: r.revisions, BillingSourceVerification: input})
		if err != nil {
			return err
		}
		if err := upsertBillingStatementRetentionTx(tx, user, start, BillingStatementRetentionIntact, string(raw), nowSeconds()); err != nil {
			return err
		}
		auditRaw, err := common.Marshal(input)
		if err != nil {
			return err
		}
		return tx.Create(&BillingStatementAudit{Action: "verify_source_retention", ActorId: actor, IdempotencyKey: billingSourceReviewKey(user, start), Reason: string(auditRaw), Result: "intact", CreatedAt: nowSeconds()}).Error
	})
}

// An old free-text attestation cannot authorize the new reviewed workflow.
// Already confirmed versions return before this check in the existing path.
func verifyBillingSourceAttestation(ctx context.Context, tx *gorm.DB, r BillingStatementRetention) error {
	var a billingSourceAttestation
	if common.UnmarshalJsonStr(string(r.Detail), &a) != nil || a.Version != BillingSourceDecisionVersion || a.UserID != r.UserId || a.Start != r.PeriodStart || a.Fingerprint == "" {
		return ErrBillingStatementSourceIncomplete
	}
	var m BillingStatementMaintenance
	if err := tx.WithContext(ctx).First(&m, billingStatementMaintenanceRowID).Error; err != nil {
		return err
	}
	if m.Enabled || m.Generation != a.Stamp.Generation || a.Stamp.Parser != BillingStatementParserVersion || a.Revisions == nil {
		return ErrBillingStatementSourceIncomplete
	}
	current := map[string]int64{}
	for scope := range a.Revisions {
		current[scope] = 0
	}
	if err := readBillingSourceReviewRevisions(tx.WithContext(ctx), current); err != nil {
		return err
	}
	for scope, want := range a.Revisions {
		if current[scope] != want {
			return ErrBillingStatementSourceIncomplete
		}
	}

	return nil
}

// Registration and confirmation serialize with writers without pretending that
// merely registering dependencies changed the underlying billing evidence.
func lockBillingSourceUser(tx *gorm.DB, user int) error {
	if err := ensureBillingSourceReviewLock(tx, user); err != nil {
		return err
	}
	return tx.Model(&BillingStatementRevision{}).Where("scope = ?", fmt.Sprintf("ev:%d:0", user)).UpdateColumn("revision", gorm.Expr("revision")).Error
}

// Effective retention keeps the UI and backend gate on the same contract.
func BillingStatementEffectiveRetention(ctx context.Context, r *BillingStatementRetention) (BillingStatementRetentionStatus, error) {
	if r == nil {
		return BillingStatementRetentionUnknown, nil
	}
	if r.Status == BillingStatementRetentionPartial {
		return r.Status, nil
	}
	err := verifyBillingSourceAttestation(ctx, DB, *r)
	if err == ErrBillingStatementSourceIncomplete || err == ErrBillingStatementVersionDisabled {
		return BillingStatementRetentionUnknown, nil
	}
	if err != nil {
		return BillingStatementRetentionUnknown, err
	}
	return r.Status, nil
}

func readBillingSourceReviewRevisions(db *gorm.DB, values map[string]int64) error {
	scopes := make([]string, 0, len(values))
	for scope := range values {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for start := 0; start < len(scopes); start += 500 {
		var rows []BillingStatementRevision
		if err := db.Session(&gorm.Session{}).Where("scope IN ?", scopes[start:min(start+500, len(scopes))]).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			values[row.Scope] = row.Revision
		}
	}
	return nil
}

// New explicit task log relations in later months must invalidate a reviewed
// earlier settlement, even when the Task row itself does not change.
func billingSourceTaskLogScope(user, token int, task string) string {
	return fmt.Sprintf("review-task-log:%d:%d:%x", user, token, sha256.Sum256([]byte(task)))
}
func addBillingSourceReviewLogScope(scopes map[string]struct{}, row billingStatementSourceRow) {
	if !strings.Contains(row.Other, `"task_id"`) {
		return
	}
	var relation struct {
		TaskID string `json:"task_id"`
	}
	if common.UnmarshalJsonStr(row.Other, &relation) == nil && relation.TaskID != "" {
		scopes[billingSourceTaskLogScope(row.UserId, row.TokenId, relation.TaskID)] = struct{}{}
	}
}
