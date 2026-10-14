package service

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// EvidenceCleanupPayload is the durable payload of one evidence-cleanup run.
// TargetTimestamp>0 marks a manual run with a fixed cutoff: execution and retry
// always use the stored boundary, never a recomputed "N days ago". A zero
// target is the scheduled retention run.
type EvidenceCleanupPayload struct {
	TargetTimestamp int64  `json:"target_timestamp,omitempty"`
	OperatorUserID  int    `json:"operator_user_id,omitempty"`
	Trigger         string `json:"trigger,omitempty"`
}

// StartEvidenceCleanupTask enqueues one evidence-cleanup run (Root manual
// entry). The returned bool is true only when a new pending row was created;
// false means a cleanup is already active and the existing task is returned so
// the caller can report busy without mutating the running scope.
func StartEvidenceCleanupTask(targetTimestamp int64, operatorUserID int) (*model.SystemTask, bool, error) {
	if targetTimestamp <= 0 || targetTimestamp > common.GetTimestamp() || operatorUserID <= 0 {
		return nil, false, errors.New("invalid_cleanup_target")
	}
	payload := EvidenceCleanupPayload{
		TargetTimestamp: targetTimestamp,
		OperatorUserID:  operatorUserID,
		Trigger:         "manual",
	}
	active, err := model.GetActiveSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention)
	if err != nil || active != nil {
		return active, false, err
	}
	task, err := model.CreateEvidenceCleanupSystemTask(targetTimestamp, payload)
	if err != nil {
		active, lookupErr := model.GetActiveSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention)
		if lookupErr == nil && active != nil {
			return active, false, nil
		}
		return nil, false, err
	}
	notifySystemTaskRunner()
	return task, true, nil
}
