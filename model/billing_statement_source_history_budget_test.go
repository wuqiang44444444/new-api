package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSourceHistoryUsesRemainingReviewBudget(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	const start, end = int64(1785513600), int64(1788191999)
	ctx := context.Background()
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	raw, err := common.Marshal(BillingSourceDecisionRecord{Version: 2, UserID: 91, PeriodStart: start, ActorID: 7, Status: "record_kept", BillingSourceReviewDecision: BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: "old-log", Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "history-budget-fixture", Note: strings.Repeat("x", 3000)}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&BillingStatementAudit{Action: "review_source_issue", IdempotencyKey: billingSourceReviewKey(91, start), Reason: string(raw)}).Error)
	limited, err := getBillingSourceReview(ctx, 91, start, end, 4096)
	assert.ErrorIs(t, err, errBillingSourceReviewBudget)
	assert.Nil(t, limited, "historical decisions must not bypass the scan budget")
	complete, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, complete.History, 1)
	assert.Equal(t, strings.Repeat("x", 3000), complete.History[0].Note)
}

func TestSourceHistoryLockedReloadReusesOnlyRemainingBudget(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	const start, end = int64(1785513600), int64(1788191999)
	ctx := context.Background()
	raw, err := common.Marshal(BillingSourceDecisionRecord{Version: 2, UserID: 91, PeriodStart: start, ActorID: 7, Status: "record_kept", BillingSourceReviewDecision: BillingSourceReviewDecision{Fingerprint: "older-source", IssueID: "old-log", Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "history-budget-fixture", Note: "first decision"}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&BillingStatementAudit{Action: "review_source_issue", IdempotencyKey: billingSourceReviewKey(91, start), Reason: string(raw)}).Error)
	report, err := getBillingSourceReview(ctx, 91, start, end, 4096)
	require.NoError(t, err)
	require.Len(t, report.History, 1)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if _, err := lockBillingStatementMaintenanceTx(ctx, tx); err != nil {
			return err
		}
		return loadBillingSourceReviewNotes(ctx, tx, report)
	}), "replacement read must not charge already loaded history twice")
	require.Len(t, report.History, 1)
	raw, err = common.Marshal(BillingSourceDecisionRecord{Version: 2, UserID: 91, PeriodStart: start, ActorID: 7, Status: "record_kept", BillingSourceReviewDecision: BillingSourceReviewDecision{Fingerprint: "older-source", IssueID: "old-log", Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "history-budget-fixture", Note: strings.Repeat("x", 3000)}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&BillingStatementAudit{Action: "review_source_issue", IdempotencyKey: billingSourceReviewKey(91, start), Reason: string(raw)}).Error)
	err = db.Transaction(func(tx *gorm.DB) error {
		if _, err := lockBillingStatementMaintenanceTx(ctx, tx); err != nil {
			return err
		}
		return loadBillingSourceReviewNotes(ctx, tx, report)
	})
	assert.ErrorIs(t, err, errBillingSourceReviewBudget, "locked reread must not reset to a fresh full scan budget")
}

func TestSourceHistoryPaginationPreservesLatestDecision(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	const start, end = int64(1785513600), int64(1788191999)
	ctx := context.Background()
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, CreatedAt: start + 1, Type: LogTypeConsume, Quota: 10, Other: `{"is_task":true,"model_price":0}`}).Error)
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	var rows []BillingStatementAudit
	// 101 is the smallest history crossing the 100-row reader boundary.
	for i := 0; i < 101; i++ {
		d := BillingSourceDecisionRecord{Version: 2, UserID: 91, PeriodStart: start, ActorID: 7, Status: "record_kept", BillingSourceReviewDecision: BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: report.Issues[0].ID, Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: fmt.Sprintf("fixture-request-%03d", i)}}
		if i == 100 {
			d.Decision, d.Note, d.Reason, d.Status = BillingSourceInvestigate, "latest decision needs investigation", "missing_evidence", "pending_review"
		}
		raw, err := common.Marshal(d)
		require.NoError(t, err)
		rows = append(rows, BillingStatementAudit{Action: "review_source_issue", IdempotencyKey: billingSourceReviewKey(91, start), Reason: string(raw), ActorId: 7})
	}
	require.NoError(t, db.CreateInBatches(&rows, 50).Error)
	report, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.History, 101)
	assert.Equal(t, rows[0].ID, report.History[0].ID)
	assert.True(t, report.History[0].Superseded)
	assert.False(t, report.History[100].Superseded)
	assert.Equal(t, rows[100].ID, report.Issues[0].DecisionID)
	assert.False(t, report.Issues[0].Reviewed)
	assert.Equal(t, "latest decision needs investigation", report.Issues[0].Note)
	assert.Equal(t, 1, report.Pending)
}
