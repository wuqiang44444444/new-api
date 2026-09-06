package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImageAcceptanceRollsBackWalletTokenTaskAndCapacity(t *testing.T) {
	for _, failure := range []string{"token_quota", "task_insert", "idempotency"} {
		t.Run(failure, func(t *testing.T) {
			cleanupImageTaskFixtures(t)
			user := &User{Id: 1010, Username: "image-atomic", Quota: 1000, AffCode: "img1010"}
			require.NoError(t, DB.Unscoped().Where("id = ?", user.Id).Delete(&User{}).Error)
			require.NoError(t, DB.Create(user).Error)
			token := &Token{UserId: user.Id, Key: common.GetUUID(), RemainQuota: 1000}
			if failure == "token_quota" {
				token.RemainQuota = 10
			}
			require.NoError(t, DB.Create(token).Error)
			t.Cleanup(func() { DB.Unscoped().Delete(token) })
			task := newImageTaskForTest(user.Id, 500)
			task.PrivateData.TokenId = token.Id
			task.PrivateData.AsyncBilling = &TaskAsyncBillingContext{State: TaskBillingStatePending}
			if failure == "task_insert" {
				const callback = "image-acceptance-fault"
				require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "tasks" {
						tx.AddError(errors.New("injected task write failure"))
					}
				}))
				t.Cleanup(func() { DB.Callback().Create().Remove(callback) })
			}
			params := ImageTaskInsertParams{Task: task, GlobalScope: ImageTaskAdmissionScopeGlobal(), AppScope: ImageTaskAdmissionScopeApp(user.Id, 7)}
			if failure == "idempotency" {
				params.IdempotencyID = -1
			}
			require.Error(t, InsertImageTask(params))
			var actual User
			require.NoError(t, DB.First(&actual, user.Id).Error)
			assert.Equal(t, 1000, actual.Quota)
			var actualToken Token
			require.NoError(t, DB.First(&actualToken, token.Id).Error)
			assert.Equal(t, token.RemainQuota, actualToken.RemainQuota)
			assert.Zero(t, actualToken.UsedQuota)
			var count int64
			require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
			assert.Zero(t, count)
			var slots []ImageTaskSlot
			require.NoError(t, DB.Find(&slots).Error)
			for _, slot := range slots {
				assert.Zero(t, slot.Count)
			}
		})
	}
}

func TestImageTerminalReleaseSurvivesRebuildAndStaleWorker(t *testing.T) {
	cleanupImageTaskFixtures(t)
	task := newImageTaskForTest(1010, 100)
	insertImageTaskForTest(t, task)
	claimed, won, err := ClaimImageTask(task.TaskID, ImageTaskExecutionScopeGlobal(), 3, ImageTaskExecutionScopeChannel(42), 3)
	require.NoError(t, err)
	require.True(t, won)
	stale, err := common.DeepCopy(claimed)
	require.NoError(t, err)
	won, err = FinishImageTaskFailure(claimed, TaskStatusFailure, "rejected")
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, RebuildImageTaskSlots())
	other := newImageTaskForTest(1009, 100)
	insertImageTaskForTest(t, other)
	won, err = FinishImageTaskFailure(stale, TaskStatusFailure, "rejected")
	require.NoError(t, err)
	assert.False(t, won)
	var slot ImageTaskSlot
	require.NoError(t, DB.First(&slot, "scope = ?", ImageTaskAdmissionScopeGlobal()).Error)
	assert.Equal(t, 1, slot.Count, "stale terminal writes cannot release another task's slot")
}
