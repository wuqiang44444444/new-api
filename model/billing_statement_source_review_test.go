package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingSourceReviewWorkflow(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	start := int64(1785513600)
	end := int64(1788191999)
	log := Log{UserId: 91, TokenId: 40, ModelName: "old-video", CreatedAt: start + 100, Type: LogTypeConsume, Quota: 913500, Other: `{"is_task":true,"model_price":0}`}
	require.NoError(t, db.Create(&log).Error)
	report, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, report.Issues, 1)
	assert.Equal(t, "unknown_billing_mode", report.Issues[0].Reasons[0])
	assert.EqualValues(t, 913500, report.NetQuota)
	verification := BillingSourceVerification{Fingerprint: report.Fingerprint, BackupEvidence: "backup scope checked", RetentionEvidence: "no cleanup or recovery gaps"}
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, verification, 1), ErrBillingStatementSourceIncomplete)
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: report.Issues[0].ID, Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "test-request-first", Note: "historical explanation missing; evidence reviewed"}, 1))
	require.NoError(t, RecordBillingSourceVerification(ctx, 91, start, end, verification, 1))
	require.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
	var stored Log
	require.NoError(t, db.First(&stored, log.Id).Error)
	assert.Equal(t, log.Quota, stored.Quota)
	assert.Equal(t, log.Other, stored.Other)
	// An unrelated month's consumption does not invalidate this month.
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 99, CreatedAt: end + 100, Type: LogTypeConsume, Quota: 10, PromptTokens: 1}).Error)
	require.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
	unchanged, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	assert.Equal(t, report.Fingerprint, unchanged.Fingerprint)
	assert.True(t, unchanged.Issues[0].Reviewed)
	require.NoError(t, db.Model(&log).Update("quota", 913501).Error)
	assert.ErrorIs(t, VerifyBillingStatementRetention(ctx, 91, start), ErrBillingStatementSourceIncomplete)
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, verification, 1), ErrBillingStatementVersionConflict)
}

func TestBillingSourceReviewAmountCannotBeAcknowledged(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	start := int64(1785513600)
	end := int64(1788191999)
	target := 120
	task := Task{UserId: 91, AppID: 40, ChannelId: 3, TaskID: "settled-task", CreatedAt: start + 50, Quota: target, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, TargetQuota: &target}}}
	require.NoError(t, db.Create(&task).Error)
	initial := Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: start + 100, Type: LogTypeConsume, Quota: 0, Other: `{"task_id":"settled-task","model_price":1}`}
	require.NoError(t, db.Create(&initial).Error)
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: end + 100, Type: LogTypeConsume, Quota: 20, Other: `{"task_id":"settled-task","actual_quota":120,"pre_consumed_quota":100,"model_price":1}`}).Error)
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Equal(t, 1, r.Blockers)
	var issue BillingSourceIssue
	for _, i := range r.Issues {
		if i.Blocking {
			issue = i
		}
	}
	assert.EqualValues(t, 20, issue.Quota)
	require.NotNil(t, issue.TargetQuota)
	assert.EqualValues(t, 120, *issue.TargetQuota)
	require.Len(t, issue.RelatedLogIDs, 2)
	d := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: issue.ID, Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "test-request-first", Note: "cannot waive money"}
	assert.ErrorIs(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1), ErrBillingStatementSourceIncomplete)
	d.Decision, d.Reason = BillingSourceInvestigate, "amount_questioned"
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, d, 1))
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1), ErrBillingStatementSourceIncomplete)
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Len(t, r.PendingRequests, 1)
	close := BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: issue.ID, PreviousID: r.PendingRequests[0].ID, Decision: BillingSourceReject, Reason: "request_rejected", Note: "business request rejected; amount evidence still needs correction", RequestID: "reject-with-open-amount"}
	require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, close, 1))
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	assert.Empty(t, r.PendingRequests)
	assert.Equal(t, 1, r.Blockers, "closing a business request never clears a real amount difference")
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1), ErrBillingStatementSourceIncomplete)
	// Simulate an approved external correction on the isolated fixture, never the real DB.
	require.NoError(t, db.Model(&initial).Update("quota", 100).Error)
	r, err = GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	assert.Zero(t, r.Blockers)
}

