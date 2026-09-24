package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Exact aggregates from the verified local account: used_quota, consume and
// refund totals, the delivery-matched refund total, the seven manual refund
// entries and the remaining unmatched refunds all reproduce the documented
// correction target 2,829,317,796.
const (
	fixtureUsedQuota     = int64(5545156555)
	fixtureWalletQuota   = int64(186392868)
	fixtureRequestCount  = int64(33799)
	fixtureConsumeQuota  = int64(6361019369)
	fixtureRefundQuota   = int64(3531701573)
	fixtureMatchedQuota  = int64(815862814)
	fixtureManualQuota   = int64(9385336)
	fixtureOtherQuota    = int64(2706453423)
	fixtureExpectedAfter = int64(2829317796)
	fixtureOperatorID    = 1
	fixtureTargetUserID  = 91
)

func newUsageRepairDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &Task{}, &TaskBillingDelivery{}, &AuditLog{}, &QuotaData{}, &Channel{}, &TaskCreateAttempt{}))
	return db
}

func seedUsageRepairFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&User{
		Id: fixtureTargetUserID, Username: "randy", Role: common.RoleCommonUser, Status: 1, AffCode: "u91",
		Quota: int(fixtureWalletQuota), UsedQuota: int(fixtureUsedQuota), RequestCount: int(fixtureRequestCount),
	}).Error)
	require.NoError(t, db.Create(&User{
		Id: fixtureOperatorID, Username: "root", Role: common.RoleRootUser, Status: 1, AffCode: "root1",
	}).Error)
	require.NoError(t, db.Create(&Channel{Id: 7, UsedQuota: 123456789012}).Error)

	require.NoError(t, db.Exec("INSERT INTO quota_data (user_id, username, model_name, created_at, quota, count) VALUES (?, 'randy', 'm', 1, 5236704637, 1), (?, 'randy', 'n', 1, 300000000, 1)",
		fixtureTargetUserID, fixtureTargetUserID).Error)

	consumeSpecs := []struct {
		id      int64
		quota   int64
		request string
	}{
		{100, 2000000000, "req-c1"},
		{103, 2000000000, "req-c4"},
		{101, 2000000000, "req-c2"},
		{102, 361019369, "req-c3"},
	}
	for _, spec := range consumeSpecs {
		require.NoError(t, db.Create(&Log{
			Id: int(spec.id), UserId: fixtureTargetUserID, Type: LogTypeConsume, Quota: int(spec.quota),
			ChannelId: 7, RequestId: spec.request, Other: "{}", CreatedAt: 1758200000,
		}).Error)
	}
}

func seedUsageRepairTask(t *testing.T, db *gorm.DB, taskRowID int64) {
	t.Helper()
	require.NoError(t, db.Create(&Task{ID: taskRowID, UserId: fixtureTargetUserID, TaskID: "task_x", Platform: "seedance"}).Error)
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: taskRowID, Event: "create", CreatedAt: 1, DeliveredAt: 1}).Error)
}

func seedUsageRepairRefunds(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedUsageRepairTask(t, db, 301)
	seedUsageRepairTask(t, db, 302)
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: 301, Event: "refund", CreatedAt: 2, DeliveredAt: 2}).Error)
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: 302, Event: "refund", CreatedAt: 2, DeliveredAt: 2}).Error)
	require.NoError(t, db.Create(&Log{Id: 200, UserId: fixtureTargetUserID, Type: LogTypeRefund, Quota: 400000000, ChannelId: 7, RequestId: "task-billing:301:refund", Other: "{}", CreatedAt: 1758300000}).Error)
	require.NoError(t, db.Create(&Log{Id: 201, UserId: fixtureTargetUserID, Type: LogTypeRefund, Quota: 415862814, ChannelId: 7, RequestId: "task-billing:302:refund", Other: "{}", CreatedAt: 1758300001}).Error)

	manualQuotas := []int64{1588394, 1277621, 1588394, 1333871, 1288394, 1098394, 1210268}
	preauthCycle := []int64{100, 101, 102}
	for i, quota := range manualQuotas {
		taskRowID := int64(4101 + i)
		seedUsageRepairTask(t, db, taskRowID)
		preauth := preauthCycle[i%len(preauthCycle)]
		other := `{"admin_info": {"manual_refund": true, "task_id": "task_m", "reason": "upstream confirmed failed", "operator": "manual-db-fix", "original_preauth_log_id": ` + strconvFormatInt(preauth) + `}}`
		require.NoError(t, db.Create(&Log{
			Id: 5000 + i, UserId: fixtureTargetUserID, Type: LogTypeRefund, Quota: int(quota), ChannelId: 7,
			RequestId: "task-billing:" + strconvFormatInt(taskRowID) + ":refund", Other: other, CreatedAt: 1758400000 + int64(i),
		}).Error)
	}
	require.NoError(t, db.Create(&Log{Id: 310, UserId: fixtureTargetUserID, Type: LogTypeRefund, Quota: int(fixtureOtherQuota - 1000000000), ChannelId: 7, RequestId: "legacy-refund-2025", Other: "{}", CreatedAt: 1758500000}).Error)
	require.NoError(t, db.Create(&Log{Id: 311, UserId: fixtureTargetUserID, Type: LogTypeRefund, Quota: 1000000000, ChannelId: 7, RequestId: "legacy-refund-2", Other: "{}", CreatedAt: 1758500001}).Error)
}

