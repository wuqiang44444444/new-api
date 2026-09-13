package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeSeedanceUsageTask builds a successful Seedance Link task with a frozen
// USD usage snapshot and probe, the shape every migrated new acceptance
// creates. budgetTokens is the administrator pre-consume budget frozen at
// submission.
func makeSeedanceUsageTask(
	t *testing.T,
	expr string,
	budgetTokens int,
	preConsumedQuota int,
) *model.Task {
	t.Helper()
	snap := &billingexpr.BillingSnapshot{
		ExprString:                expr,
		ExprHash:                  billingexpr.ExprHashString(expr),
		GroupRatio:                1,
		QuotaPerUnit:              1000,
		ExprVersion:               1,
		TaskUsageBilling:          true,
		EstimatedCompletionTokens: budgetTokens,
	}
	probeBody, err := common.Marshal(map[string]any{"_task": map[string]any{
		"resolution": "1080p", "has_video_input": false, "duration_seconds": 5,
	}})
	require.NoError(t, err)
	task := makeTask(8991, 0, preConsumedQuota, 0, BillingSourceWallet, 0)
	task.TaskID = model.GenerateTaskID()
	task.Status = model.TaskStatusSuccess
	task.FinishTime = time.Now().Unix()
	task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolModelArkV3Volcengine
	model.AttachAsyncTaskBilling(&task.PrivateData, &relaycommon.RelayInfo{
		TieredBillingSnapshot: snap,
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink},
		BillingRequestInput:   &billingexpr.RequestInput{Body: probeBody},
	}, preConsumedQuota)
	require.NotNil(t, task.PrivateData.AsyncBilling, "Seedance usage tasks must attach the funding lifecycle")
	require.NoError(t, model.DB.Create(task).Error)
	return reloadTask(t, task.ID)
}

// reportSeedanceUsage records one trusted usage observation the way polling
// does, then reloads the task so stored facts are authoritative.
func reportSeedanceUsage(t *testing.T, task *model.Task, tokens int, source string) {
	t.Helper()
	task.PrivateData.AsyncBilling.ActualTokens = tokens
	task.PrivateData.AsyncBilling.ActualUsageReported = true
	task.PrivateData.AsyncBilling.ActualUsageSource = source
	_, err := task.UpdateWithStatus(model.TaskStatusSuccess)
	require.NoError(t, err)
}

func TestSeedanceUsageSettlementLifecycle(t *testing.T) {
	const expr = `tier("base", u("tokens") * 5 / 1000000)`
	const budget = 300000

	t.Run("success without reported usage enters awaiting and never settles on the budget", func(t *testing.T) {
		truncate(t)
		seedUser(t, 8991, 10000)
		task := makeSeedanceUsageTask(t, expr, budget, 1500)
		result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
		prepareTerminalTaskBilling(task, result)
		require.True(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, result))
		after := reloadTask(t, task.ID)
		assert.Equal(t, model.TaskBillingStateAwaitingUsage, after.PrivateData.AsyncBilling.State)
		assert.Equal(t, 1500, after.Quota, "budget hold is retained while awaiting usage")
		assert.Equal(t, 10000, getUserQuota(t, 8991))
		assert.False(t, after.PrivateData.AsyncBilling.ActualUsageReported)
		assert.Zero(t, after.PrivateData.AsyncBilling.ActualTokens, "the submission budget must not become the accepted meter")
	})

	t.Run("operator verified usage settles at the expression amount exactly once", func(t *testing.T) {
		truncate(t)
		seedUser(t, 8991, 10000)
		task := makeSeedanceUsageTask(t, expr, budget, 1500)
		reportSeedanceUsage(t, task, 120000, "operator_verified_statement")
		stored := reloadTask(t, task.ID)
		require.True(t, stored.PrivateData.AsyncBilling.ActualUsageReported)
		result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
		prepareTerminalTaskBilling(stored, result)
		require.True(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, stored, result))
		settled := reloadTask(t, task.ID)
		// 120000 tokens × $5/1M × 1000 quota-per-dollar = 600 quota; pre-consumed 1500 → refund 900.
		assert.Equal(t, 600, settled.Quota)
		assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
		assert.Equal(t, 10900, getUserQuota(t, 8991))
		// A repeated reconcile must be idempotent.
		require.True(t, settleTaskTieredSnapshot(context.Background(), settled, 120000))
		assert.Equal(t, 600, reloadTask(t, task.ID).Quota)
		assert.Equal(t, 10900, getUserQuota(t, 8991))
	})

	t.Run("the reconcile scan picks up an unsettled Seedance usage task", func(t *testing.T) {
		truncate(t)
		seedUser(t, 8991, 10000)
		task := makeSeedanceUsageTask(t, expr, budget, 1500)
		reportSeedanceUsage(t, task, 120000, "usage.completion_tokens")
		summary := ReconcileTaskBilling(context.Background(), 100)
		assert.Equal(t, 1, summary.Scanned)
		assert.Equal(t, 600, reloadTask(t, task.ID).Quota)
		assert.Equal(t, 10900, getUserQuota(t, 8991), "only the difference is refunded")
	})

	t.Run("a native usage task is not scanned by the local reconcile", func(t *testing.T) {
		truncate(t)
		seedUser(t, 8991, 10000)
		task := makeTask(8991, 0, 1000, 0, BillingSourceWallet, 0)
		task.TaskID = model.GenerateTaskID()
		task.Status = model.TaskStatusSuccess
		snap := &billingexpr.BillingSnapshot{TaskUsageBilling: true}
		task.PrivateData.BillingContext.TieredSnapshot = snap
		model.AttachAsyncTaskBilling(&task.PrivateData, &relaycommon.RelayInfo{TieredBillingSnapshot: snap}, 1000)
		assert.Nil(t, task.PrivateData.AsyncBilling, "native usage billing must not acquire the local state machine")
		require.NoError(t, model.DB.Create(task).Error)
		stored := reloadTask(t, task.ID)
		stored.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStateDebt}
		stored.PrivateData.BillingContext.TieredSnapshot = snap
		assert.True(t, stored.HasTaskUsageBilling())
		assert.False(t, stored.HasSeedanceBillingFacts())
		assert.Empty(t, model.GetTerminalTasksPendingBilling(time.Now().Unix(), 100), "mixed native state stays out of the scan")
	})
}

