package service

import (
	"context"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

type CancelQueuedVideoTaskFuncType func(
	ctx context.Context,
	task *model.Task,
	providerChannel *model.Channel,
) (definitiveRejection bool, err error)

// CancelQueuedVideoTaskFunc is injected by main to keep service independent
// from relay while allowing the shared scheduler to execute revocation-driven
// cancellation against the frozen provider connection.
var CancelQueuedVideoTaskFunc CancelQueuedVideoTaskFuncType

func ReconcileRequestedTaskCancellations(ctx context.Context, limit int) int {
	if ctx == nil {
		ctx = context.Background()
	}
	tasks := model.GetRequestedTaskCancellations(limit)
	processed := 0
	for _, candidate := range tasks {
		if ctx.Err() != nil {
			break
		}
		task, claimed, err := model.ClaimRequestedTaskCancellation(candidate.ID)
		if err != nil {
			logger.LogWarn(ctx, "claim revoked task cancellation failed: "+err.Error())
			continue
		}
		if !claimed {
			continue
		}
		processed++
		if !task.Status.CanRequestCancellation() {
			_, _, _ = model.CompleteTaskCancellation(
				task.ID,
				false,
				true,
				"frozen video lifecycle does not support queued cancellation",
			)
			continue
		}
		if CancelQueuedVideoTaskFunc == nil {
			_, _, _ = model.CompleteTaskCancellation(
				task.ID,
				false,
				false,
				"automatic cancellation executor is unavailable",
			)
			continue
		}
		var currentChannel *model.Channel
		if !model.TaskUsesFrozenVideoConnection(task) {
			currentChannel, err = model.GetChannelById(task.ChannelId, true)
			if err != nil {
				_, _, _ = model.CompleteTaskCancellation(
					task.ID,
					false,
					false,
					"provider channel is unavailable",
				)
				continue
			}
		}
		providerChannel, err := taskVideoProviderChannel(task, currentChannel)
		if err != nil {
			_, _, _ = model.CompleteTaskCancellation(
				task.ID,
				false,
				false,
				"frozen provider connection is unavailable",
			)
			continue
		}
		definitive, cancelErr := CancelQueuedVideoTaskFunc(ctx, task, providerChannel)
		if cancelErr != nil {
			_, _, _ = model.CompleteTaskCancellation(
				task.ID,
				false,
				definitive,
				"provider cancellation did not complete",
			)
			continue
		}
		cancelled, wonTerminal, completeErr := model.CompleteTaskCancellation(task.ID, true, false, "")
		if completeErr != nil {
			logger.LogWarn(ctx, "complete revoked task cancellation failed: "+completeErr.Error())
			continue
		}
		if wonTerminal {
			SettleConfirmedTaskCancellation(ctx, cancelled)
		}
	}
	return processed
}
