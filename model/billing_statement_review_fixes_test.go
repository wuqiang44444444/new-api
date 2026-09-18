package model

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBillingStatementTrackedTableMigrationPreservesRevision(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	task := Task{UserId: 11, AppID: 3, TaskID: "old-task"}
	require.NoError(t, db.Create(&task).Error)
	scope := billingStatementTaskScope(11, 3, "old-task")
	require.NoError(t, db.Create(&BillingStatementRevision{Scope: scope}).Error)
	require.NoError(t, db.Table("tasks").Where("id = ?", task.ID).UpdateColumn("private_data", `{}`).Error)
	var revision BillingStatementRevision
	require.NoError(t, db.Where("scope = ?", scope).First(&revision).Error)
	assert.EqualValues(t, 1, revision.Revision)
}

func TestBillingStatementTaskIdentityChangeInvalidatesDependentMonth(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	ctx := context.Background()
	month := int64(1756656000)
	task := Task{UserId: 11, AppID: 3, TaskID: "old-task"}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 3, Type: LogTypeRefund, Quota: 10, CreatedAt: month + 100, Other: `{"task_id":"old-task","model_price":0}`}).Error)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, draft)
	require.NoError(t, db.Model(&task).Update("task_id", "new-task").Error)
	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "confirm", "", "", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
}

func TestBillingStatementMapTaskCreateTracksMissingEvidence(t *testing.T) {
	for _, withModel := range []bool{true, false} {
		t.Run(fmt.Sprint(withModel), func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			require.NoError(t, db.AutoMigrate(&Task{}))
			scope := billingStatementTaskScope(11, 3, "new-task")
			require.NoError(t, db.Create(&BillingStatementRevision{Scope: scope}).Error)
			query := db.Table("tasks")
			if withModel {
				query = db.Model(&Task{})
			}
			require.NoError(t, query.Create(map[string]interface{}{"user_id": 11, "app_id": 3, "task_id": "new-task"}).Error)
			var revision BillingStatementRevision
			require.NoError(t, db.Where("scope = ?", scope).First(&revision).Error)
			assert.EqualValues(t, 1, revision.Revision)
		})
	}
}

func TestBillingStatementMultipleEvidenceBatchesRemainConfirmableUntilChange(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	month := int64(1756656000)
	refunds := make([]Log, 501)
	for i := range refunds {
		refunds[i] = Log{UserId: 11, TokenId: 3, Type: LogTypeRefund, Quota: 1, CreatedAt: month + 100, Other: fmt.Sprintf(`{"task_id":"refund-%d","model_price":0}`, i)}
	}
	require.NoError(t, db.CreateInBatches(&refunds, 100).Error)
	vector, err := SnapshotBillingStatementDependencyVector(context.Background(), 11, month)
	require.NoError(t, err)
	changed, err := BillingStatementVectorChanged(context.Background(), vector)
	require.NoError(t, err)
	assert.False(t, changed, "unchanged sources across all batches must remain eligible")
	require.NoError(t, db.Create(&Task{UserId: 11, AppID: 3, TaskID: "refund-500"}).Error)
	changed, err = BillingStatementVectorChanged(context.Background(), vector)
	require.NoError(t, err)
	assert.True(t, changed, "late evidence must invalidate the frozen month")
}
