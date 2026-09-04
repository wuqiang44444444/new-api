package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Link video polling facts. The shared video polling loop in task_polling.go
// stays upstream-owned; every Link-specific decision is expressed through the
// narrow helpers below so the polling file keeps only single-call wiring.

// markTaskReconciliationRequired moves a polled task into the reconciliation
// state after an upstream contract violation, preserving progress context.
func markTaskReconciliationRequired(task *model.Task) error {
	if task == nil {
		return nil
	}
	oldStatus := task.Status
	task.Status = model.TaskStatusReconciliationRequired
	task.FailReason = "upstream_contract_violation"
	if task.Progress == "" || task.Progress == "0%" {
		task.Progress = taskcommon.ProgressInProgress
	}
	_, err := task.UpdateWithStatus(oldStatus)
	return err
}

// resolveLinkVideoPollVersion resolves the frozen Link southbound adapter
// version for a video task poll. When the frozen version no longer resolves,
// the task is marked for reconciliation and proceed=false is returned
// together with the mark error.
func resolveLinkVideoPollVersion(ch *model.Channel, task *model.Task) (version relaycommon.VideoSouthboundAdapterVersion, proceed bool, err error) {
	version, versionErr := relaycommon.ResolveVideoSouthboundAdapterVersion(
		ch.Type,
		taskVideoUpstreamProfile(task, ch),
		task.PrivateData.SouthboundAdapterVersion,
	)
	if versionErr == nil {
		return version, true, nil
	}
	if markErr := markTaskReconciliationRequired(task); markErr != nil {
		return version, false, fmt.Errorf("mark invalid adapter version for task %s: %w", task.TaskID, markErr)
	}
	return version, false, nil
}

// linkVideoContractViolationHandled reports whether a poll error is an
// upstream contract violation and, if so, routes the task to reconciliation.
func linkVideoContractViolationHandled(task *model.Task, err error) (handled bool, markErr error) {
	if err == nil {
		return false, nil
	}
	var contractViolation *relaycommon.UpstreamContractViolation
	if !errors.As(err, &contractViolation) {
		return false, nil
	}
	if markErr := markTaskReconciliationRequired(task); markErr != nil {
		return true, markErr
	}
	return true, nil
}

// linkVideoRedactResponse applies the protocol-specific redaction for a polled
// response body before it is persisted on the task.
func linkVideoRedactResponse(version relaycommon.VideoSouthboundAdapterVersion, body []byte) []byte {
	if version.IsFeicaiVideos() {
		return redactTaskResponseForLog(body)
	}
	return redactVideoResponseBody(body)
}

// clearLinkVideoFailReasonOnRecovery drops a reconciliation failure marker
// when the same task is observed making progress again.
func clearLinkVideoFailReasonOnRecovery(fromStatus model.TaskStatus, task *model.Task) {
	if fromStatus != model.TaskStatusReconciliationRequired || task == nil {
		return
	}
	if task.Status == model.TaskStatusQueued ||
		task.Status == model.TaskStatusInProgress ||
		task.Status == model.TaskStatusSuccess {
		task.FailReason = ""
	}
}
