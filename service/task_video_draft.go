package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// keepUnconfirmedDraftForReconciliation handles both missing result facts and
// failed queries. Reaching the poll limit is never proof of a business failure.
func keepUnconfirmedDraftForReconciliation(ctx context.Context, task *model.Task, fromStatus model.TaskStatus, reason string) error {
	if fromStatus == model.TaskStatusSuccess || fromStatus == model.TaskStatusReconciliationRequired {
		return nil
	}
	task.PrivateData.PollFailures++
	if limit := constant.TaskPollMaxFailures; limit > 0 && task.PrivateData.PollFailures >= limit {
		return markTaskReconciliationRequired(ctx, task, reason)
	}
	_, err := task.UpdateWithStatus(fromStatus)
	return err
}

func applyDraftObservation(ctx context.Context, task *model.Task, result *relaycommon.TaskInfo, body []byte, fromStatus model.TaskStatus, adapterVersion relaycommon.VideoSouthboundAdapterVersion) (bool, error) {
	if !task.IsVideoDraftOperation() || result.Status != model.TaskStatusSuccess {
		return false, nil
	}
	marker := model.ModelArkDraftMarker(body)
	if _, err := relaycommon.ValidateHTTPSVideoResultURL(result.Url); err != nil && fromStatus != model.TaskStatusSuccess {
		return true, keepUnconfirmedDraftForReconciliation(ctx, task, fromStatus, "successful provider observation lacks a trusted video result")
	}
	mismatch := task.PrivateData.VideoDraftRequested && marker == model.ModelArkConfirmedOrdinary ||
		task.PrivateData.VideoDraftSourceID != "" && marker == model.ModelArkConfirmedDraft
	if mismatch {
		if fromStatus == model.TaskStatusSuccess {
			return true, nil // A later usage query cannot replace an accepted result.
		}
		return true, rejectDraftResult(ctx, task, result, linkVideoRedactResponse(adapterVersion, task.ClientProtocol, body), fromStatus)
	}
	if task.PrivateData.VideoDraftRequested && marker != model.ModelArkConfirmedDraft &&
		(fromStatus != model.TaskStatusSuccess || model.ModelArkDraftMarker(task.Data) != model.ModelArkConfirmedDraft) {
		return true, keepUnconfirmedDraftForReconciliation(ctx, task, fromStatus, "successful provider observation does not confirm a deliverable draft")
	}
	return false, nil
}

func rejectDraftResult(ctx context.Context, task *model.Task, result *relaycommon.TaskInfo, observation []byte, fromStatus model.TaskStatus) error {
	task.Status = model.TaskStatusProviderContractFailure
	task.Progress = taskcommon.ProgressComplete
	task.FinishTime = time.Now().Unix()
	task.FailReason = "The provider returned an ordinary video for a draft request"
	if task.PrivateData.VideoDraftSourceID != "" {
		task.FailReason = "The provider returned a draft for a final video request"
	}
	task.PrivateData.ResultURL = ""
	task.PrivateData.ModelArkUsage = result.ModelArkUsage

	// Preserve upstream success and meter evidence without a deliverable.
	var facts map[string]any
	if common.Unmarshal(observation, &facts) == nil {
		delete(facts, "content")
		task.Data, _ = common.Marshal(facts)
	}
	prepareTerminalTaskBilling(task, result)
	won, err := task.UpdateWithStatus(fromStatus)
	if err == nil && won {
		// Even a zero customer hold can leave a supplier cost exposure.
		refundTaskWithReconcile(ctx, task, task.FailReason)
	}
	return err
}
