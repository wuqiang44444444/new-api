package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func persistedAsyncTask(t *testing.T, userID, quota int, status model.TaskStatus) *model.Task {
	t.Helper()
	task := makeTask(userID, 0, quota, 0, BillingSourceWallet, 0)
	task.TaskID = "async-reconcile-" + time.Now().Format("150405.000000")
	task.Status = status
	task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending}
	task.BillingState = model.TaskBillingStatePending
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func reloadTask(t *testing.T, id int64) *model.Task {
	t.Helper()
	var task model.Task
	require.NoError(t, model.DB.First(&task, id).Error)
	return &task
}

func TestRefundFailureIsPersistedAndReconcileRetriesOnce(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8101
	const tokenID = 9101
	const initialQuota = 500
	const preConsumed = 300
	seedUser(t, userID, initialQuota)
	seedToken(t, tokenID, userID, "async-reconcile-token", 1000)

	task := persistedAsyncTask(t, userID, preConsumed, model.TaskStatusFailure)
	task.PrivateData.TokenId = tokenID
	task.PrivateData.BillingSource = BillingSourceSubscription
	task.PrivateData.SubscriptionId = 999999
	require.NoError(t, task.UpdateBilling())

	refundTaskWithReconcile(ctx, task, "upstream failed")
	failed := reloadTask(t, task.ID)
	require.NotNil(t, failed.PrivateData.AsyncBilling)
	assert.Equal(t, model.TaskBillingStateFailed, failed.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1, failed.PrivateData.AsyncBilling.Attempts)
	assert.Equal(t, initialQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(1), countLogs(t))

	failed.PrivateData.BillingSource = BillingSourceWallet
	failed.PrivateData.SubscriptionId = 0
	failed.PrivateData.AsyncBilling.NextRetryAt = 0
	require.NoError(t, failed.UpdateBilling())

	summary := ReconcileTaskBilling(ctx, 10)
	assert.Equal(t, 1, summary.Scanned)
	settled := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
	assert.Zero(t, settled.Quota)
	assert.Equal(t, initialQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, 1000+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, -preConsumed, getTokenUsedQuota(t, tokenID))
	assert.Equal(t, int64(2), countLogs(t))

	second := ReconcileTaskBilling(ctx, 10)
	assert.Zero(t, second.Scanned)
	assert.Equal(t, initialQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, 1000+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(2), countLogs(t))
}

func TestFailedTaskSweepUsesAtomicAsyncBillingRefund(t *testing.T) {
	truncate(t)
	const userID = 8111
	const remainingQuota = 100
	const preConsumedQuota = 200
	seedUser(t, userID, remainingQuota)
	task := persistedAsyncTask(t, userID, preConsumedQuota, model.TaskStatusFailure)
	task.FailReason = "provider task missing"
	require.NoError(t, model.DB.Model(task).Updates(map[string]any{
		"fail_reason": task.FailReason,
		"updated_at":  time.Now().Add(-refundReconciliationGracePeriod - time.Second).Unix(),
	}).Error)

	sweepUnrefundedFailedTasks(context.Background())

	refunded := reloadTask(t, task.ID)
	assert.Zero(t, refunded.Quota)
	require.NotNil(t, refunded.PrivateData.AsyncBilling)
	assert.Equal(t, model.TaskBillingStateSettled, refunded.PrivateData.AsyncBilling.State)
	assert.Equal(t, remainingQuota+preConsumedQuota, getUserQuota(t, userID))
}