func strconvFormatInt(value int64) string {
	return fmt.Sprintf("%d", value)
}

func buildUsageRepairManifestForTest(t *testing.T, db *gorm.DB, operationID string, manualVerified bool) (*UsageRepairManifest, *UsageRepairEvidence) {
	t.Helper()
	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	ops, err := CollectUsageRepairPriorOps(db, fixtureTargetUserID)
	require.NoError(t, err)
	assessment := DeriveUsageRepairAssessment(evidence, ops, reviewedUsageFixture(t, evidence))
	require.Equal(t, UsageRepairStatusCorrectable, assessment.Status)
	require.NotNil(t, assessment.Candidate)
	fingerprint, err := evidence.Fingerprint()
	require.NoError(t, err)
	return &UsageRepairManifest{
		Version:                    2,
		Review:                     reviewedUsageFixture(t, evidence),
		SourceDBIdentity:           "sqlite:fixture",
		OperationID:                operationID,
		UserID:                     evidence.UserID,
		Username:                   evidence.Username,
		ExpectedBefore:             assessment.Candidate.ExpectedBefore,
		Delta:                      assessment.Candidate.Delta,
		ExpectedAfter:              assessment.Candidate.ExpectedAfter,
		ManualRefundBucketVerified: manualVerified,
		EvidenceFingerprint:        fingerprint,
		Evidence:                   evidence,
		CollectedAtUnix:            time.Now().Unix(),
	}, evidence
}

func TestUsageRepairPreviewBuckets(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)

	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	assert.Equal(t, fixtureUsedQuota, evidence.UsedQuota)
	assert.Equal(t, fixtureWalletQuota, evidence.WalletQuota)
	assert.Equal(t, fixtureRequestCount, evidence.RequestCount)
	assert.Equal(t, fixtureConsumeQuota, evidence.ConsumeQuota)
	assert.Equal(t, int64(4), evidence.ConsumeCount)
	assert.Equal(t, fixtureRefundQuota, evidence.RefundQuota)
	assert.Equal(t, int64(11), evidence.RefundCount)

	buckets := map[string]UsageRepairBucket{}
	for _, bucket := range evidence.Buckets {
		buckets[bucket.Kind] = bucket
	}
	assert.Equal(t, int64(2), buckets[UsageRepairBucketDeliveryMatched].Count)
	assert.Equal(t, fixtureMatchedQuota, buckets[UsageRepairBucketDeliveryMatched].Quota)
	prefix := buckets[UsageRepairBucketPrefixWithoutDelivery]
	assert.Equal(t, int64(7), prefix.Count)
	assert.Equal(t, fixtureManualQuota, prefix.Quota)
	require.Len(t, prefix.Items, 7)
	manualItems := 0
	for _, item := range prefix.Items {
		assert.True(t, item.TaskExists)
		assert.Equal(t, []string{"create"}, item.TaskDeliveryEvents)
		assert.True(t, item.PreauthExists)
		assert.Equal(t, "manual-db-fix", item.Operator)
		if item.ManualRefund {
			manualItems++
		}
	}
	assert.Equal(t, 7, manualItems, "every prefix entry carries the manual refund marker")
	assert.Equal(t, int64(2), buckets[UsageRepairBucketOtherUnmatched].Count)
	assert.Equal(t, fixtureOtherQuota, buckets[UsageRepairBucketOtherUnmatched].Quota)
	assert.Equal(t, int64(0), evidence.UndeliveredEvents)
	assert.Equal(t, int64(0), evidence.DuplicateRequestIDs)
}

func TestUsageRepairCandidateMatchesDocumentedTarget(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)

	manifest, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-target", true)
	assert.Equal(t, fixtureUsedQuota, manifest.ExpectedBefore)
	assert.Equal(t, fixtureExpectedAfter-fixtureUsedQuota, manifest.Delta)
	assert.Equal(t, fixtureExpectedAfter, manifest.ExpectedAfter)
}

func TestUsageRepairApplyCorrectsUsedQuotaExactlyOnce(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-apply", true)

	applied, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.NoError(t, err)
	assert.True(t, applied)

	var user User
	require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureExpectedAfter, int64(user.UsedQuota))
	assert.Equal(t, fixtureWalletQuota, int64(user.Quota))
	assert.Equal(t, fixtureRequestCount, int64(user.RequestCount))

	var audit AuditLog
	require.NoError(t, db.Where("event_id = ?", "usage-repair-test-apply").Take(&audit).Error)
	assert.True(t, audit.Success)
	assert.Equal(t, UsageRepairAction, audit.Action)
	assert.Equal(t, fixtureOperatorID, audit.UserId)

	// Rerunning the same operation must not subtract the delta again.
	applied, err = ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.NoError(t, err)
	assert.False(t, applied)
	require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureExpectedAfter, int64(user.UsedQuota))

	// A fresh preview reports zero pending adjustments.
	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	ops, err := CollectUsageRepairPriorOps(db, fixtureTargetUserID)
	require.NoError(t, err)
	assessment := DeriveUsageRepairAssessment(evidence, ops, reviewedUsageFixture(t, evidence))
	assert.Equal(t, UsageRepairStatusZeroPending, assessment.Status)
}

