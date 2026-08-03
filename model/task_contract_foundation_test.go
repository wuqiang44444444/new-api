package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCreateAttemptStateAndHoldTransitions(t *testing.T) {
	truncateTables(t)
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		PublicTaskID:   GenerateTaskID(),
		UserID:         101,
		ClientProtocol: TaskClientProtocolModelArkV3,
		RequestHash:    "request-hash",
		HeldQuota:      120,
		NextAttemptAt:  common.GetTimestamp() + 60,
	})
	require.NoError(t, err)
	assert.Equal(t, TaskCreateAttemptPrepared, attempt.Status)
	assert.Equal(t, TaskCreateAttemptBillingUnheld, attempt.BillingHoldState)

	won, err := TransitionTaskCreateAttempt(
		DB,
		attempt.ID,
		TaskCreateAttemptPrepared,
		TaskCreateAttemptBillingUnheld,
		TaskCreateAttemptSending,
		TaskCreateAttemptBillingHeld,
		nil,
	)
	require.NoError(t, err)
	assert.True(t, won)

	won, err = TransitionTaskCreateAttempt(
		DB,
		attempt.ID,
		TaskCreateAttemptPrepared,
		TaskCreateAttemptBillingUnheld,
		TaskCreateAttemptSending,
		TaskCreateAttemptBillingHeld,
		nil,
	)
	require.NoError(t, err)
	assert.False(t, won)
}

func TestCompletedNoReplayClaimCannotBeReusedBeforeExpiry(t *testing.T) {
	truncateTables(t)
	claim, created, err := ClaimTaskCreateIdempotency(
		102,
		TaskClientProtocolOpenAIImages,
		"key-hash",
		"request-hash",
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, MarkTaskCreateIdempotencyCompletedNoReplay(claim.ID))

	replayed, created, err := ClaimTaskCreateIdempotency(
		102,
		TaskClientProtocolOpenAIImages,
		"key-hash",
		"request-hash",
		common.GetTimestamp()+3600,
	)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, TaskCreateIdempotencyCompletedNoReplay, replayed.Status)
}

func TestTaskCreateAttemptWalletHoldAndReleaseAreAtomic(t *testing.T) {
	truncateTables(t)
	user := User{Id: 104, Username: "attempt-wallet", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "attempt-token", Status: common.TokenStatusEnabled, RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		PublicTaskID:   GenerateTaskID(),
		UserID:         user.Id,
		TokenID:        token.Id,
		ClientProtocol: TaskClientProtocolModelArkV3,
		RequestHash:    "wallet-hold-request",
	})
	require.NoError(t, err)

	hold, err := HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "wallet", Quota: 25,
	})
	require.NoError(t, err)
	assert.Equal(t, 25, hold.HeldQuota)
	assert.True(t, hold.TokenDebited)

	require.NoError(t, DB.First(&user, user.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, 75, user.Quota)
	assert.Equal(t, 75, token.RemainQuota)
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, TaskCreateAttemptSending, attempt.Status)
	assert.Equal(t, TaskCreateAttemptBillingHeld, attempt.BillingHoldState)

	_, err = ReleaseTaskCreateAttemptHold(attempt.ID, TaskCreateAttemptRejected)
	require.NoError(t, err)
	repeated, err := ReleaseTaskCreateAttemptHold(attempt.ID, TaskCreateAttemptRejected)
	require.NoError(t, err)
	assert.Zero(t, repeated.ReleasedQuota)
	require.NoError(t, DB.First(&user, user.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, 100, user.Quota)
	assert.Equal(t, 100, token.RemainQuota)
}

