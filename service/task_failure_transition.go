package service

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func failTaskWithBilling(ctx context.Context, task *model.Task, reason string) (bool, error) {
	if task == nil || !task.Status.IsActive() {
		return false, nil
	}
	fromStatus := task.Status
	now := common.GetTimestamp()
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = now
	task.FailReason = reason
	if task.PrivateData.AsyncBilling != nil {
		task.PrivateData.AsyncBilling.Operation = "refund"
		task.PrivateData.AsyncBilling.Reason = reason
	}
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil || !won {
		return won, err
	}
	if task.Quota != 0 {
		refundTaskWithReconcile(ctx, task, reason)
	}
	return true, nil
}
