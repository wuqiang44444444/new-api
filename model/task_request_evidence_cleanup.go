package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrTaskRequestEvidenceBodyClosed = errors.New("evidence body is closed")

// CommitTaskRequestEvidenceObject serializes the final object link against the
// cleanup marker. Staging, encryption and file fsync happen before this call;
// only the immutable link is published under the index lock. The callback runs
// in the actual storage worker, including after its caller's write timeout.
func CommitTaskRequestEvidenceObject(evidenceID, retentionSeconds int64, publish func() error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := writableEvidenceIndex(tx, evidenceID, common.GetTimestamp(), retentionSeconds); err != nil {
			return err
		}

		return publish()
	})
}

// CreateEvidenceCleanupSystemTask commits the accepted manual scope with its
// pending task. Every existing candidate is closed immediately; cancellation,
// restart and deletion failures leave durable markers for the scheduled run.
// The active-key unique constraint makes a concurrent busy request roll back
// both its task and its markers rather than changing the running scope.
func CreateEvidenceCleanupSystemTask(targetTimestamp int64, payload any) (*SystemTask, error) {
	taskID, err := GenerateSystemTaskID()
	if err != nil {
		return nil, err
	}
	payloadText, err := marshalSystemTaskJSON(payload)
	if err != nil {
		return nil, err
	}
	taskType := SystemTaskTypeTaskRequestEvidenceRetention
	task := &SystemTask{TaskID: taskID, Type: taskType, Status: SystemTaskStatusPending, ActiveKey: &taskType, Payload: payloadText}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		return tx.Model(&TaskRequestEvidence{}).
			Where("body_expired = ? AND created_at < ? AND (cleanup_requested_at = 0 OR cleanup_requested_at IS NULL)", false, targetTimestamp).
			Updates(map[string]any{"cleanup_requested_at": common.GetTimestamp(), "updated_at": common.GetTimestamp()}).Error
	})
	return task, err
}