func TestKnownSuccessAfterUnknownRecoversWithoutAnotherHold(t *testing.T) {
	truncateTables(t)
	user := User{Id: 106, Username: "attempt-recovery", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "attempt-recovery-token", Status: common.TokenStatusEnabled, RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	publicTaskID := GenerateTaskID()
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		PublicTaskID:   publicTaskID,
		UserID:         user.Id,
		TokenID:        token.Id,
		ClientProtocol: TaskClientProtocolModelArkV3,
		RequestHash:    "late-known-success",
	})
	require.NoError(t, err)
	_, err = HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "wallet", Quota: 25,
	})
	require.NoError(t, err)
	require.NoError(t, MarkTaskCreateAttemptUnknown(attempt.ID, "request-106"))
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, "request-106", attempt.UpstreamRequestID)

	task := &Task{
		TaskID:         publicTaskID,
		UserId:         user.Id,
		ChannelId:      18,
		ClientProtocol: TaskClientProtocolModelArkV3,
		Quota:          25,
		Status:         TaskStatusSubmitted,
		PrivateData: TaskPrivateData{
			UpstreamTaskID: "provider-task-106",
		},
	}
	require.NoError(t, RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task))
	recovered, err := RecoverTaskCreateAttempt(attempt.ID)
	require.NoError(t, err)
	assert.Equal(t, publicTaskID, recovered.TaskID)
	assert.Equal(t, "provider-task-106", recovered.PrivateData.UpstreamTaskID)

	require.NoError(t, DB.First(&user, user.Id).Error)
	require.NoError(t, DB.First(&token, token.Id).Error)
	assert.Equal(t, 75, user.Quota)
	assert.Equal(t, 75, token.RemainQuota)
	require.NoError(t, DB.First(attempt, attempt.ID).Error)
	assert.Equal(t, TaskCreateAttemptComplete, attempt.Status)
	assert.Equal(t, TaskCreateAttemptBillingTransferred, attempt.BillingHoldState)
}

func TestAttemptRecoveryCompletesUnknownClientClaim(t *testing.T) {
	truncateTables(t)
	user := User{Id: 108, Username: "attempt-claim-recovery", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	publicTaskID := GenerateTaskID()
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		PublicTaskID:   publicTaskID,
		UserID:         user.Id,
		ClientProtocol: TaskClientProtocolModelArkV3,
		RequestHash:    "request-hash-claim-recovery",
	})
	require.NoError(t, err)
	transitioned, err := TransitionTaskCreateAttempt(
		nil, attempt.ID,
		TaskCreateAttemptPrepared, TaskCreateAttemptBillingUnheld,
		TaskCreateAttemptSending, TaskCreateAttemptBillingHeld,
		nil,
	)
	require.NoError(t, err)
	require.True(t, transitioned)
	claim := TaskCreateIdempotency{
		UserID: user.Id, Protocol: TaskClientProtocolModelArkV3,
		KeyHash: "key-hash-claim-recovery", RequestHash: "request-hash-claim-recovery",
		AttemptID: attempt.AttemptID, Status: TaskCreateIdempotencyUnknown,
		ExpiresAt: common.GetTimestamp() + 3600,
	}
	require.NoError(t, DB.Create(&claim).Error)
	task := &Task{
		TaskID: publicTaskID, UserId: user.Id,
		ClientProtocol: TaskClientProtocolModelArkV3,
		Status:         TaskStatusQueued,
		PrivateData: TaskPrivateData{
			UpstreamTaskID: "upstream-claim-recovery",
			AsyncBilling:   &TaskAsyncBillingContext{State: TaskBillingStatePending},
		},
	}
	require.NoError(t, RecordTaskCreateAttemptUpstreamSuccess(attempt.ID, task))

	recovered, err := RecoverTaskCreateAttempt(attempt.ID)
	require.NoError(t, err)
	assert.Equal(t, task.TaskID, recovered.TaskID)
	require.NoError(t, DB.First(&claim, claim.ID).Error)
	assert.Equal(t, TaskCreateIdempotencyComplete, claim.Status)
	assert.Equal(t, task.TaskID, claim.TaskID)
}

