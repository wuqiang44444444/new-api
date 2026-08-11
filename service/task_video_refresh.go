package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/model"
)

// RefreshVideoTask performs the ModelArk single-task GET observation with the
// adapter and connection frozen when the task was created.
func RefreshVideoTask(ctx context.Context, task *model.Task) error {
	if task == nil {
		return errors.New("video task is required")
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
