package service

import (
	"context"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

func seedanceTaskSucceeded(task *model.Task) bool {
	return task != nil && task.Status == model.TaskStatusSuccess && task.HasSeedanceBillingFacts()
}

// Registered Seedance protocols share missing-usage recovery. Pricing dependencies
// come only from the frozen expression; provider parsing remains adapter-owned.
func awaitSeedanceUsage(ctx context.Context, task *model.Task) bool {
	async := task.PrivateData.AsyncBilling
	if !task.HasSeedanceBillingFacts() ||
		!billingexpr.RequiresUsage(async.TieredSnapshot.ExprString) {
		return false
	}
	async.Operation = "settle"
	async.Reason = "awaiting provider token usage; precharge retained"
	async.TargetQuota = nil
	setTaskBillingState(task, model.TaskBillingStateAwaitingUsage, "provider_usage_missing")
	if err := task.UpdateBilling(); err != nil {
		logger.LogWarn(ctx, "failed to persist task awaiting usage: "+err.Error())
	}
	return true
}
