package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingSourceReviewBudgetAbortsWithoutPartialReport(t *testing.T) {
	for _, kind := range []string{"historical links", "monthly issues", "task facts"} {
		t.Run(kind, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			require.NoError(t, db.AutoMigrate(&Task{}))
			start, end := int64(1785513600), int64(1788191999)
			switch kind {
			case "historical links":
				require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, Type: LogTypeConsume, CreatedAt: start - 1, Quota: 10, Other: `{"task_id":"old-task"}`}).Error)
			case "monthly issues":
				require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, Type: LogTypeConsume, CreatedAt: start + 1, Quota: 10, Other: `{"is_task":true,"model_price":0}`}).Error)
			case "task facts":
				require.NoError(t, db.Create(&Task{UserId: 91, AppID: 40, TaskID: "task", CreatedAt: start + 1}).Error)
			}
			ctx := context.Background()
			budget := 1000
			if kind == "monthly issues" {
				budget = 1500 // The source row fits; retaining its issue must fail.
			}
			report, err := getBillingSourceReview(ctx, 91, start, end, budget)
			assert.ErrorIs(t, err, errBillingSourceReviewBudget)
			assert.Nil(t, report, "no partial result or fingerprint may escape the budget fence")
			var audits, retention int64
			require.NoError(t, db.Model(&BillingStatementAudit{}).Count(&audits).Error)
			require.NoError(t, db.Model(&BillingStatementRetention{}).Count(&retention).Error)
			assert.Zero(t, audits)
			assert.Zero(t, retention)
			complete, err := GetBillingSourceReview(ctx, 91, start, end)
			require.NoError(t, err)
			assert.NotEmpty(t, complete.Fingerprint)
		})
	}
}

func TestBillingSourceReviewDuplicateTaskIdentityRemainsBlocking(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	start, end := int64(1785513600), int64(1788191999)
	target := 10
	for _, at := range []int64{start + 1, end + 1} {
		require.NoError(t, db.Create(&Task{UserId: 91, AppID: 40, TaskID: "duplicate", CreatedAt: at, ChannelId: 3, Quota: target, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 40, AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateSettled, TargetQuota: &target}}}).Error)
	}
	require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ChannelId: 3, ModelName: "video", Type: LogTypeConsume, CreatedAt: start + 2, Quota: target, Other: `{"task_id":"duplicate","model_price":1}`}).Error)
	report, err := GetBillingSourceReview(context.Background(), 91, start, end)
	require.NoError(t, err)
	assert.Equal(t, 2, report.Blockers, "an out-of-month duplicate owner must not be lost by page-scoped grouping")
}
