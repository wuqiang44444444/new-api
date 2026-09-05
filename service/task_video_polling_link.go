package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Link video polling facts. The shared video polling loop in task_polling.go
// stays upstream-owned; every Link-specific decision is expressed through the
// narrow helpers below so the polling file keeps only single-call wiring.

// reconciliationReasonMaxChars bounds the provider-violation detail appended to
// the persisted FailReason. Reasons are local adapter constants, but the cap
// keeps the column bounded if a future reason embeds provider text.
const reconciliationReasonMaxChars = 200

// markTaskReconciliationRequired moves a polled task into the reconciliation
// state after an upstream contract violation, preserving progress context and
// the violation reason so operators can diagnose without upstream access.
func markTaskReconciliationRequired(ctx context.Context, task *model.Task, reason string) error {
	if task == nil {
		return nil
	}
	oldStatus := task.Status
	task.Status = model.TaskStatusReconciliationRequired
	task.FailReason = reconciliationFailReason(reason)
	if task.Progress == "" || task.Progress == "0%" {
		task.Progress = taskcommon.ProgressInProgress
	}
	logger.LogWarn(ctx, fmt.Sprintf("task %s marked RECONCILIATION_REQUIRED: %s", task.TaskID, task.FailReason))
	_, err := task.UpdateWithStatus(oldStatus)
	return err
}

func reconciliationFailReason(reason string) string {
	characters := []rune(reason)
	if len(characters) > reconciliationReasonMaxChars {
		reason = string(characters[:reconciliationReasonMaxChars])
	}
	if reason == "" {
		return "upstream_contract_violation"
	}
	return "upstream_contract_violation: " + reason
}

// resolveLinkVideoPollVersion resolves the frozen Link southbound adapter
// version for a video task poll. When the frozen version no longer resolves,
// the task is marked for reconciliation and proceed=false is returned
// together with the mark error.
func resolveLinkVideoPollVersion(ctx context.Context, ch *model.Channel, task *model.Task) (version relaycommon.VideoSouthboundAdapterVersion, proceed bool, err error) {
	version, versionErr := relaycommon.ResolveVideoSouthboundAdapterVersion(
		ch.Type,
		taskVideoUpstreamProfile(task, ch),
		task.PrivateData.SouthboundAdapterVersion,
	)
	if versionErr == nil {
		return version, true, nil
	}
	if markErr := markTaskReconciliationRequired(ctx, task, versionErr.Error()); markErr != nil {
		return version, false, fmt.Errorf("mark invalid adapter version for task %s: %w", task.TaskID, markErr)
	}
	return version, false, nil
}

// linkVideoContractViolationHandled reports whether a poll error is an
// upstream contract violation and, if so, routes the task to reconciliation.
func linkVideoContractViolationHandled(ctx context.Context, task *model.Task, err error) (handled bool, markErr error) {
	if err == nil {
		return false, nil
	}
	var contractViolation *relaycommon.UpstreamContractViolation
	if !errors.As(err, &contractViolation) {
		return false, nil
	}
	if markErr := markTaskReconciliationRequired(ctx, task, contractViolation.Reason); markErr != nil {
		return true, markErr
	}
	return true, nil
}

// linkVideoUpstreamTaskMissing reports whether a poll error is the provider's
// definitive "task does not exist" business response. It is treated as a
// counted poll failure instead of an immediate failure so a provider-side
// creation-to-query visibility window cannot kill freshly created tasks; the
// shared consecutive-failure cutoff then retires the task with a refund.
func linkVideoUpstreamTaskMissing(err error) bool {
	if err == nil {
		return false
	}
	var taskNotFound *relaycommon.UpstreamTaskNotFound
	return errors.As(err, &taskNotFound)
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
