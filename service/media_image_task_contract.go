package service

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func reconcileMediaImageTaskContract(task *model.Task) (*model.Task, error) {
	fromStatus := task.Status
	task.Status = model.TaskStatusReconciliationRequired
	task.FailReason = "upstream_contract_violation"
	if task.Progress == "" || task.Progress == "0%" {
		task.Progress = "50%"
	}
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil || won {
		return task, err
	}
	current, exists, err := model.GetByOnlyTaskId(task.TaskID)
	if err != nil || !exists {
		return task, err
	}
	return current, nil
}

func finalizeMediaImageProviderContractFailure(
	ctx context.Context,
	task *model.Task,
) (*model.Task, error) {
	fromStatus := task.Status
	task.Status = model.TaskStatusProviderContractFailure
	task.Progress = "100%"
	task.FinishTime = common.GetTimestamp()
	task.FailReason = providerContractFailureReason
	if task.PrivateData.AsyncBilling != nil {
		task.PrivateData.AsyncBilling.Operation = "refund"
		task.PrivateData.AsyncBilling.Reason = providerContractFailureReason
	}
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil {
		return task, err
	}
	if !won {
		current, exists, loadErr := model.GetByOnlyTaskId(task.TaskID)
		if loadErr != nil || !exists {
			return task, loadErr
		}
		return current, nil
	}
	refundTaskWithReconcile(ctx, task, providerContractFailureReason)
	return task, nil
}