func TestBillingSourceReviewFailsClosedAndKeepsSettlementScope(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	start := int64(1785513600)
	end := int64(1788191999)
	require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 0, TokenName: "模型测试", Content: "模型测试", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100}).Error)
	require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 0, Type: LogTypeConsume, CreatedAt: start + 2, Quota: 25, PromptTokens: 1, Other: `{"request_path":"/mj/submit/imagine"}`}).Error)
	r, err := GetBillingSourceReview(ctx, 11, start, end)
	require.NoError(t, err)
	assert.EqualValues(t, 1, r.Rows)
	assert.EqualValues(t, 25, r.NetQuota)
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = GetBillingSourceReview(cancelCtx, 11, start, end)
	require.Error(t, err)
	_, err = GetBillingSourceReview(ctx, 11, start+1, end)
	assert.Error(t, err)
	require.NoError(t, upsertBillingStatementRetentionTx(db, 11, start, BillingStatementRetentionIntact, "legacy text", time.Now().Unix()))
	assert.ErrorIs(t, VerifyBillingStatementRetention(ctx, 11, start), ErrBillingStatementSourceIncomplete)
	assert.ErrorIs(t, RecordBillingSourceVerification(ctx, 11, start, end, BillingSourceVerification{}, 1), ErrBillingStatementSourceIncomplete)
	raw, err := common.Marshal(r)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "private_data")
	assert.NotContains(t, string(raw), "Other")
	require.NoError(t, db.Migrator().DropTable(&Task{}))
	_, err = GetBillingSourceReview(ctx, 11, start, end)
	assert.Error(t, err)
}

func TestBillingSourceReviewTracksFutureTaskRelations(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	start := int64(1785513600)
	end := int64(1788191999)
	target := 100
	task := Task{UserId: 91, AppID: 40, ChannelId: 3, TaskID: "balanced-task", CreatedAt: start + 50, Quota: target, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, TargetQuota: &target}}}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: start + 100, Type: LogTypeConsume, Quota: 100, Other: `{"task_id":"balanced-task","model_price":1}`}).Error)
	r, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Zero(t, r.Blockers)
	for _, i := range r.Issues {
		require.NoError(t, RecordBillingSourceReviewDecision(ctx, 91, start, end, BillingSourceReviewDecision{Fingerprint: r.Fingerprint, IssueID: i.ID, Decision: BillingSourceKeep, Reason: "accept_missing_details", RequestID: "test-request-first", Note: "fixture checked"}, 1))
	}
	require.NoError(t, RecordBillingSourceVerification(ctx, 91, start, end, BillingSourceVerification{Fingerprint: r.Fingerprint, BackupEvidence: "backup", RetentionEvidence: "retention"}, 1))
	require.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
	// The same opaque ID in another application is a separate identity.
	other := task
	other.ID = 0
	other.AppID = 41
	other.PrivateData.TokenId = 41
	other.CreatedAt = end + 100
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 41, ChannelId: 3, ModelName: "video", CreatedAt: end + 101, Type: LogTypeConsume, Quota: 100, Other: `{"task_id":"balanced-task","model_price":1}`}).Error)
	unchanged, err := GetBillingSourceReview(ctx, 91, start, end)
	require.NoError(t, err)
	require.Zero(t, unchanged.Blockers)
	assert.Equal(t, r.Fingerprint, unchanged.Fingerprint)
	require.NoError(t, VerifyBillingStatementRetention(ctx, 91, start))
	// A later related consume log must invalidate prior source approval without
	// any change to the task or an existing August log.
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", CreatedAt: end + 100, Type: LogTypeConsume, Quota: 20, Other: `{"task_id":"balanced-task","model_price":1}`}).Error)
	assert.ErrorIs(t, VerifyBillingStatementRetention(ctx, 91, start), ErrBillingStatementSourceIncomplete)
}

func TestBillingSourceReviewRetentionMigrationPreservesEvidence(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&BillingStatementRetention{}))
	// Reproduce the pre-review TEXT schema, then run the real upgrade twice.
	type legacyRetention struct {
		ID          int64                           `gorm:"primary_key"`
		UserId      int                             `gorm:"uniqueIndex:uidx_bsret_user_period,priority:1"`
		PeriodStart int64                           `gorm:"bigint;uniqueIndex:uidx_bsret_user_period,priority:2"`
		Status      BillingStatementRetentionStatus `gorm:"type:varchar(16);index"`
		Detail      string                          `gorm:"type:text"`
		UpdatedAt   int64                           `gorm:"bigint"`
	}
	require.NoError(t, db.Table("billing_statement_retentions").AutoMigrate(&legacyRetention{}))
	require.NoError(t, db.Table("billing_statement_retentions").Create(&legacyRetention{UserId: 1, PeriodStart: 1785513600, Status: BillingStatementRetentionPartial, Detail: "historical source gap"}).Error)
	require.NoError(t, migrateBillingStatementVersionDB())
	require.NoError(t, migrateBillingStatementVersionDB())
	old, err := GetBillingStatementRetention(context.Background(), 1, 1785513600)
	require.NoError(t, err)
	require.NotNil(t, old)
	assert.Equal(t, "historical source gap", string(old.Detail))
	assert.Equal(t, BillingStatementRetentionPartial, old.Status)
	evidence := strings.Repeat("verified-dependency;", 5000)
	require.NoError(t, upsertBillingStatementRetentionTx(db, 2, 1785513600, BillingStatementRetentionIntact, evidence, nowSeconds()))
	stored, err := GetBillingStatementRetention(context.Background(), 2, 1785513600)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, evidence, string(stored.Detail))
}
