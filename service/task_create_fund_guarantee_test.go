package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createGuaranteeTestUserAndToken(t *testing.T, userID int) (*model.User, int) {
	t.Helper()
	user := model.User{Id: userID, Username: "fund-guarantee-user", Quota: 100}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId: userID, Key: "fund-guarantee-token",
		Status: common.TokenStatusEnabled, RemainQuota: 100,
	}
	require.NoError(t, model.DB.Create(&token).Error)
	return &user, token.Id
}

func createHeldUnknownOverdueAttempt(
	t *testing.T,
	userID, tokenID int,
	suffix string,
	quota int,
) *model.TaskCreateAttempt {
	t.Helper()
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-fund-guarantee-" + suffix,
		UserID:         userID,
		TokenID:        tokenID,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "fund-guarantee-" + suffix,
	})
	require.NoError(t, err)
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: BillingSourceWallet, Quota: quota,
	})
	require.NoError(t, err)
	require.NoError(t, model.MarkTaskCreateAttemptUnknown(attempt.ID, "provider-request"))
	require.NoError(t, model.DB.Model(&model.TaskCreateAttempt{}).Where("id = ?", attempt.ID).
		Update("funds_deadline_at", time.Now().Unix()-60).Error)
	return attempt
}

func TestRunTaskCreateFundGuaranteeOnceReleasesOverdueHold(t *testing.T) {
	truncate(t)
	user, tokenID := createGuaranteeTestUserAndToken(t, 1601)
	attempt := createHeldUnknownOverdueAttempt(t, user.Id, tokenID, "wallet", 25)

	summary := RunTaskCreateFundGuaranteeOnce(context.Background())
	assert.Equal(t, 1, summary.Debts)
	assert.Equal(t, 1, summary.Released)
	assert.Equal(t, int64(25), summary.ReleasedQuota)

	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
	assert.Equal(t, model.TaskCreateAttemptReleaseWarrantyDeadline, attempt.ReleaseReason)

	// 第二轮无待办，也不重复退款。
	summary = RunTaskCreateFundGuaranteeOnce(context.Background())
	assert.Zero(t, summary.Debts)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestReconcileRoutesOverdueUpstreamSucceededToWarrantyRefund(t *testing.T) {
	truncate(t)
	user, tokenID := createGuaranteeTestUserAndToken(t, 1602)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-fund-guarantee-route",
		UserID:         user.Id,
		TokenID:        tokenID,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "fund-guarantee-route",
	})
	require.NoError(t, err)
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: BillingSourceWallet, Quota: 30,
	})
	require.NoError(t, err)
	task := &model.Task{
		TaskID:         attempt.PublicTaskID,
		UserId:         user.Id,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		Quota:          30,
		Status:         model.TaskStatusSubmitted,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-fund-guarantee-route",
			AsyncBilling:   &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
	}
	require.NoError(t, model.RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task))
	require.NoError(t, model.DB.Model(&model.TaskCreateAttempt{}).Where("id = ?", attempt.ID).
		Updates(map[string]any{
			"funds_deadline_at": common.GetTimestamp() - 60,
			"next_attempt_at":   common.GetTimestamp() - 1,
		}).Error)

	assert.Equal(t, 1, ReconcileTaskCreateAttempts(context.Background()))

	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, model.TaskCreateAttemptUpstreamSucceeded, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
	assert.Equal(t, model.TaskCreateAttemptReleaseWarrantyDeadline, attempt.ReleaseReason)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Where("task_id = ?", attempt.PublicTaskID).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
}

func TestRecoverAfterFundsDeadlineMovesToWarrantyRefundAndRefusesLateRecover(t *testing.T) {
	truncate(t)
	user, tokenID := createGuaranteeTestUserAndToken(t, 1603)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-fund-guarantee-late",
		UserID:         user.Id,
		TokenID:        tokenID,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "fund-guarantee-late",
	})
	require.NoError(t, err)
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: BillingSourceWallet, Quota: 40,
	})
	require.NoError(t, err)
	task := &model.Task{
		TaskID:         attempt.PublicTaskID,
		UserId:         user.Id,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		Quota:          40,
		Status:         model.TaskStatusSubmitted,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-fund-guarantee-late",
			AsyncBilling:   &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
	}
	require.NoError(t, model.RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task))
	require.NoError(t, model.DB.Model(&model.TaskCreateAttempt{}).Where("id = ?", attempt.ID).
		Update("funds_deadline_at", common.GetTimestamp()-60).Error)

	_, err = model.RecoverTaskCreateAttempt(attempt.ID)
	require.ErrorIs(t, err, model.ErrTaskCreateAttemptMovedToWarrantyRefund)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)

	// 退款后的迟到恢复被拒绝，不再建立收费 Task。
	_, err = model.RecoverTaskCreateAttempt(attempt.ID)
	require.Error(t, err)
	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Where("task_id = ?", attempt.PublicTaskID).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
}

func TestTaskCreateTransportProbeStateProvenUnsent(t *testing.T) {
	testCases := []struct {
		name       string
		dial       uint32
		failures   uint32
		establish  uint32
		wrote      bool
		expectSent bool
	}{
		{name: "no observation stays unknown"},
		{name: "dial refused without any write is unsent", dial: 1, failures: 1, expectSent: true},
		{name: "established connection stays unknown", dial: 1, establish: 1},
		{name: "any write stays unknown", dial: 2, failures: 1, establish: 1, wrote: true},
		{name: "dial failure after write stays unknown", dial: 2, failures: 2, wrote: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state := &taskCreateTransportProbeState{}
			state.dialAttempted.Store(tc.dial)
			state.dialFailures.Store(tc.failures)
			state.connsEstablished.Store(tc.establish)
			state.wroteAny.Store(tc.wrote)
			assert.Equal(t, tc.expectSent, state.provenUnsent())
		})
	}
}
