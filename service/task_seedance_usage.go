package service

import (
	"context"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
)

func seedanceTaskSucceeded(task *model.Task) bool {
	return task != nil && task.HasSeedanceBillingFacts() && task.Status == model.TaskStatusSuccess
}

// Registered Seedance protocols share missing-usage recovery. Pricing dependencies
// come only from the frozen expression; provider parsing remains adapter-owned.
// u() expressions wait only for the measured token meter (§5.5); pure frozen
// condition expressions settle directly, and legacy c/_task expressions keep
// the generic usage dependency.
func awaitSeedanceUsage(ctx context.Context, task *model.Task) bool {
	async := task.PrivateData.AsyncBilling
	if !task.HasSeedanceBillingFacts() ||
		!seedancebilling.RequiresMeasuredTaskUsage(async.TieredSnapshot) {
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
