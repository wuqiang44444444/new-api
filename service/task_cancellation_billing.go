package service

import (
	"context"

	"github.com/QuantumNous/new-api/model"
)

// SettleConfirmedTaskCancellation moves a confirmed cancellation to a zero
// billing target through the same persistent idempotency gate used by polling.
func SettleConfirmedTaskCancellation(ctx context.Context, task *model.Task) {
	if task == nil || task.Status != model.TaskStatusCancelled {
		return
	}
	if task.PrivateData.AsyncBilling != nil {
		task.PrivateData.AsyncBilling.Operation = "refund"
		task.PrivateData.AsyncBilling.Reason = "video task cancelled"
	}
	refundTaskWithReconcile(ctx, task, "video task cancelled")
}