func TestTieredSettlementSupplementsWhenFundingIsAvailable(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8102
	const initialQuota = 2000
	seedUser(t, userID, initialQuota)
	task := persistedAsyncTask(t, userID, 200, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("base", c * 2)`, 200)
	require.NoError(t, task.UpdateBilling())

	settled := settleTaskTieredSnapshot(ctx, task, 1000)
	require.True(t, settled)
	reloaded := reloadTask(t, task.ID)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	assert.Equal(t, model.TaskBillingStateSettled, reloaded.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1000, reloaded.Quota)
	assert.Equal(t, initialQuota-800, getUserQuota(t, userID))

	second := ReconcileTaskBilling(ctx, 10)
	assert.Zero(t, second.Scanned)
	assert.Equal(t, initialQuota-800, getUserQuota(t, userID))
}

func TestTieredSettlementKeepsSuccessfulTaskInDebtUntilSupplementCanBePaid(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8109
	const initialQuota = 100
	seedUser(t, userID, initialQuota)
	task := persistedAsyncTask(t, userID, 200, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("base", c * 2)`, 200)
	require.NoError(t, task.UpdateBilling())

	require.True(t, settleTaskTieredSnapshot(ctx, task, 1000))
	debt := reloadTask(t, task.ID)
	assert.EqualValues(t, model.TaskStatusSuccess, debt.Status)
	assert.Equal(t, model.TaskBillingStateDebt, debt.PrivateData.AsyncBilling.State)
	require.NotNil(t, debt.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, 1000, *debt.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, 200, debt.Quota)
	assert.Equal(t, initialQuota, getUserQuota(t, userID))

	require.NoError(t, model.IncreaseUserQuota(userID, 1000, true))
	debt.PrivateData.AsyncBilling.NextRetryAt = 0
	require.NoError(t, debt.UpdateBilling())

	summary := ReconcileTaskBilling(ctx, 10)
	assert.Equal(t, 1, summary.Scanned)
	settled := reloadTask(t, task.ID)
	assert.EqualValues(t, model.TaskStatusSuccess, settled.Status)
	assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1000, settled.Quota)
	assert.Equal(t, initialQuota+1000-800, getUserQuota(t, userID))

	second := ReconcileTaskBilling(ctx, 10)
	assert.Zero(t, second.Scanned)
	assert.Equal(t, initialQuota+1000-800, getUserQuota(t, userID))
}

func TestTieredSettlementUsesFrozenExpressionAndMissingUsageKeepsPrecharge(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8104
	seedUser(t, userID, 100)

	task := persistedAsyncTask(t, userID, 1000, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("frozen", c * 2)`, 1000)
	require.NoError(t, task.UpdateBilling())

	require.True(t, settleTaskTieredSnapshot(ctx, task, 100))
	settled := reloadTask(t, task.ID)
	assert.Equal(t, 100, settled.Quota)
	assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1000, getUserQuota(t, userID))

	missingUsage := persistedAsyncTask(t, userID, 700, model.TaskStatusSuccess)
	missingUsage.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("frozen", c * 999)`, 700)
	require.NoError(t, missingUsage.UpdateBilling())
	require.True(t, settleTaskTieredSnapshot(ctx, missingUsage, 0))
	unchanged := reloadTask(t, missingUsage.ID)
	assert.Equal(t, 700, unchanged.Quota)
	assert.Equal(t, model.TaskBillingStateSettled, unchanged.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1000, getUserQuota(t, userID))

	reportedZero := persistedAsyncTask(t, userID, 500, model.TaskStatusSuccess)
	reportedZero.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("frozen", c * 999)`, 500)
	reportedZero.PrivateData.AsyncBilling.ActualUsageReported = true
	require.NoError(t, reportedZero.UpdateBilling())
	require.True(t, settleTaskTieredSnapshot(ctx, reportedZero, 0))
	zeroSettled := reloadTask(t, reportedZero.ID)
	assert.Zero(t, zeroSettled.Quota)
	assert.Equal(t, model.TaskBillingStateSettled, zeroSettled.PrivateData.AsyncBilling.State)
	assert.Equal(t, 1500, getUserQuota(t, userID))
}

func TestReconcileUsesPersistedTargetAfterSettlementInterrupted(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8108
	const preConsumedQuota = 500
	const targetQuota = 200
	seedUser(t, userID, 100)

	task := persistedAsyncTask(t, userID, preConsumedQuota, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.Operation = "settle"
	task.PrivateData.AsyncBilling.Reason = "persisted image usage target"
	task.PrivateData.AsyncBilling.TargetQuota = common.GetPointer(targetQuota)
	require.NoError(t, task.UpdateBilling())

	interrupted := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStatePending, interrupted.PrivateData.AsyncBilling.State)
	require.NotNil(t, interrupted.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, targetQuota, *interrupted.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, preConsumedQuota, interrupted.Quota)

	summary := ReconcileTaskBilling(ctx, 10)
	assert.Equal(t, 1, summary.Scanned)
	settled := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateSettled, settled.PrivateData.AsyncBilling.State)
	assert.Equal(t, targetQuota, settled.Quota)
	assert.Equal(t, 100+preConsumedQuota-targetQuota, getUserQuota(t, userID))

	second := ReconcileTaskBilling(ctx, 10)
	assert.Zero(t, second.Scanned)
	assert.Equal(t, 100+preConsumedQuota-targetQuota, getUserQuota(t, userID))
}

func TestTieredSettlementLogRecordsCompletionTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8107
	seedUser(t, userID, 1000)

	// 预扣 1000；表达式 c*2，真实 token=100 → 结算额度 200 < 预扣，产生退款日志(type=6)。
	task := persistedAsyncTask(t, userID, 1000, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("frozen", c * 2)`, 1000)
	require.NoError(t, task.UpdateBilling())

	require.True(t, settleTaskTieredSnapshot(ctx, task, 100))

	log := getLastLog(t)
	require.NotNil(t, log)
	// 结算/退款日志必须回填真实 completion_tokens（修复前 RecordTaskBillingLogParams 无该字段，恒为 0）
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, 100, log.CompletionTokens)
}

