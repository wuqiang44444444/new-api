package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// Incorrectly attached pre-fix state cannot prove whether funds have settled.
// Preserve it for reconciliation instead of reopening or reinterpreting it.
// Seedance Link tasks are the registered exception: their USD expression is
// frozen together with the typed VideoUpstreamProtocol identity, so the local
// async billing state machine owns their funding lifecycle even though the
// amount is denominated through the native u() engine.
func rejectTaskUsageAsyncBilling(ctx context.Context, task *model.Task) bool {
	if task.PrivateData.AsyncBilling == nil || !task.HasTaskUsageBilling() || task.HasSeedanceBillingFacts() {
		return false
	}
	logger.LogWarn(ctx, fmt.Sprintf("task %s native usage billing has unexpected local async state; manual reconciliation required", task.TaskID))
	return true
}
