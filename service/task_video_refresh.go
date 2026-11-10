package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/model"
)

// RefreshVideoTask observes a task with its frozen adapter and connection for
// internal use, including background usage recovery.
func RefreshVideoTask(ctx context.Context, task *model.Task) error {
	if task == nil {
		return errors.New("video task is required")
	}
	if task.Status.IsTerminal() && task.Status != model.TaskStatusSuccess {
		return nil // The accepted failure/cancellation reason is already durable.
	}
	if GetTaskAdaptorFunc == nil {
		return errors.New("video task adaptor factory is unavailable")
	}
	adaptor := GetTaskAdaptorFunc(task.Platform)
	if adaptor == nil {
		return errors.New("video task adaptor is unavailable")
	}
	providerChannel, err := taskVideoProviderChannel(task, nil)
	if err != nil {
		return err
	}
	return updateVideoSingleTask(ctx, adaptor, providerChannel, task.GetUpstreamTaskID(), map[string]*model.Task{
		task.GetUpstreamTaskID(): task,
	})
}
