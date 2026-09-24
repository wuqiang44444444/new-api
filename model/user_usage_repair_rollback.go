package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// PrepareUsageRepairRollback binds a reverse operation to a successful audit.
// Apply rechecks that audit in its write transaction. A retry of the same
// rollback uses the same operation ID and reports already-done without writes.
func PrepareUsageRepairRollback(db *gorm.DB, original *UsageRepairManifest) (*UsageRepairManifest, bool, error) {
	if err := ValidateUsageRepairManifest(original); err != nil {
		return nil, false, err
	}
	if original.RollbackOf != "" || original.Delta >= 0 {
		return nil, false, fmt.Errorf("only a negative forward correction can be rolled back")
	}
	var forward AuditLog
	if err := db.Where("event_id = ? AND action = ? AND success = ?", original.OperationID, UsageRepairAction, true).Take(&forward).Error; err != nil {
		return nil, false, fmt.Errorf("original successful repair audit is required")
	}
	var content usageRepairAuditContent
	if err := common.UnmarshalJsonStr(forward.Content, &content); err != nil {
		return nil, false, err
	}
	if content.UserID != original.UserID || content.OperationID != original.OperationID || content.Before != original.ExpectedBefore || content.After != original.ExpectedAfter || content.Delta != original.Delta || content.EvidenceFingerprint != original.EvidenceFingerprint || content.RollbackOf != "" {
		return nil, false, fmt.Errorf("original manifest does not match the successful audit")
	}
	reviewFingerprint := ""
	if original.Review != nil {
		var err error
		reviewFingerprint, err = original.Review.Fingerprint()
		if err != nil {
			return nil, false, err
		}
	}
	if content.Policy != original.Policy {
		return nil, false, fmt.Errorf("original policy does not match audit")
	}
	if content.ReviewFingerprint != reviewFingerprint {
		return nil, false, fmt.Errorf("original review does not match the successful audit")
	}
	return prepareUsageRepairRollbackFromAudit(db, &forward, &content, original.SourceDBIdentity)
}

// PrepareRecordedUsageRepairRollback uses the durable startup audit; automatic
// migration does not produce a human-approved manifest file. Normal CLI actor,
// backup and stopped-writer requirements still apply when writing the reversal.
func PrepareRecordedUsageRepairRollback(db *gorm.DB, userID int, source string) (*UsageRepairManifest, bool, error) {
	var forward AuditLog
	operationID := fmt.Sprintf("usage-%s:%d", usageRecordedRefundPolicy, userID)
	if err := db.Where("event_id = ? AND action = ? AND success = ?", operationID, UsageRepairAction, true).Take(&forward).Error; err != nil {
		return nil, false, err
	}
	var content usageRepairAuditContent
	if err := common.UnmarshalJsonStr(forward.Content, &content); err != nil {
		return nil, false, err
	}
	if content.Policy != usageRecordedRefundPolicy || content.OperationID != operationID || content.UserID != userID || content.Before < 0 || content.After < 0 || content.Delta >= 0 || content.Delta < -content.Before || content.Before+content.Delta != content.After || content.RollbackOf != "" {
		return nil, false, fmt.Errorf("successful negative startup correction is required")
	}
	return prepareUsageRepairRollbackFromAudit(db, &forward, &content, source)
}

