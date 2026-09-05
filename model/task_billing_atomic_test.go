package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyTaskBillingTargetClassifiesFundingShortfallsAndRollsBack(t *testing.T) {
	t.Run("wallet", func(t *testing.T) {
		truncateTables(t)
		user := User{Username: "billing-wallet-shortfall", Quota: 100}
		require.NoError(t, DB.Create(&user).Error)
		task := persistedBillingTargetTask(t, user.Id, 0, "wallet", 0)

		applied, _, err := ApplyTaskBillingTarget(task, 800)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTaskBillingInsufficientFunding))
		assert.False(t, applied)
		assertBillingTargetUnchanged(t, task.ID, user.Id, 0, 100)
	})

	t.Run("token", func(t *testing.T) {
		truncateTables(t)
		user := User{Username: "billing-token-shortfall", Quota: 1000}
		require.NoError(t, DB.Create(&user).Error)
		token := Token{
			UserId: user.Id, Key: "billing-token-shortfall", Status: common.TokenStatusEnabled,
			RemainQuota: 100,
		}
		require.NoError(t, DB.Create(&token).Error)
		task := persistedBillingTargetTask(t, user.Id, token.Id, "wallet", 0)

		applied, _, err := ApplyTaskBillingTarget(task, 800)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTaskBillingInsufficientFunding))
		assert.False(t, applied)
		assertBillingTargetUnchanged(t, task.ID, user.Id, token.Id, 1000)
	})

	t.Run("subscription", func(t *testing.T) {
		truncateTables(t)
		user := User{Username: "billing-subscription-shortfall", Quota: 1000}
		require.NoError(t, DB.Create(&user).Error)
		subscription := UserSubscription{UserId: user.Id, AmountTotal: 500, AmountUsed: 100, Status: "active"}
		require.NoError(t, DB.Create(&subscription).Error)
		task := persistedBillingTargetTask(t, user.Id, 0, "subscription", subscription.Id)

		applied, _, err := ApplyTaskBillingTarget(task, 800)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrTaskBillingInsufficientFunding))
		assert.False(t, applied)
		assertBillingTargetUnchanged(t, task.ID, user.Id, 0, 1000)
		var reloaded UserSubscription
		require.NoError(t, DB.First(&reloaded, subscription.Id).Error)
		assert.Equal(t, int64(100), reloaded.AmountUsed)
	})

	t.Run("missing user is a state error", func(t *testing.T) {
		truncateTables(t)
		task := persistedBillingTargetTask(t, 999999, 0, "wallet", 0)

		applied, _, err := ApplyTaskBillingTarget(task, 800)

		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrTaskBillingInsufficientFunding))
		assert.False(t, applied)
	})
}

func persistedBillingTargetTask(t *testing.T, userID, tokenID int, fundingSource string, subscriptionID int) *Task {
	t.Helper()
	task := &Task{
		TaskID: GenerateTaskID(), UserId: userID, Quota: 0, Status: TaskStatus(TaskStatusSuccess),
		PrivateData: TaskPrivateData{
			BillingSource: fundingSource, SubscriptionId: subscriptionID, TokenId: tokenID,
			AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStatePending},
		},
	}
	require.NoError(t, DB.Create(task).Error)
	return task
}

func assertBillingTargetUnchanged(t *testing.T, taskID int64, userID, tokenID, expectedUserQuota int) {
	t.Helper()
	var task Task
	require.NoError(t, DB.First(&task, taskID).Error)
	assert.Zero(t, task.Quota)
	assert.Equal(t, TaskBillingStatePending, task.PrivateData.AsyncBilling.State)
	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	assert.Equal(t, expectedUserQuota, user.Quota)
	if tokenID == 0 {
		return
	}
	var token Token
	require.NoError(t, DB.First(&token, tokenID).Error)
	assert.Equal(t, 100, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}
