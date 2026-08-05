package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePreparedTaskAttemptBindsIdempotencyAtomically(t *testing.T) {
	truncateTables(t)
	claim, created, err := ClaimTaskCreateIdempotency(
		993,
		TaskClientProtocolModelArkV3,
		"atomic-attempt-key",
		"atomic-attempt-request",
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	require.True(t, created)
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		IdempotencyID:  claim.ID,
		PublicTaskID:   "task-atomic-attempt",
		UserID:         993,
		ClientProtocol: TaskClientProtocolModelArkV3,
		RequestHash:    "atomic-attempt-request",
	})
	require.NoError(t, err)
	require.NoError(t, DB.First(claim, claim.ID).Error)
	assert.Equal(t, attempt.AttemptID, claim.AttemptID)

	_, err = CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		IdempotencyID:  claim.ID + 1000,
		PublicTaskID:   "task-atomic-attempt-rollback",
		UserID:         993,
		ClientProtocol: TaskClientProtocolModelArkV3,
		RequestHash:    "atomic-attempt-rollback-request",
	})
	require.Error(t, err)
	var count int64
	require.NoError(t, DB.Model(&TaskCreateAttempt{}).
		Where("public_task_id = ?", "task-atomic-attempt-rollback").Count(&count).Error)
	assert.Zero(t, count)
}
