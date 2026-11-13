package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingSourceSnapshotTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupBillingStatementVersionTestDB(t)
	if os.Getenv("TEST_BILLING_STATEMENT_DIALECT") == "" {
		// Use the production SQLite locking options against a disposable WAL database.
		var err error
		dsn := filepath.Join(t.TempDir(), "review.db") + "?_pragma=journal_mode(WAL)&_txlock=immediate"
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		pool, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, pool.Close()) })
		DB, LOG_DB = db, db
		require.NoError(t, migrateBillingStatementVersionDB())
		require.NoError(t, InitBillingStatementSourceTracking())
		require.NoError(t, db.AutoMigrate(&Log{}))
	}
	require.NoError(t, db.AutoMigrate(&Task{}))
	return db
}

func TestBillingSourceReviewSnapshotKeepsConcurrentLogForNextCheck(t *testing.T) {
	db := setupBillingSourceSnapshotTestDB(t)
	const start, end = int64(1785513600), int64(1788191999)
	const user = 91
	initial := Log{UserId: user, TokenId: 40, ModelName: "video", CreatedAt: start + 1,
		Type: LogTypeConsume, Quota: 20, Other: `{"model_price":1}`}
	require.NoError(t, db.Create(&initial).Error)

	var writeErr error
	wrote := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("review_test:concurrent_write", func(query *gorm.DB) {
		if wrote || query.Statement.Table != "logs" {
			return
		}
		wrote = true
		newLog := initial
		newLog.Id, newLog.Quota = 0, 10
		writeErr = db.Create(&newLog).Error
	}))

	ctx := context.Background()
	first, err := GetBillingSourceReview(ctx, user, start, end)
	require.NoError(t, err)
	require.True(t, wrote)
	require.NoError(t, writeErr)
	assert.EqualValues(t, 1, first.Rows)
	assert.EqualValues(t, 20, first.NetQuota)

	second, err := GetBillingSourceReview(ctx, user, start, end)
	require.NoError(t, err)
	assert.EqualValues(t, 2, second.Rows)
	assert.EqualValues(t, 30, second.NetQuota)
}

func TestBillingSourceReviewSnapshotRejectsMaintenanceGenerationChange(t *testing.T) {
	db := setupBillingSourceSnapshotTestDB(t)
	var changeErr error
	changed := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("review_test:maintenance_change", func(query *gorm.DB) {
		if changed || query.Statement.Table != "logs" {
			return
		}
		changed = true
		changeErr = db.Model(&BillingStatementMaintenance{}).Where("id = ?", billingStatementMaintenanceRowID).
			UpdateColumn("generation", gorm.Expr("generation + 1")).Error
	}))
	report, err := GetBillingSourceReview(context.Background(), 91, 1785513600, 1788191999)
	require.True(t, changed)
	require.NoError(t, changeErr)
	assert.ErrorIs(t, err, ErrBillingStatementMaintenanceActive)
	assert.Nil(t, report)
}

func TestBillingSourceReviewClosingDifferenceFollowsSingleTaskEvidence(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	const user = 91
	target := 120
	task := Task{UserId: user, AppID: 40, ChannelId: 3, TaskID: "settled-task", CreatedAt: start + 50,
		Quota: target, Properties: Properties{OriginModelName: "video"},
		PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{
			State: TaskBillingStateSettled, TargetQuota: &target,
		}}}
	require.NoError(t, db.Create(&task).Error)
	log := Log{UserId: user, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: start + 100,
		Type: LogTypeConsume, Quota: 100, Other: `{"task_id":"settled-task","model_price":1}`}
	require.NoError(t, db.Create(&log).Error)

	report, err := GetBillingSourceReview(ctx, user, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	require.True(t, report.Issues[0].CanCloseDifference)
	decision := BillingSourceReviewDecision{
		Decision: BillingSourceCloseDifference, Reason: "small_difference_reviewed", Note: "Reviewed the difference.",
		Fingerprint: report.Fingerprint, IssueID: report.Issues[0].ID, RequestID: "close-review-task-first",
	}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, user, start, end, decision, 1))
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, user, start, end, decision, 1))
	require.NoError(t, db.Create(&Log{UserId: user, TokenId: 40, ModelName: "other",
		CreatedAt: start + 101, Type: LogTypeConsume, Quota: 50, Other: `{"model_price":1}`}).Error)

	closed, err := GetBillingSourceReview(ctx, user, start, end)
	require.NoError(t, err)
	assert.NotEqual(t, report.Fingerprint, closed.Fingerprint)
	assert.Zero(t, closed.Blockers)
	assert.Zero(t, closed.Pending)
	assert.EqualValues(t, 150, closed.NetQuota)
	require.Len(t, closed.History, 1)
	assert.Equal(t, "small_difference_closed", closed.History[0].Status)
	assert.True(t, closed.History[0].CurrentSource)
	assert.Zero(t, closed.History[0].StatementDelta)
	assert.Zero(t, closed.History[0].BalanceDelta)
	require.NoError(t, RecordBillingSourceVerification(ctx, user, start, end, BillingSourceVerification{
		Fingerprint: closed.Fingerprint, BackupEvidence: "checked backup", RetentionEvidence: "checked retention",
	}, 1))
	require.NoError(t, VerifyBillingStatementRetention(ctx, user, start))

	require.NoError(t, db.Model(&log).Update("quota", 110).Error)
	reopened, err := GetBillingSourceReview(ctx, user, start, end)
	require.NoError(t, err)
	require.Len(t, reopened.Issues, 1)
	assert.Equal(t, 1, reopened.Blockers)
	assert.False(t, reopened.History[0].CurrentSource)
	assert.True(t, reopened.Issues[0].CanCloseDifference)
	assert.ErrorIs(t, VerifyBillingStatementRetention(ctx, user, start), ErrBillingStatementSourceIncomplete)

	require.NoError(t, db.Model(&log).Update("quota", 130).Error)
	opposite, err := GetBillingSourceReview(ctx, user, start, end)
	require.NoError(t, err)
	require.Len(t, opposite.Issues, 1)
	assert.False(t, opposite.Issues[0].CanCloseDifference)
	decision.Fingerprint, decision.PreviousID, decision.RequestID = opposite.Fingerprint, opposite.History[0].ID, "close-review-task-second"
	assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, user, start, end, decision, 1), ErrBillingStatementSourceIncomplete)
}

