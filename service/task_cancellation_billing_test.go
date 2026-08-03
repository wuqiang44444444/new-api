package service

import (
	"context"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmedCancellationRefundIsAtomicAndIdempotent(t *testing.T) {
	truncate(t)
	seedUser(t, 993, 900)
	task := model.Task{
		TaskID: "cancelled-task",
		UserId: 993,
		Quota:  100,
		Status: model.TaskStatusCancelled,
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{OriginModelName: "video-model", PerCallBilling: true},
			AsyncBilling: &model.TaskAsyncBillingContext{
				State: model.TaskBillingStatePending,
			},
		},
	}
	require.NoError(t, task.Insert())

	var first, second model.Task
	require.NoError(t, model.DB.First(&first, "id = ?", task.ID).Error)
	require.NoError(t, model.DB.First(&second, "id = ?", task.ID).Error)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		SettleConfirmedTaskCancellation(context.Background(), &first)
	}()
	go func() {
		defer wait.Done()
		SettleConfirmedTaskCancellation(context.Background(), &second)
	}()
	wait.Wait()

	var user model.User
	require.NoError(t, model.DB.First(&user, "id = ?", task.UserId).Error)
	assert.Equal(t, 1000, user.Quota)
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, "id = ?", task.ID).Error)
	assert.Zero(t, saved.Quota)
	require.NotNil(t, saved.PrivateData.AsyncBilling)
	assert.Equal(t, model.TaskBillingStateSettled, saved.PrivateData.AsyncBilling.State)
	assert.Equal(t, "refund", saved.PrivateData.AsyncBilling.Operation)
	assert.Equal(t, "video task cancelled", saved.PrivateData.AsyncBilling.Reason)
	require.NotNil(t, saved.PrivateData.AsyncBilling.TargetQuota)
	assert.Zero(t, *saved.PrivateData.AsyncBilling.TargetQuota)

	var refundLogs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).
		Where("type = ? AND user_id = ? AND quota = ?", model.LogTypeRefund, task.UserId, 100).
		Count(&refundLogs).Error)
	assert.Equal(t, int64(1), refundLogs)
}
