package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectUnknownTaskCreateAttemptRequiresVerificationAndReleasesOnce(t *testing.T) {
	truncate(t)
	user := &model.User{Id: 961, Username: "attempt-rejection", Quota: 100}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{
		UserId:      user.Id,
		Key:         "attempt-rejection-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 100,
	}
	require.NoError(t, model.DB.Create(token).Error)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-manual-rejection",
		UserID:         user.Id,
		TokenID:        token.Id,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "manual-rejection-request",
	})
	require.NoError(t, err)
	claim, created, err := model.ClaimTaskCreateIdempotency(
		user.Id,
		model.TaskClientProtocolModelArkV3,
		"manual-rejection-key-hash",
		"manual-rejection-request",
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, model.BindTaskCreateIdempotencyAttempt(claim.ID, attempt.AttemptID))
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID:     attempt.ID,
		FundingSource: BillingSourceWallet,
		Quota:         25,
	})
	require.NoError(t, err)
	require.NoError(t, model.MarkTaskCreateAttemptUnknown(attempt.ID, "request-rejection"))
	require.NoError(t, model.MarkTaskCreateIdempotencyUnknown(claim.ID))

	_, err = RejectUnknownTaskCreateAttempt(
		attempt.AttemptID,
		false,
		user.Id,
		"provider console verified no task",
	)
	require.Error(t, err)

	_, err = RejectUnknownTaskCreateAttempt(
		attempt.AttemptID,
		true,
		user.Id,
		"verified at https://provider.example/task",
	)
	require.Error(t, err)

	rejected, err := RejectUnknownTaskCreateAttempt(
		attempt.AttemptID,
		true,
		user.Id,
		"provider console verified no task",
	)
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskCreateAttemptRejected), rejected.Status)
	assert.Equal(t, string(model.TaskCreateAttemptBillingReleased), rejected.BillingHoldState)
	assert.Equal(t, 25, rejected.ReleasedQuota)

	replayed, err := RejectUnknownTaskCreateAttempt(
		attempt.AttemptID,
		true,
		user.Id,
		"idempotent provider rejection replay",
	)
	require.NoError(t, err)
	assert.Zero(t, replayed.ReleasedQuota)

	require.NoError(t, model.DB.First(user, user.Id).Error)
	require.NoError(t, model.DB.First(token, token.Id).Error)
	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, 100, user.Quota)
	assert.Equal(t, 100, token.RemainQuota)
	assert.Equal(t, model.TaskCreateAttemptRejected, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
	assert.Equal(t, user.Id, attempt.ManualRecoveryBy)
	assert.NotZero(t, attempt.ManualRecoveryAt)
	assert.Equal(t, "provider console verified no task", attempt.ManualRecoveryNote)
	assert.Equal(t, "request-rejection", attempt.UpstreamRequestID)
	var claimCount int64
	require.NoError(t, model.DB.Model(&model.TaskCreateIdempotency{}).
		Where("id = ?", claim.ID).
		Count(&claimCount).Error)
	assert.Zero(t, claimCount)

	_, created, err = model.ClaimTaskCreateIdempotency(
		user.Id,
		model.TaskClientProtocolModelArkV3,
		"manual-rejection-key-hash",
		"manual-rejection-request",
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	assert.True(t, created)
}

func TestRejectUnknownTaskCreateAttemptDoesNotReleaseSendingAttempt(t *testing.T) {
	truncate(t)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   "task-still-sending",
		UserID:         962,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		RequestHash:    "still-sending-request",
	})
	require.NoError(t, err)
	transitioned, err := model.TransitionTaskCreateAttempt(
		nil,
		attempt.ID,
		model.TaskCreateAttemptPrepared,
		model.TaskCreateAttemptBillingUnheld,
		model.TaskCreateAttemptSending,
		model.TaskCreateAttemptBillingHeld,
		nil,
	)
	require.NoError(t, err)
	require.True(t, transitioned)

	_, err = RejectUnknownTaskCreateAttempt(
		attempt.AttemptID,
		true,
		962,
		"provider console verified no task",
	)
	require.Error(t, err)
	require.NoError(t, model.DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
}
