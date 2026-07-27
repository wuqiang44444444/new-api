package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
)

func videoTaskProviderChannel(task *model.Task) (*model.Channel, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	if model.TaskUsesFrozenVideoConnection(task) {
		channel, ok := model.FrozenVideoTaskChannel(task)
		if !ok {
			return nil, fmt.Errorf("frozen video connection is unavailable")
		}
		return channel, nil
	}
	return model.GetChannelById(task.ChannelId, true)
}