func TestUnknownTaskCreateAttemptReleaseWritesExposureInSameTransaction(t *testing.T) {
	truncateTables(t)
	user := User{Id: 105, Username: "attempt-exposure", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "attempt-exposure-token", Status: common.TokenStatusEnabled, RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	billing, err := common.Marshal(map[string]any{"public_model": "public-video-sku"})
	require.NoError(t, err)
	attempt, err := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
		PublicTaskID:    GenerateTaskID(),
		UserID:          user.Id,
		TokenID:         token.Id,
		ClientProtocol:  TaskClientProtocolModelArkV3,
		RequestHash:     "unknown-request",
		BillingSnapshot: billing,
	})
	require.NoError(t, err)
	_, err = HoldTaskCreateAttempt(TaskAttemptHoldParams{
		AttemptID: attempt.ID, FundingSource: "wallet", Quota: 30,
	})
	require.NoError(t, err)
	require.NoError(t, MarkTaskCreateAttemptUnknown(attempt.ID, "request-exposure"))

	_, err = ReleaseTaskCreateAttemptHold(attempt.ID, TaskCreateAttemptReleasedWithExposure)
	require.NoError(t, err)
	var exposure ProviderCostExposure
	require.NoError(t, DB.First(&exposure, "source_kind = ? AND source_id = ?",
		ProviderCostExposureSourceAttempt, attempt.AttemptID).Error)
	assert.Equal(t, 30, exposure.CustomerQuotaReleased)
	assert.Equal(t, "public-video-sku", exposure.PublicModel)
	assert.Equal(t, string(TaskCreateAttemptReleasedWithExposure), exposure.Reason)
}

func TestProviderContractFailureBillingAndExposureAreAtomic(t *testing.T) {
	truncateTables(t)
	user := User{Id: 103, Username: "contract-user", Quota: 0}
	require.NoError(t, DB.Create(&user).Error)
	target := 0
	task := &Task{
		TaskID:       GenerateTaskID(),
		UserId:       user.Id,
		ChannelId:    9,
		Quota:        120,
		Status:       TaskStatusProviderContractFailure,
		BillingState: TaskBillingStatePending,
		Properties:   Properties{OriginModelName: "public-video-sku"},
		PrivateData: TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{
			State:       TaskBillingStatePending,
			Operation:   "refund",
			TargetQuota: &target,
			Reason:      "provider_contract_failure",
		}},
	}
	require.NoError(t, DB.Create(task).Error)

	_, _, err := ApplyTaskBillingTargetWithExposure(task, 0, &ProviderCostExposure{})
	require.Error(t, err)
	var unchanged Task
	require.NoError(t, DB.First(&unchanged, task.ID).Error)
	assert.Equal(t, 120, unchanged.Quota)

	exposure := &ProviderCostExposure{
		SourceKind:  ProviderCostExposureSourceTask,
		SourceID:    task.TaskID,
		Reason:      "provider_contract_failure",
		UserID:      task.UserId,
		ChannelID:   task.ChannelId,
		PublicModel: task.Properties.OriginModelName,
	}
	applied, delta, err := ApplyTaskBillingTargetWithExposure(task, 0, exposure)
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, -120, delta)

	var savedExposure ProviderCostExposure
	require.NoError(t, DB.First(&savedExposure, "source_kind = ? AND source_id = ? AND reason = ?",
		ProviderCostExposureSourceTask,
		task.TaskID,
		"provider_contract_failure",
	).Error)
	assert.Equal(t, 120, savedExposure.CustomerQuotaReleased)

	applied, _, err = ApplyTaskBillingTargetWithExposure(task, 0, exposure)
	require.NoError(t, err)
	assert.False(t, applied)
	var count int64
	require.NoError(t, DB.Model(&ProviderCostExposure{}).Where("source_id = ?", task.TaskID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestProviderContractFailureIsTerminalAndRefundable(t *testing.T) {
	assert.True(t, TaskStatusProviderContractFailure.IsTerminal())
	assert.True(t, TaskStatusProviderContractFailure.ShouldRefundOnTerminal())
	assert.Contains(t, TerminalTaskStatuses(), TaskStatusProviderContractFailure)
}