func TestTerminalTaskBillingPersistsProviderEvidenceAndKeepsItAdminOnly(t *testing.T) {
	task := &model.Task{
		Properties:  model.Properties{OriginModelName: "seedance-fast", UpstreamModelName: "seedance-2-fast"},
		PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{}},
	}
	evidence := &relaycommon.ProviderBillingEvidence{
		Provider: "funcloud", TokenSource: "completionTokens", ReportedTokens: 40594,
		RawConsumption: "0.232731", ConsumptionUnit: "pointConsume",
		ProviderModel: "seedance-2-fast", Resolution: "720p",
	}
	prepareTerminalTaskBilling(task, &relaycommon.TaskInfo{
		CompletionTokens: 40594, ProviderBillingEvidence: evidence,
	})

	assert.True(t, task.PrivateData.AsyncBilling.ActualUsageReported)
	assert.Equal(t, 40594, task.PrivateData.AsyncBilling.ActualTokens)
	require.NotNil(t, task.PrivateData.AsyncBilling.ProviderBillingEvidence)
	assert.Equal(t, "0.232731", task.PrivateData.AsyncBilling.ProviderBillingEvidence.RawConsumption)
	other := taskBillingOther(task).Snapshot()
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, *evidence, adminInfo["provider_billing_evidence"])
}

func TestFirstBillingFailureKeepsProviderEvidenceInAdminLog(t *testing.T) {
	truncate(t)
	task := persistedAsyncTask(t, 8110, 500, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.ProviderBillingEvidence = &relaycommon.ProviderBillingEvidence{
		Provider: "funcloud", TokenSource: "completionTokens", ReportedTokens: 40594,
		RawConsumption: "0.232731", ConsumptionUnit: "pointConsume",
	}
	require.NoError(t, task.UpdateBilling())

	persistTaskBillingFailure(context.Background(), task, model.TaskBillingStateFailed, assert.AnError)

	log := getLastLog(t)
	require.NotNil(t, log)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, string(model.TaskBillingStateFailed), adminInfo["task_billing_state"])
	assert.Equal(t, assert.AnError.Error(), adminInfo["task_billing_error"])
	providerEvidence, ok := adminInfo["provider_billing_evidence"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "funcloud", providerEvidence["provider"])
	assert.Equal(t, "0.232731", providerEvidence["raw_consumption"])
}

func TestTerminalTaskBillingDistinguishesReportedZeroFromMissingUsage(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{}}}

	prepareTerminalTaskBilling(task, &relaycommon.TaskInfo{
		UsageReported: true, CompletionTokensReported: true,
	})

	assert.True(t, task.PrivateData.AsyncBilling.ActualUsageReported)
	assert.Zero(t, task.PrivateData.AsyncBilling.ActualTokens)
}

func TestTerminalTaskBillingKeepsUnverifiedUsageAsEvidenceWithoutBilling(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{AsyncBilling: &model.TaskAsyncBillingContext{}}}

	prepareTerminalTaskBilling(task, &relaycommon.TaskInfo{
		UsageSource:   "usage.total_tokens-usage.prompt_tokens",
		UsageEvidence: map[string]int{"usage.completion_tokens_details.reasoning_tokens": 0},
	})

	assert.False(t, task.PrivateData.AsyncBilling.ActualUsageReported)
	assert.Zero(t, task.PrivateData.AsyncBilling.ActualTokens)
	assert.Equal(t, "usage.total_tokens-usage.prompt_tokens", task.PrivateData.AsyncBilling.ActualUsageSource)
	assert.Equal(t, map[string]int{"usage.completion_tokens_details.reasoning_tokens": 0}, task.PrivateData.AsyncBilling.ActualUsageEvidence)
}

