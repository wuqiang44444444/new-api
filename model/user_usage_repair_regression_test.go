package model

import (
	"fmt"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUsageRepairRequiresIndependentReview(t *testing.T) {
	// Correct lifetime net usage is 150. A cleared consume log of 50 makes
	// retained consumption also 150; the old aggregate rule would subtract twice.
	evidence := &UsageRepairEvidence{UserID: 91, UsedQuota: 150, ConsumeCount: 1, ConsumeQuota: 150, RefundCount: 1, RefundQuota: 50,
		Buckets: []UsageRepairBucket{{Kind: UsageRepairBucketOtherUnmatched, Count: 1, Quota: 50, Items: []UsageRepairRefundItem{{LogID: 1, Quota: 50}}}}}
	result := DeriveUsageRepairAssessment(evidence, nil, nil)
	assert.Equal(t, UsageRepairStatusNotCorrectable, result.Status)
	assert.Nil(t, result.Candidate)
	review := reviewedUsageFixture(t, evidence)
	review.LogHistoryComplete = false
	assert.Equal(t, UsageRepairStatusNotCorrectable, DeriveUsageRepairAssessment(evidence, nil, review).Status)
	review.LogHistoryComplete = true
	review.Refunds[0].Decision = "already_accounted"
	// Even an asserted complete history cannot override contradictory figures.
	assert.Equal(t, UsageRepairStatusNotCorrectable, DeriveUsageRepairAssessment(evidence, nil, review).Status)
}

func TestUsageRepairUsesReviewedMissedRefundsOnly(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	require.NoError(t, db.Model(&User{}).Where("id = ?", fixtureTargetUserID).UpdateColumn("used_quota", fixtureUsedQuota-1000000000).Error)
	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	review := reviewedUsageFixture(t, evidence)
	for i := range review.Refunds {
		if review.Refunds[i].LogID == 311 {
			review.Refunds[i].Decision = "already_accounted"
		}
	}
	result := DeriveUsageRepairAssessment(evidence, nil, review)
	require.Equal(t, UsageRepairStatusCorrectable, result.Status)
	assert.Equal(t, fixtureExpectedAfter, result.Candidate.ExpectedAfter)
	assert.Equal(t, -(fixtureOtherQuota + fixtureManualQuota - 1000000000), result.Candidate.Delta)
	review.Refunds[0].Decision = ""
	assert.Equal(t, UsageRepairStatusNotCorrectable, DeriveUsageRepairAssessment(evidence, nil, review).Status)
}

func TestUsageRepairCountsDuplicateGroups(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	for _, request := range []string{"duplicate-a", "duplicate-a", "duplicate-a", "duplicate-b", "duplicate-b"} {
		require.NoError(t, db.Create(&Log{UserId: fixtureTargetUserID, Type: LogTypeConsume, RequestId: request, Quota: 1}).Error)
	}
	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), evidence.DuplicateRequestIDs)
	assert.Equal(t, UsageRepairStatusNotCorrectable, DeriveUsageRepairAssessment(evidence, nil, nil).Status)
}

func TestUsageRepairRejectsChangedSourceDetails(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *gorm.DB)
	}{
		{"offsetting existing consumption", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&Log{}).Where("id = ?", 100).UpdateColumn("quota", gorm.Expr("quota + 5")).Error)
			require.NoError(t, db.Model(&Log{}).Where("id = ?", 101).UpdateColumn("quota", gorm.Expr("quota - 5")).Error)
		}},
		{"refund metadata", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&Log{}).Where("id = ?", 310).UpdateColumn("other", `{"review":"changed"}`).Error)
		}},
		{"task facts", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&Task{}).Where("id = ?", 301).UpdateColumn("quota", 99).Error)
		}},
		{"delivery amount", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&TaskBillingDelivery{}).Where("task_row_id = ? AND event = ?", 301, "refund").UpdateColumn("after_quota", 99).Error)
		}},
		{"attempt facts", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Create(&TaskCreateAttempt{UserID: fixtureTargetUserID, AttemptID: "attempt-new"}).Error)
		}},
		{"maintenance audit", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Create(&AuditLog{Action: "manual_adjustment", EventId: "manual-new", Content: "{}", Success: true}).Error)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newUsageRepairDB(t)
			seedUsageRepairFixture(t, db)
			seedUsageRepairRefunds(t, db)
			manifest, _ := buildUsageRepairManifestForTest(t, db, "repair-source-change", true)
			tc.change(t, db)
			applied, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
			require.ErrorContains(t, err, "source evidence changed")
			assert.False(t, applied)
			var user User
			require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
			assert.Equal(t, fixtureUsedQuota, int64(user.UsedQuota))
		})
	}
}

func TestUsageRepairRejectsConsistentButWrongTarget(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "repair-wrong-target", true)
	manifest.Delta = -1
	manifest.ExpectedAfter = manifest.ExpectedBefore - 1
	_, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.ErrorContains(t, err, "target differs")
	manifest.ExpectedBefore = math.MaxInt64
	manifest.Delta = 1
	manifest.ExpectedAfter = math.MinInt64
	require.ErrorContains(t, ValidateUsageRepairManifest(manifest), "overflows")
}