func prepareUsageRepairRollbackFromAudit(db *gorm.DB, forward *AuditLog, content *usageRepairAuditContent, source string) (*UsageRepairManifest, bool, error) {
	version := 2
	if content.Policy == usageRecordedRefundPolicy {
		version = 3
	}
	sum := sha256.Sum256([]byte("rollback:" + content.OperationID))
	reverse := &UsageRepairManifest{
		Version: version, Policy: content.Policy, OperationID: "usage-rollback-" + hex.EncodeToString(sum[:16]),
		UserID: content.UserID, Username: content.Username,
		ExpectedBefore: content.After, Delta: -content.Delta, ExpectedAfter: content.Before,
		ManualRefundBucketVerified: content.ManualRefundBucketVerified,
		SourceDBIdentity:           source, RollbackOf: content.OperationID,
		CollectedAtUnix: time.Now().Unix(), CollectedAtUTCOffset: time.Now().Format("-07:00"),
	}
	var prior AuditLog
	err := db.Where("event_id = ?", reverse.OperationID).Take(&prior).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if err == nil {
		var recorded usageRepairAuditContent
		if err := common.UnmarshalJsonStr(prior.Content, &recorded); err != nil {
			return nil, false, err
		}
		var user User
		if err := db.Select("id", "used_quota").Take(&user, content.UserID).Error; err != nil {
			return nil, false, err
		}
		if prior.Action != UsageRepairAction || !prior.Success || recorded.OperationID != reverse.OperationID || recorded.UserID != content.UserID || recorded.Before != reverse.ExpectedBefore || recorded.After != reverse.ExpectedAfter || recorded.Delta != reverse.Delta || recorded.RollbackOf != content.OperationID || int64(user.UsedQuota) != reverse.ExpectedAfter {
			return nil, false, fmt.Errorf("rollback already recorded but its audit or account state differs")
		}
		return reverse, true, nil
	}
	evidence, err := CollectUserUsageRepairEvidence(db, db, content.UserID)
	if err != nil {
		return nil, false, err
	}
	if evidence.UsedQuota != content.After {
		return nil, false, fmt.Errorf("rollback before value differs; rebuild the baseline")
	}
	if err := verifyUsageRepairRollbackBaseline(db, evidence, content, forward.Id); err != nil {
		return nil, false, err
	}
	reverse.Evidence = evidence
	reverse.EvidenceFingerprint, err = evidence.Fingerprint()
	return reverse, false, err
}

// A rollback may undo only this maintenance write, not intervening business or
// maintenance activity. Excluding precisely the forward audit also detects
// edits/deletions of earlier audits, including amount-canceling changes.
func verifyUsageRepairRollbackBaseline(db *gorm.DB, evidence *UsageRepairEvidence, original *usageRepairAuditContent, auditID int) error {
	if original.RollbackBaseline == "" || original.PriorAuditDigest.Table != "audit_logs" {
		return fmt.Errorf("original audit has no rollback evidence")
	}
	baseline, err := usageRepairRollbackBaseline(evidence)
	if err != nil {
		return err
	}
	if baseline != original.RollbackBaseline {
		return fmt.Errorf("source facts changed after repair; rollback requires a new baseline")
	}
	excluded := []int{auditID}
	if original.Policy == usageRecordedRefundPolicy {
		// The same startup continues with other users after this correction.
		// Those automatic audits do not change this user's rollback baseline.
		var later []AuditLog
		if err := db.Where("id > ?", auditID).Find(&later).Error; err != nil {
			return err
		}
		for _, row := range later {
			var sibling usageRepairAuditContent
			if common.UnmarshalJsonStr(row.Content, &sibling) != nil || row.Action != UsageRepairAction || !row.Success || row.AuthMethod != "startup_migration" || sibling.Policy != usageRecordedRefundPolicy || sibling.UserID <= 0 || sibling.UserID == original.UserID || sibling.OperationID != fmt.Sprintf("usage-%s:%d", usageRecordedRefundPolicy, sibling.UserID) || row.EventId != sibling.OperationID || sibling.Before < 0 || sibling.After < 0 || sibling.Delta > 0 || sibling.Delta < -sibling.Before || sibling.Before+sibling.Delta != sibling.After || sibling.RollbackOf != "" || sibling.EvidenceFingerprint == "" {
				return fmt.Errorf("maintenance audit changed after repair; rollback requires a new baseline")
			}
			excluded = append(excluded, row.Id)
		}
	}
	audit, err := usageRepairSourceDigest("audit_logs", db.Table("audit_logs").Where("id NOT IN ?", excluded))
	if err != nil {
		return err
	}
	if audit != original.PriorAuditDigest {
		return fmt.Errorf("maintenance audit changed after repair; rollback requires a new baseline")
	}
	return nil
}

func usageRepairRollbackBaseline(e *UsageRepairEvidence) (string, error) {
	baseline := *e
	baseline.UsedQuota = 0
	baseline.PriorOps = nil
	baseline.Sources = nil
	for _, source := range e.Sources {
		if source.Table != "audit_logs" {
			baseline.Sources = append(baseline.Sources, source)
		}
	}
	return baseline.Fingerprint()
}
