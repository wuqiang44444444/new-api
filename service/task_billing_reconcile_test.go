package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
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

func TestTieredSettlementNeverSupplementsBeyondUpperBound(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	const userID = 8102
	const initialQuota = 100
	seedUser(t, userID, initialQuota)
	task := persistedAsyncTask(t, userID, 200, model.TaskStatusSuccess)
	task.PrivateData.AsyncBilling.TieredSnapshot = tieredTestSnapshot(`tier("base", c * 1000000)`, 200)
	require.NoError(t, task.UpdateBilling())

	settled := settleTaskTieredSnapshot(ctx, task, 1000)
	require.True(t, settled)
	debt := reloadTask(t, task.ID)
	assert.Equal(t, model.TaskBillingStateDebt, debt.PrivateData.AsyncBilling.State)
	assert.Equal(t, initialQuota, getUserQuota(t, userID))
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
