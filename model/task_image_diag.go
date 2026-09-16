package model

import "errors"

// TaskImageFailureEvidence carries the bounded execution facts captured at the
// provider boundary (F3): the upstream HTTP status, the sanitized whitelist
// request-correlation ID, and whether the transition carries the existing
// violation-fee policy's fixed marker match. Facts only — never a provider
// body, arbitrary provider code/type, media, or credentials.
type TaskImageFailureEvidence struct {
	UpstreamStatus    int
	ProviderRequestID string
	ViolationMarker   bool
}

// FinishImageTaskFailureWithEvidence commits the same CAS outcome transition
// as FinishImageTaskFailure while persisting the bounded evidence in the same
// transaction. Zero-valued evidence fields leave existing facts untouched.
func FinishImageTaskFailureWithEvidence(task *Task, status TaskStatus, code string, evidence TaskImageFailureEvidence) (bool, error) {
	if status != TaskStatusFailure && status != TaskStatusExpired && status != TaskStatusReconciliationRequired {
		return false, errors.New("invalid image task failure status")
	}
	return transitionImageTask(task, status, func(data *TaskImageExecutionData) {
		data.FailureCode = code
		if evidence.UpstreamStatus > 0 {
			data.FailureStatus = evidence.UpstreamStatus
		}
		if evidence.ProviderRequestID != "" {
			data.ProviderRequestID = evidence.ProviderRequestID
		}
		if evidence.ViolationMarker {
			data.ViolationMarker = true
		}
	})
}
