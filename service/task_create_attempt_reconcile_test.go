package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcileTaskCreateAttemptsRejectsStalePreparedAttempt(t *testing.T) {
	truncate(t)
	claim, created, err := model.ClaimTaskCreateIdempotency(
		991,
		model.TaskClientProtocolModelArkV3,
		"stale-prepared-key",
		"stale-prepared-request",
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	require.True(t, created)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		IdempotencyID:            claim.ID,
		PublicTaskID:             "task-stale-prepared",
		UserID:                   991,
		ClientProtocol:           model.TaskClientProtocolModelArkV3,
		RequestHash:              "stale-prepared-request",
		FrozenConnectionSnapshot: []byte(`{"key":"provider-test-secret"}`),
		NextAttemptAt:            common.GetTimestamp() - 1,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, ReconcileTaskCreateAttempts(context.Background()))
	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, model.TaskCreateAttemptRejected, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
	assert.Empty(t, attempt.FrozenConnectionSnapshot)
	assert.Zero(t, attempt.NextAttemptAt)

	var claimCount int64
	require.NoError(t, model.DB.Model(&model.TaskCreateIdempotency{}).Where("id = ?", claim.ID).Count(&claimCount).Error)
	assert.Zero(t, claimCount)
	_, created, err = model.ClaimTaskCreateIdempotency(
		991,
		model.TaskClientProtocolModelArkV3,
		"stale-prepared-key",
		"stale-prepared-request",
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	assert.True(t, created)
}

func TestReconcileTaskCreateAttemptsStopsUnknownWithoutReleasingHold(t *testing.T) {
	truncate(t)
	user := &model.User{Id: 992, Username: "unknown-hold", Quota: 100}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: "unknown-hold-token", Status: common.TokenStatusEnabled, RemainQuota: 100}
	require.NoError(t, model.DB.Create(token).Error)
	now := common.GetTimestamp()
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-unknown-hold",
		UserID:         user.Id,
		TokenID:        token.Id,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "unknown-hold-request",
		NextAttemptAt:  now - 1,
	})
	require.NoError(t, err)
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: BillingSourceWallet, Quota: 25,
	})
	require.NoError(t, err)
	require.NoError(t, model.MarkTaskCreateAttemptUnknown(attempt.ID, "provider-request"))
	require.NoError(t, model.ScheduleTaskCreateAttemptReconcile(attempt.ID, model.TaskCreateAttemptUnknown, now-1))

	assert.Equal(t, 1, ReconcileTaskCreateAttempts(context.Background()))
	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	require.NoError(t, model.DB.First(token, token.Id).Error)
	assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
	assert.Zero(t, attempt.NextAttemptAt)
	assert.Equal(t, 75, user.Quota)
	assert.Equal(t, 75, token.RemainQuota)

	var exposures int64
	require.NoError(t, model.DB.Model(&model.ProviderCostExposure{}).
		Where("source_id = ?", attempt.AttemptID).Count(&exposures).Error)
	assert.Zero(t, exposures)
}