func TestUsageRepairManualBucketRequiresVerification(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)

	manifest, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-gate", false)
	err := ValidateUsageRepairManifest(manifest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manual_refund_bucket_verified")

	manifest.ManualRefundBucketVerified = true
	require.NoError(t, ValidateUsageRepairManifest(manifest))
}

func TestUsageRepairApplyRejectsCancelingEvidenceChange(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-cancel", true)

	// Two canceling writes leave used_quota identical but change the source
	// evidence, which must invalidate the manifest.
	require.NoError(t, db.Create(&Log{Id: 400, UserId: fixtureTargetUserID, Type: LogTypeConsume, Quota: 5000, ChannelId: 7, RequestId: "req-late", Other: "{}", CreatedAt: 1758600000}).Error)
	require.NoError(t, db.Create(&Log{Id: 401, UserId: fixtureTargetUserID, Type: LogTypeRefund, Quota: 5000, ChannelId: 7, RequestId: "refund-late", Other: "{}", CreatedAt: 1758600001}).Error)

	_, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evidence changed")

	var user User
	require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureUsedQuota, int64(user.UsedQuota))
}

func TestUsageRepairApplyRejectsTamperedManifest(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-tamper", true)

	manifest.ExpectedAfter = manifest.ExpectedBefore // arithmetic no longer holds
	require.Error(t, ValidateUsageRepairManifest(manifest))

	manifest2, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-tamper2", true)
	manifest2.Delta = -999
	require.Error(t, ValidateUsageRepairManifest(manifest2))
}

func TestUsageRepairApplyRequiresRootOperator(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-op", true)

	_, err := ApplyUserUsageRepair(db, manifest, fixtureTargetUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a root user")
}

func TestUsageRepairBlocksUndeliveredEvents(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	require.NoError(t, db.Create(&Task{ID: 9001, UserId: fixtureTargetUserID, TaskID: "task_pending", Platform: "seedance"}).Error)
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: 9001, Event: "adjustment", CreatedAt: 3, DeliveredAt: 0, NextRetryAt: 3}).Error)

	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), evidence.UndeliveredEvents)
	assessment := DeriveUsageRepairAssessment(evidence, nil, nil)
	assert.Equal(t, UsageRepairStatusNotCorrectable, assessment.Status)
	assert.NotEmpty(t, assessment.Issues)
}

func TestUsageRepairRollbackRestoresPreviousValue(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	manifest, _ := buildUsageRepairManifestForTest(t, db, "usage-repair-test-fwd", true)
	applied, err := ApplyUserUsageRepair(db, manifest, fixtureOperatorID)
	require.NoError(t, err)
	require.True(t, applied)

	evidence, err := CollectUserUsageRepairEvidence(db, db, fixtureTargetUserID)
	require.NoError(t, err)
	require.Equal(t, manifest.ExpectedAfter, evidence.UsedQuota)
	fingerprint, err := evidence.Fingerprint()
	require.NoError(t, err)
	reverse := &UsageRepairManifest{
		Version: 2, OperationID: "usage-repair-test-back", UserID: manifest.UserID, Username: manifest.Username,
		ExpectedBefore: manifest.ExpectedAfter, Delta: -manifest.Delta, ExpectedAfter: manifest.ExpectedBefore,
		ManualRefundBucketVerified: manifest.ManualRefundBucketVerified,
		EvidenceFingerprint:        fingerprint, Evidence: evidence,
		RollbackOf: manifest.OperationID,
	}
	applied, err = ApplyUserUsageRepair(db, reverse, fixtureOperatorID)
	require.NoError(t, err)
	require.True(t, applied)

	var user User
	require.NoError(t, db.Take(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureUsedQuota, int64(user.UsedQuota))
	var audits []AuditLog
	require.NoError(t, db.Where("action = ?", UsageRepairAction).Order("id").Find(&audits).Error)
	require.Len(t, audits, 2)
}

func reviewedUsageFixture(t *testing.T, evidence *UsageRepairEvidence) *UsageRepairReview {
	t.Helper()
	review, err := NewUsageRepairReview(evidence, "sqlite:fixture")
	require.NoError(t, err)
	review.ReviewedByUserID = fixtureOperatorID
	review.LogHistoryComplete = true
	review.LogHistoryEvidence = "fixture: complete consume/refund history"
	review.PriorAdjustmentsEvidence = "fixture: no prior manual adjustments"
	for i := range review.Refunds {
		review.Refunds[i].Decision = "missed_usage_decrement"
		review.Refunds[i].EvidenceReference = "fixture: legacy writer omitted statistic decrement; funding already refunded"
	}
	return review
}