func TestSeedanceFrozenConditionExpressionSettlesWithoutWaiting(t *testing.T) {
	expr := `u("resolution") == "4k" ? tier("4k", 0.7) : tier("base", 0.5)`
	truncate(t)
	seedUser(t, 8991, 10000)
	task := makeSeedanceUsageTask(t, expr, 0, 500)
	assert.False(t, seedancebilling.RequiresMeasuredTokens(expr))
	result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
	prepareTerminalTaskBilling(task, result)
	require.True(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, result))
	settled := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
	// 1080p falls into the base tier: $0.5 × 1000 = 500 quota, hold unchanged.
	assert.Equal(t, 500, settled.Quota)
	assert.Equal(t, 10000, getUserQuota(t, 8991))
}

func TestSeedanceUsageTaskFailureRefundsFullHold(t *testing.T) {
	expr := `tier("base", u("tokens") * 5 / 1000000)`
	truncate(t)
	seedUser(t, 8991, 10000)
	task := makeSeedanceUsageTask(t, expr, 300000, 1500)
	task.Status = model.TaskStatusFailure
	require.NoError(t, model.DB.Save(task).Error)
	stored := reloadTask(t, task.ID)
	result := &relaycommon.TaskInfo{Status: model.TaskStatusFailure}
	prepareTerminalTaskBilling(stored, result)
	require.False(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, stored, result))
	refundTaskWithReconcile(context.Background(), stored, "provider failed")
	assert.Zero(t, reloadTask(t, task.ID).Quota)
	assert.Equal(t, 11500, getUserQuota(t, 8991), "the full hold is refunded against the seeded wallet")
}

func TestSeedanceFundingTargetRejectsSettleWithoutMeasuredUsage(t *testing.T) {
	// 绕过轮询直接申请资金目标：数据库资金入口执行同一实测依赖规则。
	truncate(t)
	seedUser(t, 8991, 10000)
	task := makeSeedanceUsageTask(t, `tier("base", u("tokens") * 5 / 1000000)`, 300000, 1500)
	_, _, err := model.ApplyTaskBillingTarget(task, 900)
	require.ErrorContains(t, err, "cannot settle without required provider usage")
	assert.Equal(t, 10000, getUserQuota(t, 8991))
}

func TestSeedanceFixedAndZeroUsageSettlement(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		reported         bool
		expected         int
	}{
		{"fixed USD", `tier("fixed", 0.5)`, false, 500},
		{"explicit zero usage", `tier("tokens", u("tokens") * 5 / 1000000)`, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 8991, 10000)
			task := makeSeedanceUsageTask(t, tc.expression, 300000, 1500)
			if tc.reported {
				reportSeedanceUsage(t, task, 0, "usage.completion_tokens")
			}
			stored := reloadTask(t, task.ID)
			result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
			prepareTerminalTaskBilling(stored, result)
			require.True(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, stored, result))
			settled := reloadTask(t, task.ID)
			assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
			assert.Equal(t, tc.expected, settled.Quota)
			assert.Equal(t, tc.reported, settled.PrivateData.AsyncBilling.ActualUsageReported)
			assert.Equal(t, 11500-tc.expected, getUserQuota(t, 8991))
			require.True(t, settleTaskTieredSnapshot(context.Background(), settled, 300000))
			assert.Equal(t, 11500-tc.expected, getUserQuota(t, 8991), "repeat settlement cannot charge the budget")
		})
	}
}

func TestSeedanceUSDSettlementPreservesFrozenContractDiscount(t *testing.T) {
	truncate(t)
	seedUser(t, 8991, 10000)
	task := makeSeedanceUsageTask(t, `tier("base", u("tokens") * 5 / 1000000)`, 300000, 1044)
	task.PrivateData.AsyncBilling.TieredSnapshot.GroupRatio = 0.87
	task.PrivateData.BillingContext.ContractFact = contractBillingFact()
	require.NoError(t, model.DB.Save(task).Error)
	reportSeedanceUsage(t, task, 120000, "usage.completion_tokens")
	stored := reloadTask(t, task.ID)
	require.True(t, settleTaskTieredSnapshot(context.Background(), stored, 300000))
	settled := reloadTask(t, task.ID)
	assert.Equal(t, 418, settled.Quota, "$0.6 × 1000 × 0.87 × 0.8, rounded once")
	assert.Equal(t, 10626, getUserQuota(t, 8991))
	require.True(t, settleTaskTieredSnapshot(context.Background(), settled, 300000))
	assert.Equal(t, 10626, getUserQuota(t, 8991))
}
