package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// Incorrectly attached pre-fix state cannot prove whether funds have settled.
// Preserve it for reconciliation instead of reopening or reinterpreting it.
func rejectTaskUsageAsyncBilling(ctx context.Context, task *model.Task) bool {
	if task.PrivateData.AsyncBilling == nil || !task.HasTaskUsageBilling() {
		return false
	}
	logger.LogWarn(ctx, fmt.Sprintf("task %s native usage billing has unexpected local async state; manual reconciliation required", task.TaskID))
	return true
}