func TestBillingSourceReviewRejectsCrossMonthDuplicatePreauthRefund(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	target := 120
	task := Task{UserId: 91, AppID: 40, ChannelId: 3, TaskID: "audit-task", CreatedAt: start + 1, Quota: 120, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, TargetQuota: &target}}}
	require.NoError(t, db.Create(&task).Error)
	charge := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: start + 2, Type: LogTypeConsume, Quota: 120, Other: `{"task_id":"audit-task","model_price":1}`}
	require.NoError(t, db.Create(&charge).Error)
	for _, at := range []int64{start + 3, end + 1} {
		refund := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: at, Type: LogTypeRefund, Quota: 10, Other: `{"model_price":1,"admin_info":{"original_preauth_log_id":` + strconv.Itoa(charge.Id) + `}}`}
		require.NoError(t, db.Create(&refund).Error)
	}
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	for _, issue := range report.Issues {
		if issue.Kind == "task_amount" {
			assert.False(t, issue.CanCloseDifference)
			require.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: issue.ID, Decision: BillingSourceCloseDifference, Reason: "small_difference_reviewed", Note: "duplicate refund test", RequestID: "audit-cross-month-refund"}, 1), ErrBillingStatementSourceIncomplete)
			closed, err := GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			assert.Equal(t, 1, closed.Blockers, "invalid links must keep task issue blocked")
			return
		}
	}
	t.Fatal("task issue not found")
}

func TestBillingSourceReviewSaveIgnoresOnlyUnrelatedConcurrentFunding(t *testing.T) {
	for _, inMonth := range []bool{false, true} {
		t.Run(fmt.Sprintf("in_month_%t", inMonth), func(t *testing.T) {
			db := setupBillingSourceSnapshotTestDB(t)
			const start, end = int64(1785513600), int64(1788191999)
			ctx := context.Background()
			log := Log{UserId: 91, TokenId: 40, ModelName: "text", CreatedAt: start + 1, Type: LogTypeConsume, Quota: 20, Other: `{"model_price":1}`}
			require.NoError(t, db.Create(&log).Error)
			r, err := GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			var writeErr error
			wrote := false
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("review_test:submit_concurrent_write", func(query *gorm.DB) {
				if wrote || query.Statement.Table != "logs" {
					return
				}
				wrote = true
				next := log
				next.Id = 0
				if !inMonth {
					next.CreatedAt = end + 1
				}
				writeErr = db.Create(&next).Error
			}))
			err = RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "fixture backup", RetentionEvidence: "fixture retention"}, 1)
			require.True(t, wrote)
			require.NoError(t, writeErr)
			if inMonth {
				assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
			} else {
				require.NoError(t, err)
				assert.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
			}
		})
	}
}

func TestBillingSourceReviewReopensCloseAfterCrossMonthDuplicate(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	target := 120
	task := Task{UserId: 91, AppID: 40, ChannelId: 3, TaskID: "audit-task", CreatedAt: start + 1, Quota: 120, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, TargetQuota: &target}}}
	require.NoError(t, db.Create(&task).Error)
	charge := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: start + 2, Type: LogTypeConsume, Quota: 120, Other: `{"task_id":"audit-task","model_price":1}`}
	require.NoError(t, db.Create(&charge).Error)
	refund := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: end + 1, Type: LogTypeRefund, Quota: 120, Other: `{"model_price":1,"admin_info":{"original_preauth_log_id":` + strconv.Itoa(charge.Id) + `}}`}
	require.NoError(t, db.Create(&refund).Error)
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	require.True(t, report.Issues[0].CanCloseDifference)
	require.EqualValues(t, 0, report.Issues[0].Quota)
	require.EqualValues(t, 120, report.NetQuota)
	req := BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: report.Issues[0].ID, Decision: BillingSourceCloseDifference, Reason: "small_difference_reviewed", Note: "independent review", RequestID: "independent-first"}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, req, 1))
	report, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Zero(t, report.Blockers)
	refund.Id = 0
	refund.CreatedAt += 86400
	require.NoError(t, db.Create(&refund).Error)
	report, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	assert.Equal(t, 1, report.Blockers)
	assert.False(t, report.Issues[0].CanCloseDifference)
	assert.EqualValues(t, 120, report.NetQuota)
	req.Fingerprint, req.PreviousID, req.RequestID = report.Fingerprint, report.History[0].ID, "independent-second"
	assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, req, 1), ErrBillingStatementSourceIncomplete)
}

