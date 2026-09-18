package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingSourceDecisionHistoryAndRevocation(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	row := Log{UserId: 91, TokenId: 40, CreatedAt: start + 100, ModelName: "historical-video", Type: LogTypeConsume, Quota: 913500, Other: `{"is_task":true,"model_price":0}`}
	require.NoError(t, db.Create(&row).Error)
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.Issues, 1)
	d := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: r.Issues[0].ID, Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "keep-record-request"}
	// Accepting a missing explanation needs no invented free-text evidence.
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.History, 1)
	assert.True(t, r.Issues[0].Reviewed)
	assert.Equal(t, "record_kept", r.History[0].Status)
	assert.EqualValues(t, 913500, r.History[0].RecordedQuota)
	assert.Zero(t, r.History[0].StatementDelta)
	assert.Zero(t, r.History[0].BalanceDelta)
	verify := BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "fixture backup", RetentionEvidence: "fixture retention"}
	require.NoError(t, RecordBillingSourceVerification(ctx, 91, start, end, verify, 1))
	require.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
	conflicting := d
	conflicting.Note = "different retry"
	assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, conflicting, 1), ErrBillingStatementVersionConflict)
	revoke := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: d.IssueID, Decision: BillingSourceInvestigate, Reason: "amount_questioned", Note: "customer disputes the amount", RequestID: "investigate-request", PreviousID: r.Issues[0].DecisionID}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, revoke, 2))
	assert.ErrorIs(t, VerifyBillingStatementRetention(ctx, 91, start), ErrBillingStatementSourceIncomplete)
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, verify, 1), ErrBillingStatementSourceIncomplete)
	competing := revoke
	competing.RequestID = "competing-request"
	assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, competing, 1), ErrBillingStatementVersionConflict)
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.History, 2)
	assert.True(t, r.History[0].Superseded)
	assert.False(t, r.History[1].Superseded)
	assert.False(t, r.Issues[0].Reviewed)
	assert.Equal(t, "pending_review", r.History[1].Status)
	assert.Equal(t, 2, r.History[1].ActorID)
	assert.Equal(t, r.History[0].ID, r.History[1].PreviousID)
	// Changed sources keep the full decision history, but cannot certify the new amount.
	require.NoError(t, db.Model(&row).Update("quota", 913501).Error)
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.History, 2)
	assert.False(t, r.History[0].CurrentSource)
	assert.False(t, r.History[1].CurrentSource)
	assert.False(t, r.Issues[0].Reviewed)
}

func TestBillingSourceAdjustmentRequestsNeverExecuteMoney(t *testing.T) {
	for _, tc := range []struct{ decision, reason string }{
		{BillingSourceWaiver, "customer_agreement"},
		{BillingSourceDuplicate, "suspected_duplicate"},
		{BillingSourcePeriod, "wrong_period"},
	} {
		t.Run(tc.decision, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			require.NoError(t, db.AutoMigrate(&Task{}, &User{}))
			user := User{Id: 91, Username: "customer", Quota: 7000000, UsedQuota: 913500}
			require.NoError(t, db.Create(&user).Error)
			ctx := context.Background()
			const start, end = int64(1785513600), int64(1788191999)
			row := Log{UserId: 91, TokenId: 40, CreatedAt: start + 10, Type: LogTypeConsume, Quota: 913500, Other: `{"is_task":true,"model_price":0}`}
			require.NoError(t, db.Create(&row).Error)
			r, err := GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			d := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: r.Issues[0].ID, Decision: tc.decision, Reason: tc.reason, Note: "customer request; amount and refund need verification", RequestID: "adjustment-request"}
			require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
			r, err = GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			assert.Equal(t, 1, r.Pending)
			assert.EqualValues(t, 913500, r.NetQuota)
			require.Len(t, r.History, 1)
			assert.Equal(t, "pending_review", r.History[0].Status)
			assert.Zero(t, r.History[0].StatementDelta)
			assert.Zero(t, r.History[0].BalanceDelta)
			var stored User
			require.NoError(t, db.First(&stored, 91).Error)
			assert.Equal(t, user.Quota, stored.Quota)
			assert.Equal(t, user.UsedQuota, stored.UsedQuota)
			var count int64
			require.NoError(t, db.Model(&Log{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
		})
	}
}

func TestBillingSourceLegacyAndInvalidDecisionCannotAuthorize(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	row := Log{UserId: 91, CreatedAt: start + 10, Type: LogTypeRefund, Quota: 913500}
	require.NoError(t, db.Create(&row).Error)
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.Issues, 1)
	legacy, err := common.Marshal(map[string]any{"fingerprint": r.Fingerprint, "issue_id": r.Issues[0].ID, "accept": true, "note": "old free text"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&BillingStatementAudit{Action: "review_source_issue", IdempotencyKey: billingSourceReviewKey(91, start), Reason: string(legacy), ActorId: 1}).Error)
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	assert.False(t, r.Issues[0].Reviewed)
	require.Len(t, r.History, 1)
	assert.Equal(t, "legacy_note", r.History[0].Status)
	for _, d := range []BillingSourceReviewDecision{
		{Decision: "exclude_from_bill", Reason: "suspected_duplicate"},
		{Decision: BillingSourceKeep, Reason: "made_up"},
		{Decision: BillingSourceWaiver, Reason: "customer_agreement", Note: "must not waive a refund"},
		{Decision: BillingSourceInvestigate, Reason: "missing_evidence"},
	} {
		d.Fingerprint = r.Fingerprint
		d.IssueID = r.Issues[0].ID
		d.PreviousID = r.Issues[0].DecisionID
		d.RequestID = "invalid-decision-request"
		assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1), ErrBillingStatementSourceIncomplete)
	}
}

// 501 is the smallest history crossing the 500-row reader boundary. Processed
// text rows must not exhaust a retained-evidence budget on the next page.
func TestBillingSourceReviewReleasesProcessedHistoryPages(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	const start, end = int64(1785513600), int64(1788191999)
	rows := make([]Log, 501)
	for i := range rows {
		rows[i] = Log{UserId: 91, TokenId: 40, CreatedAt: start - 1, Type: LogTypeConsume, Quota: 1, PromptTokens: 1}
	}
	require.NoError(t, db.CreateInBatches(&rows, 100).Error)
	report, err := getBillingSourceReview(context.Background(), 91, start, end, 500*1024)
	require.NoError(t, err)
	assert.Zero(t, report.Rows)
	assert.Empty(t, report.Issues)
	assert.NotEmpty(t, report.Fingerprint)
}
