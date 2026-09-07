package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func persistedFunCloudUsageTask(t *testing.T, userID, wallet int) *model.Task {
	t.Helper()
	truncate(t)
	seedUser(t, userID, wallet)
	task := persistedAsyncTask(t, userID, 700, model.TaskStatusSuccess)
	task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3
	task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolFunCloudModelArkV3
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, task.PrivateData.VideoUpstreamProfile)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("tokens", c * 2)`, 700)
	task.Data = []byte(`{"status":"succeeded"}`)
	require.NoError(t, model.DB.Save(task).Error)
	require.True(t, settleTaskTieredSnapshot(context.Background(), task, 0))
	return reloadTask(t, task.ID)
}

func observeFunCloudUsage(t *testing.T, task *model.Task, usage *int) {
	t.Helper()
	result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
	payload := map[string]any{"status": "succeeded"}
	if usage != nil {
		result.CompletionTokens, result.TotalTokens = *usage, *usage+10
		result.CompletionTokensReported, result.UsageReported = true, true
		result.UsageSource = "usage.completion_tokens"
		result.UsageEvidence = map[string]int{"usage.completion_tokens": *usage, "usage.total_tokens": *usage + 10}
		payload["usage"] = map[string]int{"completion_tokens": *usage, "total_tokens": *usage + 10}
	}
	prepareTerminalTaskBilling(task, result)
	var err error
	task.Data, err = common.Marshal(payload)
	require.NoError(t, err)
	won, err := task.UpdateWithStatus(model.TaskStatusSuccess)
	require.NoError(t, err)
	require.True(t, won)
}

func TestFunCloudStaleWritersCannotReopenSettlement(t *testing.T) {
	ctx := context.Background()
	current := persistedFunCloudUsageTask(t, 8130, 1000)
	staleBilling := reloadTask(t, current.ID)
	stalePoll := reloadTask(t, current.ID)
	usage := 100
	observeFunCloudUsage(t, current, &usage)
	require.Equal(t, model.TaskBillingStatePending, current.PrivateData.AsyncBilling.State)
	// A crash here must leave accepted usage eligible for the independent worker.
	require.Equal(t, 1, ReconcileTaskBilling(ctx, 10).Scanned)
	require.Equal(t, 1600, getUserQuota(t, current.UserId))
	// B had read quota=700 before A refunded 600. Neither write may restore it.
	require.NoError(t, staleBilling.UpdateBilling())
	observeFunCloudUsage(t, stalePoll, nil)
	require.True(t, settleTaskTieredSnapshot(ctx, stalePoll, 0))
	actual := reloadTask(t, current.ID)
	assert.Equal(t, 100, actual.Quota)
	assert.Equal(t, model.TaskBillingStateSettled, actual.PrivateData.AsyncBilling.State)
	assert.Equal(t, 100, actual.PrivateData.AsyncBilling.ActualTokens)
	require.NotNil(t, actual.PrivateData.AsyncBilling.UsageDiscrepancy)
	assert.False(t, actual.PrivateData.AsyncBilling.UsageDiscrepancy.Reported)
	require.NotNil(t, actual.ToModelArkVideoTask().Usage)
	assert.Equal(t, 100, actual.ToModelArkVideoTask().Usage.CompletionTokens)
	assert.Equal(t, 110, actual.ToModelArkVideoTask().Usage.TotalTokens)
	assert.Equal(t, 1600, getUserQuota(t, current.UserId))
	assert.Zero(t, ReconcileTaskBilling(ctx, 10).Scanned)
}

func TestFunCloudDebtKeepsAcceptedUsageAndFundingTarget(t *testing.T) {
	for _, conflicting := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "different"}[conflicting], func(t *testing.T) {
			task := persistedFunCloudUsageTask(t, 8131, 0)
			usage := 1000
			observeFunCloudUsage(t, task, &usage)
			require.True(t, settleTaskTieredSnapshot(context.Background(), task, usage))
			debt := reloadTask(t, task.ID)
			require.Equal(t, model.TaskBillingStateDebt, debt.PrivateData.AsyncBilling.State)
			require.NotNil(t, debt.PrivateData.AsyncBilling.TargetQuota)
			require.Equal(t, 1000, *debt.PrivateData.AsyncBilling.TargetQuota)
			var observed *int
			if conflicting {
				v := 2000
				observed = &v
			}
			observeFunCloudUsage(t, debt, observed)
			require.True(t, settleTaskTieredSnapshot(context.Background(), debt, 0))
			stored := reloadTask(t, task.ID)
			assert.Equal(t, model.TaskBillingStateDebt, stored.PrivateData.AsyncBilling.State)
			assert.Equal(t, 1000, stored.PrivateData.AsyncBilling.ActualTokens)
			assert.Equal(t, 1000, *stored.PrivateData.AsyncBilling.TargetQuota)
			require.NotNil(t, stored.PrivateData.AsyncBilling.UsageDiscrepancy)
			require.NotNil(t, stored.ToModelArkVideoTask().Usage)
			assert.Equal(t, 1000, stored.ToModelArkVideoTask().Usage.CompletionTokens)
			assert.Equal(t, 1010, stored.ToModelArkVideoTask().Usage.TotalTokens)
			applied, _, err := model.ApplyTaskBillingTarget(stored, 2000)
			require.Error(t, err)
			assert.False(t, applied)
			require.Error(t, recordPollFailure(context.Background(), nil, stored, model.TaskStatusSuccess, pollClassTransport, 0, ""))
			assert.Equal(t, model.TaskBillingStateDebt, reloadTask(t, task.ID).PrivateData.AsyncBilling.State)
			// Top-up and scheduled retry must use the original target exactly once.
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", task.UserId).Update("quota", 500).Error)
			stored = reloadTask(t, task.ID)
			stored.PrivateData.AsyncBilling.NextRetryAt = 0
			require.NoError(t, stored.UpdateBilling())
			require.Equal(t, 1, ReconcileTaskBilling(context.Background(), 10).Scanned)
			assert.Equal(t, 200, getUserQuota(t, task.UserId))
			assert.Equal(t, 1000, reloadTask(t, task.ID).Quota)
			assert.Zero(t, ReconcileTaskBilling(context.Background(), 10).Scanned)
			assert.Equal(t, 200, getUserQuota(t, task.UserId))
		})
	}
}

func TestFunCloudFirstAcceptedZeroSurvivesConflictingPoll(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8132, 1000)
	stale := reloadTask(t, task.ID)
	zero := 0
	observeFunCloudUsage(t, task, &zero)
	// A second valid response arrives before the first worker computes its target.
	different := 200
	observeFunCloudUsage(t, stale, &different)
	require.True(t, settleTaskTieredSnapshot(context.Background(), stale, different))
	stored := reloadTask(t, task.ID)
	assert.Zero(t, stored.Quota)
	assert.Equal(t, 1700, getUserQuota(t, task.UserId))
	require.NotNil(t, stored.ToModelArkVideoTask().Usage)
	assert.Zero(t, stored.ToModelArkVideoTask().Usage.CompletionTokens)
	assert.Equal(t, 10, stored.ToModelArkVideoTask().Usage.TotalTokens)
	require.NotNil(t, stored.PrivateData.AsyncBilling.UsageDiscrepancy)
	assert.Equal(t, 200, stored.PrivateData.AsyncBilling.UsageDiscrepancy.Tokens)
	publicJSON, err := common.Marshal(stored.ToModelArkVideoTask())
	require.NoError(t, err)
	assert.NotContains(t, string(publicJSON), "usage_discrepancy")
}

func TestFunCloudSuccessfulTaskCannotRegressWhileBillingPending(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8133, 1000)
	for _, next := range []model.TaskStatus{model.TaskStatusInProgress, model.TaskStatusFailure, model.TaskStatusReconciliationRequired} {
		stale := reloadTask(t, task.ID)
		stale.Status = next
		won, err := stale.UpdateWithStatus(model.TaskStatusSuccess)
		require.NoError(t, err)
		assert.False(t, won)
		assert.EqualValues(t, model.TaskStatusSuccess, stale.Status)
		assert.Equal(t, 1000, getUserQuota(t, task.UserId))
	}
}

func TestFunCloudFailedObservationWriteCannotTriggerBilling(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8134, 1000)
	callback := "test:funcloud_reject_observation"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			tx.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Update().Remove(callback) })
	usage := 100
	adaptor := &funCloudUsagePollingAdaptor{usage: &usage}
	require.ErrorIs(t, updateVideoSingleTask(context.Background(), adaptor, &model.Channel{Type: constant.ChannelTypeSeedanceLink}, task.TaskID, map[string]*model.Task{task.TaskID: task}), assert.AnError)
	stored := reloadTask(t, task.ID)
	assert.Equal(t, 700, stored.Quota)
	assert.False(t, stored.PrivateData.AsyncBilling.ActualUsageReported)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, stored.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1000, getUserQuota(t, task.UserId))
}

func TestFunCloudChangedTotalIsAuditedWithoutChangingPublicUsage(t *testing.T) {
	task := persistedFunCloudUsageTask(t, 8135, 1000)
	usage := 100
	observeFunCloudUsage(t, task, &usage)
	require.True(t, settleTaskTieredSnapshot(context.Background(), task, usage))
	current := reloadTask(t, task.ID)
	result := &relaycommon.TaskInfo{
		CompletionTokens: 100, TotalTokens: 999, CompletionTokensReported: true, UsageReported: true,
		UsageSource:   "usage.completion_tokens",
		UsageEvidence: map[string]int{"usage.completion_tokens": 100, "usage.total_tokens": 999},
	}
	prepareTerminalTaskBilling(current, result)
	current.Data = []byte(`{"status":"succeeded","usage":{"completion_tokens":100,"total_tokens":999}}`)
	won, err := current.UpdateWithStatus(model.TaskStatusSuccess)
	require.NoError(t, err)
	require.True(t, won)
	saved := reloadTask(t, task.ID)
	require.NotNil(t, saved.PrivateData.AsyncBilling.UsageDiscrepancy)
	assert.Equal(t, 999, saved.PrivateData.AsyncBilling.UsageDiscrepancy.Evidence["usage.total_tokens"])
	assert.Equal(t, 110, saved.ToModelArkVideoTask().Usage.TotalTokens)
	assert.Equal(t, 1600, getUserQuota(t, task.UserId))
}

func TestFunCloudStaleBillingPreservesFundingAndFrozenConnection(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		t.Run(map[bool]string{false: "wallet", true: "subscription"}[subscription], func(t *testing.T) {
			task := persistedFunCloudUsageTask(t, 8136, 1000)
			seedToken(t, 8136, task.UserId, "test-funcloud-accounting", 1000)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 8136).Update("used_quota", 700).Error)
			task.PrivateData.TokenId = 8136
			task.PrivateData.VideoUpstreamQueryBaseURL = "https://frozen.example"
			if subscription {
				seedSubscription(t, 8136, task.UserId, 2000, 700)
				task.PrivateData.BillingSource = "subscription"
				task.PrivateData.SubscriptionId = 8136
			}
			require.NoError(t, model.DB.Save(task).Error)
			stale := reloadTask(t, task.ID)
			usage := 100
			observeFunCloudUsage(t, task, &usage)
			require.True(t, settleTaskTieredSnapshot(context.Background(), task, usage))
			stale.PrivateData.VideoUpstreamQueryBaseURL = "https://changed.example"
			stale.PrivateData.AsyncBilling.TieredSnapshot.ExprString = `tier("changed", 1)`
			require.NoError(t, stale.UpdateBilling())
			require.True(t, settleTaskTieredSnapshot(context.Background(), stale, 0))
			assert.Equal(t, "https://frozen.example", stale.PrivateData.VideoUpstreamQueryBaseURL)
			assert.Equal(t, `tier("tokens", c * 2)`, stale.PrivateData.AsyncBilling.TieredSnapshot.ExprString)
			var token model.Token
			require.NoError(t, model.DB.First(&token, 8136).Error)
			assert.Equal(t, 1600, token.RemainQuota)
			assert.Equal(t, 100, token.UsedQuota)
			if subscription {
				var sub model.UserSubscription
				require.NoError(t, model.DB.First(&sub, 8136).Error)
				assert.EqualValues(t, 100, sub.AmountUsed)
				assert.Equal(t, 1000, getUserQuota(t, task.UserId))
			} else {
				assert.Equal(t, 1600, getUserQuota(t, task.UserId))
			}
		})
	}
}