func TestUsageRepairRollsBackWhenAuditFails(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "repair-audit-fail", true)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("reject_repair_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(fmt.Errorf("audit unavailable"))
		}
	}))
	applied, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.ErrorContains(t, err, "audit unavailable")
	assert.False(t, applied)
	var user User
	require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureUsedQuota, int64(user.UsedQuota))
	assert.Equal(t, fixtureWalletQuota, int64(user.Quota))
	var count int64
	require.NoError(t, db.Model(&AuditLog{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUsageRepairRollbackRequiresAuditAndIsRepeatable(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "repair-rollback-binding", true)
	_, _, err := PrepareUsageRepairRollback(db, manifest)
	require.ErrorContains(t, err, "original successful repair audit")
	_, err = ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.NoError(t, err)
	reverse, already, err := PrepareUsageRepairRollback(db, manifest)
	require.NoError(t, err)
	assert.False(t, already)
	applied, err := ApplyUserUsageRepair(db, reverse, fixtureOperatorID)
	require.NoError(t, err)
	assert.True(t, applied)
	retry, already, err := PrepareUsageRepairRollback(db, manifest)
	require.NoError(t, err)
	assert.True(t, already)
	assert.Equal(t, reverse.OperationID, retry.OperationID)
	var count int64
	require.NoError(t, db.Model(&AuditLog{}).Where("action = ?", UsageRepairAction).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestUsageRepairRollbackRejectsInterveningFacts(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "repair-rollback-drift", true)
	_, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Log{}).Where("id = ?", 100).UpdateColumn("quota", gorm.Expr("quota + 5")).Error)
	require.NoError(t, db.Model(&Log{}).Where("id = ?", 101).UpdateColumn("quota", gorm.Expr("quota - 5")).Error)
	_, _, err = PrepareUsageRepairRollback(db, manifest)
	require.ErrorContains(t, err, "source facts changed")
	// Rebuilding a reverse manifest manually must not bypass that same guard.
	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	fingerprint, err := evidence.Fingerprint()
	require.NoError(t, err)
	reverse := &UsageRepairManifest{Version: 2, OperationID: "manual-reverse", UserID: manifest.UserID,
		ExpectedBefore: manifest.ExpectedAfter, ExpectedAfter: manifest.ExpectedBefore, Delta: -manifest.Delta,
		Evidence: evidence, EvidenceFingerprint: fingerprint, ManualRefundBucketVerified: true, RollbackOf: manifest.OperationID}
	_, err = ApplyUserUsageRepair(db, reverse, fixtureOperatorID)
	require.ErrorContains(t, err, "source facts changed")
}

func TestUsageRepairPriorAuditBeyondTwoHundredOtherUsers(t *testing.T) {
	db := newUsageRepairDB(t)
	contents, err := common.Marshal(usageRepairAuditContent{UserID: fixtureTargetUserID, After: 55})
	require.NoError(t, err)
	require.NoError(t, db.Create(&AuditLog{EventId: "old-target", Action: UsageRepairAction, Content: string(contents), Success: true}).Error)
	other, err := common.Marshal(usageRepairAuditContent{UserID: 99, After: 10})
	require.NoError(t, err)
	// This count reproduces the former global LIMIT 200 loss of target history.
	rows := make([]AuditLog, 201)
	for i := range rows {
		rows[i] = AuditLog{EventId: fmt.Sprintf("other-%d", i), Action: UsageRepairAction, Content: string(other), Success: true}
	}
	require.NoError(t, db.Create(&rows).Error)
	ops, err := CollectUsageRepairPriorOps(db, fixtureTargetUserID)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "old-target", ops[0].EventID)
}

func TestUsageRepairEvidenceDoesNotExportPrivateSourceContent(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	const private = "private-source-must-not-be-exported"
	require.NoError(t, db.Model(&Log{}).Where("id = ?", 100).UpdateColumn("other", `{"private":"`+private+`"}`).Error)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "repair-private-evidence", true)
	raw, err := common.Marshal(manifest)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), private)
	_, err = ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.NoError(t, err)
	var audit AuditLog
	require.NoError(t, db.Where("event_id = ?", manifest.OperationID).Take(&audit).Error)
	assert.NotContains(t, audit.Content, private)
	assert.NotContains(t, audit.Content, "evidence_reference") // full review stays in the approved artifact
}

func TestUsageRepairRejectsInvalidReviewerAndReviewEntries(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "repair-review-boundary", true)
	manifest.Review.ReviewedByUserID = fixtureTargetUserID
	_, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.ErrorContains(t, err, "reviewer is not an active root")
	manifest.Review.ReviewedByUserID = fixtureOperatorID
	manifest.Review.Refunds = append(manifest.Review.Refunds, manifest.Review.Refunds[0])
	require.ErrorContains(t, ValidateUsageRepairManifest(manifest), "duplicated")
}
