package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBillingSourcePendingSurvivesResolvedExplanation(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	row := Log{UserId: 91, TokenId: 40, CreatedAt: start + 10, Type: LogTypeConsume, Quota: 100, Other: `{"is_task":true,"model_price":0}`}
	require.NoError(t, db.Create(&row).Error)
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.Issues, 1)
	d := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: r.Issues[0].ID, Decision: BillingSourceWaiver, Reason: "customer_agreement", Note: "customer requests compensation", RequestID: "pending-after-repair"}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
	require.NoError(t, db.Model(&row).Update("other", `{"model_price":1,"group_ratio":1}`).Error)
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Empty(t, r.Issues)
	assert.Equal(t, 1, r.Pending, "resolving billing evidence does not resolve the customer request")
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1), ErrBillingStatementSourceIncomplete)
}

func TestBillingSourceCorruptAuditReadableButBlocked(t *testing.T) {
	for _, raw := range []string{`{broken`, `null`, `{}`, `{"version":2,"issue_id":"log:1"}`} {
		t.Run(raw, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			require.NoError(t, db.AutoMigrate(&Task{}))
			const start, end = int64(1785513600), int64(1788191999)
			require.NoError(t, db.Create(&BillingStatementAudit{Action: "review_source_issue", IdempotencyKey: billingSourceReviewKey(91, start), ActorId: 1, Reason: raw}).Error)
			r, err := GetBillingSourceReview(context.Background(), 91, start, end)
			require.NoError(t, err, "audit damage must not hide the entire review page")
			require.NotNil(t, r)
			assert.Greater(t, r.Blockers, 0)
			assert.ErrorIs(t, RecordBillingSourceVerification(context.Background(), 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1), ErrBillingStatementSourceIncomplete)
		})
	}
}

func TestBillingSourceRequestClosureIsExplicitAndDoesNotMoveMoney(t *testing.T) {
	for _, decision := range []string{BillingSourceWithdraw, BillingSourceReject} {
		t.Run(decision, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			require.NoError(t, db.AutoMigrate(&Task{}, &User{}))
			user := User{Id: 91, Username: "closure-customer", Quota: 900, UsedQuota: 100}
			require.NoError(t, db.Create(&user).Error)
			const start, end = int64(1785513600), int64(1788191999)
			ctx := context.Background()
			row := Log{UserId: 91, TokenId: 40, CreatedAt: start + 1, Type: LogTypeConsume, Quota: 100, Other: `{"is_task":true,"model_price":0}`}
			require.NoError(t, db.Create(&row).Error)
			r, err := GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			d := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: r.Issues[0].ID, Decision: BillingSourceWaiver, Reason: "customer_agreement", Note: "customer requests a waiver", RequestID: "closure-initial-request"}
			require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
			r, err = GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			require.Len(t, r.PendingRequests, 1)
			previous := r.PendingRequests[0].ID
			keep := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: d.IssueID, PreviousID: previous, Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "cannot-hide-open-request"}
			assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, keep, 1), ErrBillingStatementSourceIncomplete)
			require.NoError(t, db.Model(&row).Update("other", `{"model_price":1,"group_ratio":1}`).Error)
			r, err = GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			require.Empty(t, r.Issues)
			require.Len(t, r.PendingRequests, 1)
			close := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: d.IssueID, PreviousID: previous, Decision: decision, Reason: "request_withdrawn", Note: "customer and manager agree to keep existing accounting", RequestID: "explicit-closure-request"}
			if decision == BillingSourceReject {
				close.Reason = "request_rejected"
			}
			bad := close
			bad.PreviousID = 0
			assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, bad, 1), ErrBillingStatementVersionConflict)
			require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, close, 1))
			require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, close, 1))
			competing := close
			competing.RequestID = "competing-closure-request"
			assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, competing, 2), ErrBillingStatementVersionConflict)
			r, err = GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			assert.Empty(t, r.PendingRequests)
			assert.Zero(t, r.Pending)
			require.Len(t, r.History, 2)
			assert.True(t, r.History[0].Superseded)
			assert.Equal(t, close.Reason, r.History[1].Status)
			assert.EqualValues(t, 100, r.History[1].RecordedQuota)
			assert.Zero(t, r.History[1].BalanceDelta)
			assert.Zero(t, r.History[1].StatementDelta)
			require.NoError(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1))
			require.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
			var stored User
			require.NoError(t, db.First(&stored, 91).Error)
			assert.Equal(t, user.Quota, stored.Quota)
			assert.Equal(t, user.UsedQuota, stored.UsedQuota)
			var storedLog Log
			require.NoError(t, db.First(&storedLog, row.Id).Error)
			assert.Equal(t, row.Quota, storedLog.Quota)
		})
	}
}

func TestBillingSourceClosureDoesNotAcceptRemainingExplanation(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	const start, end = int64(1785513600), int64(1788191999)
	ctx := context.Background()
	require.NoError(t, db.Create(&Log{UserId: 91, CreatedAt: start + 1, Type: LogTypeRefund, Quota: 100, Other: `{}`}).Error)
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.Issues, 1)
	d := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: r.Issues[0].ID, Decision: BillingSourceInvestigate, Reason: "missing_evidence", Note: "review the refund", RequestID: "refund-review-request"}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	d.PreviousID = r.PendingRequests[0].ID
	d.Decision = BillingSourceWithdraw
	d.Reason = "request_withdrawn"
	d.RequestID = "refund-closure-request"
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	assert.Empty(t, r.PendingRequests)
	assert.Equal(t, 1, r.Pending)
	assert.False(t, r.Issues[0].Reviewed)
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1), ErrBillingStatementSourceIncomplete)
}

func TestBillingSourceDamagedLatestDecisionCannotResurrectAcceptance(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	const start, end = int64(1785513600), int64(1788191999)
	ctx := context.Background()
	require.NoError(t, db.Create(&Log{UserId: 91, CreatedAt: start + 1, Type: LogTypeConsume, Quota: 100, Other: `{}`}).Error)
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	d := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: r.Issues[0].ID, Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "keep-before-damage"}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
	require.NoError(t, db.Create(&BillingStatementAudit{Action: "review_source_issue", IdempotencyKey: billingSourceReviewKey(91, start), Reason: `{"decision":"investigate",`, ActorId: 2}).Error)
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.AuditIssues, 1)
	assert.False(t, r.Issues[0].Reviewed)
	assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1), ErrBillingStatementSourceIncomplete)
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1), ErrBillingStatementSourceIncomplete)
	var count int64
	require.NoError(t, db.Model(&BillingStatementAudit{}).Where("action = ?", "review_source_issue").Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func TestBillingSourceOldAttestationRequiresFreshReview(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	const start, end = int64(1785513600), int64(1788191999)
	ctx := context.Background()
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.NoError(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1))
	var retention BillingStatementRetention
	require.NoError(t, db.Where("user_id = ? AND period_start = ?", 91, start).First(&retention).Error)
	var a billingSourceAttestation
	require.NoError(t, common.UnmarshalJsonStr(string(retention.Detail), &a))
	a.Version = 2
	raw, err := common.Marshal(a)
	require.NoError(t, err)
	require.NoError(t, db.Model(&retention).Update("detail", string(raw)).Error)
	assert.ErrorIs(t, VerifyBillingStatementRetention(ctx, 91, start), ErrBillingStatementSourceIncomplete)
}
