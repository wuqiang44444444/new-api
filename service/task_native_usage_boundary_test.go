package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeUsageCompletionAfterBillingAttachment(t *testing.T) {
	for _, tc := range []struct {
		name, expr, field string
		estimated, actual float64
		tokens, target    int
	}{
		{"tokens refund", `tier("base", u("tokens") * 5 / 1000000)`, "tokens", 200000, 100000, 100000, 500},
		{"tokens supplement", `tier("base", u("tokens") * 5 / 1000000)`, "tokens", 200000, 300000, 300000, 1500},
		{"seconds refund", `tier("base", u("seconds") * 0.1)`, "seconds", 10, 5, 0, 500},
		{"credits supplement", `tier("base", u("credits") * 0.1)`, "credits", 10, 15, 0, 1500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 8991, 10000)
			snap := &billingexpr.BillingSnapshot{ExprString: tc.expr, ExprHash: billingexpr.ExprHashString(tc.expr), GroupRatio: 1, QuotaPerUnit: 1000, ExprVersion: 1, TaskUsageBilling: true, UsageFacts: map[string]any{tc.field: tc.estimated}}
			task := makeTask(8991, 0, 1000, 0, BillingSourceWallet, 0)
			task.TaskID = model.GenerateTaskID()
			task.Status = model.TaskStatusSuccess
			task.PrivateData.BillingContext.TieredSnapshot = snap
			model.AttachAsyncTaskBilling(&task.PrivateData, &relaycommon.RelayInfo{TieredBillingSnapshot: snap}, 1000)
			require.NoError(t, model.DB.Create(task).Error)
			task = reloadTask(t, task.ID)
			result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, CompletionTokens: tc.tokens, CompletionTokensReported: tc.tokens > 0, UsageFacts: map[string]any{tc.field: tc.actual}}
			prepareTerminalTaskBilling(task, result)
			persistTaskCalculationFixture(t, task)
			require.True(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, result))
			assert.Equal(t, tc.target, task.Quota)
			assert.Equal(t, 11000-tc.target, getUserQuota(t, 8991))
			assert.Equal(t, tc.actual, task.PrivateData.BillingContext.TieredSnapshot.UsageFacts[tc.field])
		})
	}
}

func TestNativeUsageFailureRefundAfterBillingAttachment(t *testing.T) {
	truncate(t)
	seedUser(t, 8991, 10000)
	task := makeTask(8991, 0, 1000, 0, BillingSourceWallet, 0)
	task.TaskID = model.GenerateTaskID()
	task.Status = model.TaskStatusFailure
	snap := &billingexpr.BillingSnapshot{TaskUsageBilling: true}
	task.PrivateData.BillingContext.TieredSnapshot = snap
	model.AttachAsyncTaskBilling(&task.PrivateData, &relaycommon.RelayInfo{TieredBillingSnapshot: snap}, 1000)
	require.NoError(t, model.DB.Create(task).Error)
	task = reloadTask(t, task.ID)
	result := &relaycommon.TaskInfo{Status: model.TaskStatusFailure}
	prepareTerminalTaskBilling(task, result)
	persistTaskCalculationFixture(t, task)
	require.False(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, result))
	refundTaskWithReconcile(context.Background(), task, "provider failed")
	refundTaskWithReconcile(context.Background(), reloadTask(t, task.ID), "duplicate observation")
	assert.Zero(t, reloadTask(t, task.ID).Quota)
	assert.Equal(t, 11000, getUserQuota(t, 8991))
}

func TestNativeUsageWithHistoricalAsyncStateRequiresReconciliation(t *testing.T) {
	for _, state := range []model.TaskBillingState{model.TaskBillingStatePending, model.TaskBillingStateFailed, model.TaskBillingStateDebt, model.TaskBillingStateSettled} {
		t.Run(string(state), func(t *testing.T) {
			truncate(t)
			seedUser(t, 8991, 10000)
			task := makeTask(8991, 0, 1000, 0, BillingSourceWallet, 0)
			task.TaskID = model.GenerateTaskID()
			task.Status = model.TaskStatusSuccess
			task.PrivateData.BillingContext.TieredSnapshot = &billingexpr.BillingSnapshot{TaskUsageBilling: true}
			target := 2000
			task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{State: state, TargetQuota: &target, ActualTokens: 42}
			require.NoError(t, model.DB.Create(task).Error)
			task = reloadTask(t, task.ID)
			before := *task.PrivateData.AsyncBilling
			result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, CompletionTokens: 100000, UsageFacts: map[string]any{"tokens": 100000}}
			prepareTerminalTaskBilling(task, result)
			persistTaskCalculationFixture(t, task)
			require.True(t, settleTaskBillingOnComplete(context.Background(), &mockAdaptor{}, task, result))
			require.True(t, settleTaskTieredSnapshot(context.Background(), task, 100000))
			recalculateTaskQuotaWithReconcile(context.Background(), task, target, "must not supplement")
			task.Status = model.TaskStatusFailure
			refundTaskWithReconcile(context.Background(), task, "must not automatically refund")
			assert.Empty(t, model.GetTerminalTasksPendingBilling(time.Now().Unix(), 100))
			assert.Equal(t, before, *task.PrivateData.AsyncBilling)
			assert.Equal(t, before, *reloadTask(t, task.ID).PrivateData.AsyncBilling)
			assert.Equal(t, 1000, reloadTask(t, task.ID).Quota)
			assert.Equal(t, 10000, getUserQuota(t, 8991))
		})
	}
}
