package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskCreateAttemptUnlimitedTokenTracksUsageAndRelease(t *testing.T) {
	truncateTables(t)
	user := User{Id: 1201, Username: "attempt-unlimited", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{
		UserId: user.Id, Key: "attempt-unlimited-token",
		Status: common.TokenStatusEnabled, UnlimitedQuota: true,
	}
	require.NoError(t, DB.Create(&token).Error)
	attempt := createBillingInvariantAttempt(t, user.Id, token.Id, "unlimited")

	hold, err := HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "wallet", Quota: 25,
	})
	require.NoError(t, err)
	assert.True(t, hold.TokenTracked)
	assert.True(t, hold.TokenDebited)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, -25, token.RemainQuota)
	assert.Equal(t, 25, token.UsedQuota)

	_, err = ReleaseTaskCreateAttemptHold(attempt.ID, TaskCreateAttemptRejected)
	require.NoError(t, err)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Zero(t, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Empty(t, attempt.FrozenConnectionSnapshot)
}

func TestTaskCreateAttemptPlaygroundNeverAdjustsTokenQuota(t *testing.T) {
	truncateTables(t)
	user := User{Id: 1202, Username: "attempt-playground", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{
		UserId: user.Id, Key: "attempt-playground-token",
		Status: common.TokenStatusEnabled, RemainQuota: 50,
	}
	require.NoError(t, DB.Create(&token).Error)
	attempt := createBillingInvariantAttempt(t, user.Id, token.Id, "playground")

	hold, err := HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "wallet", Quota: 20, IsPlayground: true,
	})
	require.NoError(t, err)
	assert.False(t, hold.TokenTracked)
	assert.False(t, hold.TokenDebited)

	task := billingInvariantTask(attempt, user.Id, 10)
	require.NoError(t, RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task))
	require.NoError(t, InsertTaskWithCreateAttempt(task, 0, attempt.ID))
	assert.True(t, task.PrivateData.SkipTokenQuota)
	assert.Equal(t, TaskBillingStatePending, task.PrivateData.AsyncBilling.State)
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Empty(t, attempt.FrozenConnectionSnapshot)

	require.NoError(t, DB.First(&user, user.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, 90, user.Quota)
	assert.Equal(t, 50, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)

	applied, delta, err := ApplyTaskBillingTarget(task, 0)
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, -10, delta)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, 100, user.Quota)
	assert.Equal(t, 50, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestTaskCreateAttemptSubscriptionZeroQuotaTransferIsAtomicAndExposed(t *testing.T) {
	truncateTables(t)
	user := User{Id: 1203, Username: "attempt-subscription-zero", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{
		UserId: user.Id, Key: "attempt-subscription-zero-token",
		Status: common.TokenStatusEnabled, RemainQuota: 100,
	}
	require.NoError(t, DB.Create(&token).Error)
	plan := SubscriptionPlan{Title: "attempt-plan", TotalAmount: 100}
	require.NoError(t, DB.Create(&plan).Error)
	subscription := UserSubscription{
		UserId: user.Id, PlanId: plan.Id,
		AmountTotal: 100, Status: "active",
		StartTime: time.Now().Add(-time.Hour).Unix(),
		EndTime:   time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, DB.Create(&subscription).Error)
	attempt := createBillingInvariantAttempt(t, user.Id, token.Id, "subscription-zero")

	hold, err := HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "subscription", Quota: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, hold.HeldQuota)
	assert.True(t, hold.TokenDebited)

	task := billingInvariantTask(attempt, user.Id, 0)
	require.NoError(t, RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task))
	require.NoError(t, InsertTaskWithCreateAttempt(task, 0, attempt.ID))

	require.NoError(t, DB.First(&subscription, subscription.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Zero(t, subscription.AmountUsed)
	assert.Equal(t, 100, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Zero(t, task.Quota)
	assert.Equal(t, TaskBillingStatePending, task.PrivateData.AsyncBilling.State)

	var record SubscriptionPreConsumeRecord
	require.NoError(t, DB.First(&record, "request_id = ?", attempt.AttemptID).Error)
	assert.Equal(t, "transferred", record.Status)
	assert.Zero(t, record.PreConsumed)

	exposure := &ProviderCostExposure{
		SourceKind: ProviderCostExposureSourceTask,
		SourceID:   task.TaskID,
		Reason:     "provider_contract_failure",
		UserID:     task.UserId,
	}
	applied, delta, err := ApplyTaskBillingTargetWithExposure(task, 0, exposure)
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Zero(t, delta)
	var saved ProviderCostExposure
	require.NoError(t, DB.First(&saved, "source_kind = ? AND source_id = ?",
		ProviderCostExposureSourceTask, task.TaskID).Error)
	assert.Zero(t, saved.CustomerQuotaReleased)
}

func TestTaskCreateAttemptTransferRollbackKeepsOriginalHold(t *testing.T) {
	truncateTables(t)
	user := User{Id: 1204, Username: "attempt-transfer-rollback", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{
		UserId: user.Id, Key: "attempt-transfer-rollback-token",
		Status: common.TokenStatusEnabled, RemainQuota: 100,
	}
	require.NoError(t, DB.Create(&token).Error)
	attempt := createBillingInvariantAttempt(t, user.Id, token.Id, "transfer-rollback")
	_, err := HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "wallet", Quota: 20,
	})
	require.NoError(t, err)
	task := billingInvariantTask(attempt, user.Id, 10)
	require.NoError(t, RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task))

	callbackName := "test:fail_task_create_attempt_transfer"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Task" {
			tx.AddError(errors.New("forced task insert failure"))
		}
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Create().Remove(callbackName)
	})

	require.Error(t, InsertTaskWithCreateAttempt(task, 0, attempt.ID))
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 80, token.RemainQuota)
	assert.Equal(t, 20, token.UsedQuota)
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, TaskCreateAttemptUpstreamSucceeded, attempt.Status)
	assert.Equal(t, TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
	assert.NotEmpty(t, attempt.FrozenConnectionSnapshot)
	var taskCount int64
	require.NoError(t, DB.Model(&Task{}).Where("task_id = ?", task.TaskID).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
}

func TestRejectedTaskCreateAttemptResetsIdempotencyClaimForSameRequestRetry(t *testing.T) {
	truncateTables(t)
	user := User{Id: 1205, Username: "attempt-safe-retry", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{
		UserId: user.Id, Key: "attempt-safe-retry-token",
		Status: common.TokenStatusEnabled, RemainQuota: 100,
	}
	require.NoError(t, DB.Create(&token).Error)
	claim, created, err := ClaimTaskCreateIdempotency(
		user.Id,
		TaskClientProtocolModelArkV3,
		"safe-retry-key",
		"safe-retry-request",
		time.Now().Add(time.Hour).Unix(),
	)
	require.NoError(t, err)
	require.True(t, created)
	attempt := createBillingInvariantAttempt(t, user.Id, token.Id, "safe-retry")
	require.NoError(t, BindTaskCreateIdempotencyAttempt(claim.ID, attempt.AttemptID))
	_, err = HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "wallet", Quota: 20,
	})
	require.NoError(t, err)

	_, err = ReleaseTaskCreateAttemptHold(attempt.ID, TaskCreateAttemptRejected)
	require.NoError(t, err)
	require.NoError(t, DB.First(claim, claim.ID).Error)
	assert.Equal(t, TaskCreateIdempotencyCreating, claim.Status)
	assert.Empty(t, claim.AttemptID)

	nextAttempt := createBillingInvariantAttempt(t, user.Id, token.Id, "safe-retry-next")
	require.NoError(t, BindTaskCreateIdempotencyAttempt(claim.ID, nextAttempt.AttemptID))
	require.NoError(t, DB.First(claim, claim.ID).Error)
	assert.Equal(t, nextAttempt.AttemptID, claim.AttemptID)
}

func createBillingInvariantAttempt(t *testing.T, userID, tokenID int, suffix string) *TaskCreateAttempt {
	t.Helper()
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		PublicTaskID:             GenerateTaskID(),
		UserID:                   userID,
		TokenID:                  tokenID,
		ClientProtocol:           TaskClientProtocolModelArkV3,
		RequestHash:              "billing-invariant-" + suffix,
		FrozenConnectionSnapshot: []byte(`{"key":"must-clear-after-close"}`),
	})
	require.NoError(t, err)
	return attempt
}

func billingInvariantTask(attempt *TaskCreateAttempt, userID, quota int) *Task {
	return &Task{
		TaskID:         attempt.PublicTaskID,
		UserId:         userID,
		ClientProtocol: TaskClientProtocolModelArkV3,
		Quota:          quota,
		Status:         TaskStatusSubmitted,
		PrivateData: TaskPrivateData{
			UpstreamTaskID: "upstream-" + attempt.AttemptID,
			AsyncBilling:   &TaskAsyncBillingContext{State: TaskBillingStatePending},
		},
	}
}
