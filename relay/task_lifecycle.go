package relay

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
)

func TaskLifecycleCapabilities(task *model.Task) channel.TaskLifecycleCapabilities {
	if task == nil {
		return channel.TaskLifecycleCapabilities{}
	}
	adaptor := GetTaskAdaptor(task.Platform)
	lifecycle, ok := adaptor.(channel.TaskLifecycleAdaptor)
	if !ok {
		return channel.TaskLifecycleCapabilities{SupportsContent: true}
	}
	return lifecycle.TaskLifecycleCapabilities(task)
}

func CancelQueuedVideoTask(ctx context.Context, task *model.Task, providerChannel *model.Channel) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}
	adaptor := GetTaskAdaptor(task.Platform)
	lifecycle, ok := adaptor.(channel.TaskLifecycleAdaptor)
	if !ok || !lifecycle.TaskLifecycleCapabilities(task).SupportsCancelQueued {
		return fmt.Errorf("selected Provider does not support queued task cancellation")
	}
	return lifecycle.CancelQueuedTask(ctx, task, providerChannel)
}

func DeleteTerminalVideoTask(ctx context.Context, task *model.Task, providerChannel *model.Channel) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}
	adaptor := GetTaskAdaptor(task.Platform)
	lifecycle, ok := adaptor.(channel.TaskLifecycleAdaptor)
	if !ok || !lifecycle.TaskLifecycleCapabilities(task).SupportsDeleteTerminal {
		return fmt.Errorf("selected Provider does not support terminal task deletion")
	}
	return lifecycle.DeleteTerminalTask(ctx, task, providerChannel)
}
