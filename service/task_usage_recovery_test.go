package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageRecoverySettlesWithoutClientQuery(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8140, 1000)
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.PrivateData.Key = "fixture-key"
	task.PrivateData.VideoUpstreamQueryBaseURL = "https://frozen.example"
	require.NoError(t, model.DB.Save(task).Error)
	usage := 100
	old := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return &funCloudUsagePollingAdaptor{usage: &usage} }
	t.Cleanup(func() { GetTaskAdaptorFunc = old })
	assert.True(t, model.HasTaskPollingWork())
	ReconcileTaskUsage(context.Background())
	saved := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
	assert.Equal(t, 100, saved.Quota)
	assert.Equal(t, 1600, getUserQuota(t, task.UserId))
	ReconcileTaskUsage(context.Background())
	assert.Equal(t, 1600, getUserQuota(t, task.UserId))
}

func TestUsageRecoveryClaimsSurviveRestartAndExpireToReview(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8141, 1000)
	now := model.GetDBTimestamp()
	first, review, err := model.ClaimTaskUsageCheck(task.ID, now)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.False(t, review)
	again, _, err := model.ClaimTaskUsageCheck(task.ID, now)
	require.NoError(t, err)
	assert.Nil(t, again)
	// Model the next process after the first worker dies without completing.
	next, review, err := model.ClaimTaskUsageCheck(task.ID, first.UsageCheckNextAt)
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.False(t, review)
	assert.Equal(t, 2, next.UsageCheckAttempts)
	require.NoError(t, model.FinishTaskUsageCheck(first, true))
	assert.Equal(t, "observation_incomplete", reloadTask(t, task.ID).UsageCheckLastError)
	expired, review, err := model.ClaimTaskUsageCheck(task.ID, now+model.TaskUsageCheckWindowSeconds)
	require.NoError(t, err)
	require.NotNil(t, expired)
	assert.True(t, review)
	assert.Equal(t, 700, expired.Quota)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, expired.BillingState)
	assert.False(t, model.HasDueTaskUsageChecks(now+model.TaskUsageCheckWindowSeconds))
	items, err := model.ListTaskUsageRecovery(true, 0, 50)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, task.TaskID, items[0].TaskID)
	assert.Equal(t, 1000, getUserQuota(t, task.UserId))
}

func TestUsageRecoveryReviewAndVerifiedStatementUseFrozenBilling(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8142, 1000)
	now := model.GetDBTimestamp()
	require.NoError(t, model.DB.Model(task).Updates(map[string]any{"usage_check_started_at": now - model.TaskUsageCheckWindowSeconds, "usage_check_attempts": model.TaskUsageCheckMaxAttempts}).Error)
	_, review, err := model.ClaimTaskUsageCheck(task.ID, now)
	require.NoError(t, err)
	require.True(t, review)
	retry, err := model.ReviewTaskUsage(task.TaskID, 9, "incident-123", nil)
	require.NoError(t, err)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, retry.BillingState)
	first, review, err := model.ClaimTaskUsageCheck(task.ID, retry.UsageCheckNextAt)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.False(t, review)
	assert.Equal(t, now-model.TaskUsageCheckWindowSeconds, first.UsageCheckStartedAt)
	_, review, err = model.ClaimTaskUsageCheck(task.ID, first.UsageCheckNextAt)
	require.NoError(t, err)
	assert.True(t, review)
	zero := 0
	resolved, err := model.ReviewTaskUsage(task.TaskID, 9, "statement-123", &zero)
	require.NoError(t, err)
	assert.Equal(t, model.TaskBillingStatePending, resolved.BillingState)
	require.Equal(t, 1, ReconcileTaskBilling(context.Background(), 10).Scanned)
	assert.Equal(t, 1700, getUserQuota(t, task.UserId))
	_, err = model.ReviewTaskUsage(task.TaskID, 9, "statement-123", &zero)
	require.NoError(t, err)
	changed := 10
	_, err = model.ReviewTaskUsage(task.TaskID, 9, "statement-changed", &changed)
	require.Error(t, err)
	assert.Zero(t, ReconcileTaskBilling(context.Background(), 10).Scanned)
	saved := reloadTask(t, task.ID)
	assert.Equal(t, 9, saved.UsageReviewOperatorID)
	assert.Equal(t, "statement-123", saved.UsageReviewReference)
	assert.Zero(t, saved.ToModelArkVideoTask().Usage.CompletionTokens)
}

func TestUsageRecoveryLateProviderUsageCannotReopenOperatorSettlement(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8143, 1000)
	stale := reloadTask(t, task.ID)
	value := 100
	_, err := model.ReviewTaskUsage(task.TaskID, 9, "statement-456", &value)
	require.NoError(t, err)
	require.Equal(t, 1, ReconcileTaskBilling(context.Background(), 10).Scanned)
	different := 200
	observeFunCloudUsage(t, stale, &different)
	require.True(t, settleTaskTieredSnapshot(context.Background(), stale, different))
	saved := reloadTask(t, task.ID)
	assert.Equal(t, 100, saved.Quota)
	assert.Equal(t, "operator_verified_statement", saved.PrivateData.AsyncBilling.ActualUsageSource)
	assert.Equal(t, 1600, getUserQuota(t, task.UserId))
}

func TestUsageRecoveryRejectsInvalidOperatorEvidence(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8144, 1000)
	negative := -1
	_, err := model.ReviewTaskUsage(task.TaskID, 9, "statement", &negative)
	require.Error(t, err)
	zero := 0
	_, err = model.ReviewTaskUsage(task.TaskID, 0, "statement", &zero)
	require.Error(t, err)
	_, err = model.ReviewTaskUsage(task.TaskID, 9, "", &zero)
	require.Error(t, err)
	assert.Equal(t, 700, reloadTask(t, task.ID).Quota)
}

func TestCancelledVideoObservationDoesNotCountAsProviderFailure(t *testing.T) {
	truncate(t)
	seedUser(t, 8145, 1000)
	task := persistedAsyncTask(t, 8145, 700, model.TaskStatusInProgress)
	previous := constant.TaskPollMaxFailures
	constant.TaskPollMaxFailures = 1
	t.Cleanup(func() { constant.TaskPollMaxFailures = previous })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, recordPollFailure(ctx, nil, task, task.Status, pollClassTransport, 0, "request cancelled"), context.Canceled)
	saved := reloadTask(t, task.ID)
	assert.EqualValues(t, model.TaskStatusInProgress, saved.Status)
	assert.Zero(t, saved.PrivateData.PollFailures)
	assert.Equal(t, 700, saved.Quota)
	assert.Equal(t, 1000, getUserQuota(t, task.UserId))
}
