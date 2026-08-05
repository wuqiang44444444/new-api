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
