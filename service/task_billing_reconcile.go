package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type TaskBillingReconcileSummary struct {
	Scanned int `json:"billing_reconcile_scanned"`
}

func prepareTerminalTaskBilling(task *model.Task, result *relaycommon.TaskInfo) {
	async := task.PrivateData.AsyncBilling
	if async == nil {
		return
	}
	async.ActualTokens = result.CompletionTokens
	if !result.CompletionTokensReported && async.ActualTokens <= 0 {
		async.ActualTokens = result.TotalTokens
	}
	async.ActualUsageReported = result.UsageReported || result.CompletionTokens > 0 || result.TotalTokens > 0
	async.ActualUsageSource = result.UsageSource
	if len(result.UsageEvidence) > 0 {
		async.ActualUsageEvidence = result.UsageEvidence
	}
	if result.ProviderBillingEvidence != nil {
		evidence := *result.ProviderBillingEvidence
		async.ProviderBillingEvidence = &evidence
	}
	if task.Status.ShouldRefundOnTerminal() {
		async.Operation = "refund"
		async.Reason = taskTerminalBillingReason(task, task.FailReason)
	}
}

// ReconcileTaskBilling retries terminal task funding operations independently
// from provider polling. Stored targets make retries deterministic and prevent
// administrator price changes from affecting in-flight tasks.
func ReconcileTaskBilling(ctx context.Context, limit int) TaskBillingReconcileSummary {
	tasks := model.GetTerminalTasksPendingBilling(time.Now().Unix(), limit)
	summary := TaskBillingReconcileSummary{Scanned: len(tasks)}
	for _, task := range tasks {
		if ctx.Err() != nil {
			break
		}
		if model.IsImageTask(task) {
			settleImageTaskBilling(ctx, task)
			continue
		}
		async := task.PrivateData.AsyncBilling
		if async == nil {
			continue
		}
		if task.Status.ShouldRefundOnTerminal() || async.Operation == "refund" {
			async.Operation = "refund"
			if async.Reason == "" {
				async.Reason = taskTerminalBillingReason(task, task.FailReason)
			}
			refundTaskWithReconcile(ctx, task, async.Reason)
			continue
		}
		if async.TargetQuota != nil {
			recalculateTaskQuotaWithReconcile(ctx, task, *async.TargetQuota, async.Reason)
			continue
		}
		if settleTaskTieredSnapshot(ctx, task, async.ActualTokens) {
			continue
		}
		if actualQuota, clamp, reason, ok := calculateTaskQuotaByTokens(task, async.ActualTokens); ok {
			async.Operation = "settle"
			async.Reason = reason
			async.TargetQuota = &actualQuota
			if err := task.UpdateBilling(); err != nil {
				persistTaskBillingFailure(ctx, task, model.TaskBillingStateFailed, err)
				continue
			}
			recalculateTaskQuotaWithReconcile(ctx, task, actualQuota, reason, clamp)
			continue
		}
		setTaskBillingState(task, model.TaskBillingStateSettled, "")
		if err := task.UpdateBilling(); err != nil {
			logger.LogWarn(ctx, "failed to finalize task billing state: "+err.Error())
		}
	}
	return summary
}