func TestFrozenExpressionFailureKeepsPrechargeForReconcile(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8105
	seedUser(t, userID, 100)
	task := persistedAsyncTask(t, userID, 500, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("broken", unknown_variable)`, 500)
	require.NoError(t, task.UpdateBilling())

	require.True(t, settleTaskTieredSnapshot(ctx, task, 100))
	failed := reloadTask(t, task.ID)
	assert.Equal(t, 500, failed.Quota)
	assert.Equal(t, model.TaskBillingStateFailed, failed.PrivateData.AsyncBilling.State)
	assert.NotEmpty(t, failed.PrivateData.AsyncBilling.Error)
	assert.Equal(t, 100, getUserQuota(t, userID))
}

func TestTieredSettlementSaturationIsAdminAuditedWithoutNegativeCharge(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8106
	seedUser(t, userID, 100)
	task := persistedAsyncTask(t, userID, 500, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("overflow", c * 1000000000000000)`, 500)
	require.NoError(t, task.UpdateBilling())

	require.True(t, settleTaskTieredSnapshot(ctx, task, 1000000))
	failed := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateDebt, failed.PrivateData.AsyncBilling.State)
	assert.Equal(t, 500, failed.Quota)
	assert.Equal(t, 100, getUserQuota(t, userID))

	log := getLastLog(t)
	require.NotNil(t, log)
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, adminInfo, "quota_saturation")
}

func TestPendingBillingScanDoesNotStarveBehindSettledHistory(t *testing.T) {
	truncate(t)
	seedUser(t, 8103, 100)
	for index := 0; index < 120; index++ {
		task := persistedAsyncTask(t, 8103, 0, model.TaskStatusSuccess)
		task.PrivateData.AsyncBilling.State = model.TaskBillingStateSettled
		require.NoError(t, task.UpdateBilling())
	}
	pending := persistedAsyncTask(t, 8103, 10, model.TaskStatusFailure)

	tasks := model.GetTerminalTasksPendingBilling(time.Now().Unix(), 1)

	require.Len(t, tasks, 1)
	assert.Equal(t, pending.ID, tasks[0].ID)
}

func TestDebtBillingRemainsEligibleAfterOrdinaryRetryLimit(t *testing.T) {
	truncate(t)
	const userID = 8112
	seedUser(t, userID, 1000)
	task := persistedAsyncTask(t, userID, 200, model.TaskStatusSuccess)
	targetQuota := 1000
	task.PrivateData.AsyncBilling.State = model.TaskBillingStateDebt
	task.PrivateData.AsyncBilling.Attempts = 10
	task.PrivateData.AsyncBilling.NextRetryAt = time.Now().Add(-time.Second).Unix()
	task.PrivateData.AsyncBilling.TargetQuota = &targetQuota
	require.NoError(t, task.UpdateBilling())

	tasks := model.GetTerminalTasksPendingBilling(time.Now().Unix(), 1)

	require.Len(t, tasks, 1)
	assert.Equal(t, task.ID, tasks[0].ID)
	assert.True(t, model.HasTerminalTasksPendingBilling())
	assert.True(t, model.HasTaskPollingWork())
}

// TestPendingBillingScanSkipsNonTerminalTasks 验证补偿扫描只在 SQL 层返回终态任务（方案 §15.3 P0-1）：
// 非终态任务 ActualTokens 恒为 0，若进入补偿会被 settleTaskTieredSnapshot 把预扣额当作目标额提前
// 标记 settled，且后续真正终态的差额结算与退款又被 settled 幂等守护吞掉。必须从扫描结果排除。
func TestPendingBillingScanSkipsNonTerminalTasks(t *testing.T) {
	truncate(t)
	const userID = 8107
	seedUser(t, userID, 1000)

	// 非终态任务（仍在运行）即使 billing_state=pending 也不应被补偿扫描。
	nonTerminal := persistedAsyncTask(t, userID, 300, model.TaskStatusInProgress)
	require.NoError(t, nonTerminal.UpdateBilling())
	// 终态失败任务仍应被扫描补偿。
	terminal := persistedAsyncTask(t, userID, 200, model.TaskStatusFailure)
	require.NoError(t, terminal.UpdateBilling())

	tasks := model.GetTerminalTasksPendingBilling(time.Now().Unix(), 10)

	require.Len(t, tasks, 1)
	assert.Equal(t, terminal.ID, tasks[0].ID)
}

func tieredTestSnapshot(expression string, preConsumed int) *billingexpr.BillingSnapshot {
	return &billingexpr.BillingSnapshot{
		BillingMode: "tiered_expr", ModelName: "test-model", ExprString: expression,
		ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1,
		EstimatedQuotaAfterGroup: preConsumed, QuotaPerUnit: 500000,
	}
}