func TestBillingSourceReviewRejectsDualReferenceRefundForBothTasks(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	targetA, targetB := 10, 120
	for _, spec := range []struct {
		id     string
		target *int
	}{{"audit-a", &targetA}, {"audit-b", &targetB}} {
		task := Task{UserId: 91, AppID: 40, ChannelId: 3, TaskID: spec.id, CreatedAt: start + 1, Quota: *spec.target, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, TargetQuota: spec.target}}}
		require.NoError(t, db.Create(&task).Error)
	}
	a := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", Type: LogTypeConsume, CreatedAt: start + 2, Quota: 10, Other: `{"task_id":"audit-a","model_price":1}`}
	b := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", Type: LogTypeConsume, CreatedAt: start + 3, Quota: 120, Other: `{"task_id":"audit-b","model_price":1}`}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	refund := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: end + 1, Type: LogTypeRefund, Quota: 10, Other: `{"task_id":"audit-b","model_price":1,"admin_info":{"original_preauth_log_id":` + strconv.Itoa(a.Id) + `}}`}
	require.NoError(t, db.Create(&refund).Error)
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 2)
	assert.Equal(t, 2, report.Blockers)
	assert.EqualValues(t, 130, report.NetQuota)
	for i, issue := range report.Issues {
		assert.False(t, issue.CanCloseDifference)
		assert.Contains(t, issue.Reasons, "identity_conflict")
		require.NotNil(t, issue.TargetQuota)
		assert.Equal(t, *issue.TargetQuota, issue.Quota)
		assert.NotContains(t, issue.RelatedLogIDs, refund.Id)
		assert.Contains(t, issue.RelatedLogs, BillingSourceRelatedLog{
			ID: refund.Id, CreatedAt: refund.CreatedAt, LogType: LogTypeRefund, Quota: 10, IdentityMatches: false,
		})
		req := BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: issue.ID, Decision: BillingSourceCloseDifference, Reason: "small_difference_reviewed", Note: "independent review", RequestID: fmt.Sprintf("independent-dual-reference-%d", i)}
		assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, req, 1), ErrBillingStatementSourceIncomplete)
	}
}

func TestBillingSourceReviewReopensCloseForMalformedCrossMonthDuplicate(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	const start, end = int64(1785513600), int64(1788191999)
	target := 120
	task := Task{UserId: 91, AppID: 40, ChannelId: 3, TaskID: "audit-task", CreatedAt: start + 1, Quota: 120, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, TargetQuota: &target}}}
	require.NoError(t, db.Create(&task).Error)
	charge := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: start + 2, Type: LogTypeConsume, Quota: 120, Other: `{"task_id":"audit-task","model_price":1}`}
	require.NoError(t, db.Create(&charge).Error)
	refund := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: end + 1, Type: LogTypeRefund, Quota: 120, Other: `{"model_price":1,"admin_info":{"original_preauth_log_id":` + strconv.Itoa(charge.Id) + `}}`}
	require.NoError(t, db.Create(&refund).Error)
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	require.True(t, report.Issues[0].CanCloseDifference)
	require.EqualValues(t, 0, report.Issues[0].Quota)
	require.EqualValues(t, 120, report.NetQuota)
	req := BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: report.Issues[0].ID, Decision: BillingSourceCloseDifference, Reason: "small_difference_reviewed", Note: "independent review", RequestID: "independent-first"}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, req, 1))
	report, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Zero(t, report.Blockers)
	refund.Id = 0
	refund.CreatedAt += 86400
	refund.Other = strings.Replace(refund.Other, "{", `{"task_id":123,`, 1)
	require.NoError(t, db.Create(&refund).Error)
	report, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	assert.Equal(t, 1, report.Blockers)
	assert.False(t, report.Issues[0].CanCloseDifference)
	assert.False(t, report.Issues[0].Reviewed)
	assert.Contains(t, report.Issues[0].RelatedLogs, BillingSourceRelatedLog{
		ID: refund.Id, CreatedAt: refund.CreatedAt, LogType: LogTypeRefund, Quota: 120, IdentityMatches: false,
	})
	require.Len(t, report.History, 1)
	assert.False(t, report.History[0].CurrentSource)
	assert.EqualValues(t, 120, report.NetQuota)
	req.Fingerprint, req.PreviousID, req.RequestID = report.Fingerprint, report.History[0].ID, "independent-second"
	assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, req, 1), ErrBillingStatementSourceIncomplete)
}
