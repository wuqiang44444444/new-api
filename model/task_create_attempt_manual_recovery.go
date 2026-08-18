package model

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrTaskCreateAttemptNotRecoverable  = errors.New("task create attempt is not recoverable")
	ErrTaskCreateAttemptAlreadyReleased = errors.New("task create attempt hold was already released")
)

// RecordTaskCreateAttemptRecoveryTemplate stores the complete local Task shape
// before the outbound POST. It intentionally contains no provider task ID; an
// administrator may add that ID only after independently verifying the provider
// accepted an ambiguous create request.
func RecordTaskCreateAttemptRecoveryTemplate(id int64, task *Task) error {
	if id <= 0 || task == nil || strings.TrimSpace(task.TaskID) == "" ||
		task.UserId <= 0 || strings.TrimSpace(task.ClientProtocol) == "" {
		return errors.New("task create attempt recovery template is incomplete")
	}
	taskCopy := *task
	privateData := task.PrivateData
	privateData.UpstreamTaskID = ""
	privateData.UpstreamRequestID = ""
	privateData.ClientRequest.Prompt = ""
	taskCopy.PrivateData = privateData
	snapshot, err := common.Marshal(taskAttemptRecoverySnapshot{
		Task:        taskCopy,
		PrivateData: privateData,
	})
	if err != nil {
		return err
	}
	result := DB.Model(&TaskCreateAttempt{}).
		Where("id = ? AND status = ? AND billing_hold_state = ?",
			id,
			TaskCreateAttemptSending,
			TaskCreateAttemptBillingHeld,
		).
		Updates(map[string]any{
			"recovery_snapshot": json.RawMessage(snapshot),
			"updated_at":        common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskCreateAttemptNotRecoverable
	}
	return nil
}

func PromoteTaskCreateAttemptManualSuccess(
	attemptID, upstreamTaskID, upstreamRequestID string,
	operatorID int,
	note string,
) (int64, error) {
	attemptID = strings.TrimSpace(attemptID)
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	validatedNote, noteErr := validateOperationalAuditNote(note)
	if attemptID == "" || upstreamTaskID == "" ||
		operatorID <= 0 || noteErr != nil ||
		len(attemptID) > 64 || len(upstreamTaskID) > 191 || len(upstreamRequestID) > 191 ||
		containsControlCharacter(upstreamTaskID) || containsControlCharacter(upstreamRequestID) {
		return 0, errors.New("manual task recovery identity is invalid")
	}
	note = validatedNote
	var internalID int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "attempt_id = ?", attemptID).Error; err != nil {
			return err
		}
		internalID = attempt.ID
		switch attempt.Status {
		case TaskCreateAttemptComplete, TaskCreateAttemptUpstreamSucceeded:
			if attempt.UpstreamTaskID != upstreamTaskID {
				return errors.New("task create attempt was recovered with a different upstream task id")
			}
			return nil
		case TaskCreateAttemptRejected:
			return ErrTaskCreateAttemptAlreadyReleased
		case TaskCreateAttemptSending, TaskCreateAttemptUnknown:
		default:
			return ErrTaskCreateAttemptNotRecoverable
		}
		if attempt.BillingHoldState != TaskCreateAttemptBillingHeld || len(attempt.RecoverySnapshot) == 0 {
			return ErrTaskCreateAttemptNotRecoverable
		}
		var snapshot taskAttemptRecoverySnapshot
		if err := common.Unmarshal(attempt.RecoverySnapshot, &snapshot); err != nil {
			return err
		}
		task := snapshot.Task
		task.PrivateData = snapshot.PrivateData
		if task.UserId != attempt.UserID || task.TaskID != attempt.PublicTaskID ||
			task.ClientProtocol != attempt.ClientProtocol {
			return errors.New("task recovery template does not match attempt")
		}
		task.PrivateData.UpstreamTaskID = upstreamTaskID
		task.PrivateData.UpstreamRequestID = upstreamRequestID
		snapshot.Task = task
		snapshot.PrivateData = task.PrivateData
		encoded, err := common.Marshal(snapshot)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		result := tx.Model(&TaskCreateAttempt{}).
			Where("id = ? AND status = ? AND billing_hold_state = ?",
				attempt.ID,
				attempt.Status,
				TaskCreateAttemptBillingHeld,
			).
			Updates(map[string]any{
				"status":               TaskCreateAttemptUpstreamSucceeded,
				"upstream_task_id":     upstreamTaskID,
				"upstream_request_id":  upstreamRequestID,
				"recovery_snapshot":    json.RawMessage(encoded),
				"next_attempt_at":      now,
				"manual_recovery_at":   now,
				"manual_recovery_by":   operatorID,
				"manual_recovery_note": note,
				"updated_at":           now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTaskCreateAttemptNotRecoverable
		}
		return nil
	})
	return internalID, err
}

func GetTaskCreateAttemptByAttemptID(attemptID string) (*TaskCreateAttempt, error) {
	var attempt TaskCreateAttempt
	err := DB.Where("attempt_id = ?", strings.TrimSpace(attemptID)).First(&attempt).Error
	return &attempt, err
}

type TaskCreateAttemptRecoveryView struct {
	AttemptID          string                            `json:"attempt_id"`
	PublicTaskID       string                            `json:"public_task_id"`
	UserID             int                               `json:"user_id"`
	ClientProtocol     string                            `json:"client_protocol"`
	ChannelID          int                               `json:"channel_id"`
	PublicModel        string                            `json:"public_model"`
	UpstreamProfile    string                            `json:"upstream_profile"`
	Status             TaskCreateAttemptStatus           `json:"status"`
	BillingHoldState   TaskCreateAttemptBillingHoldState `json:"billing_hold_state"`
	HeldQuota          int                               `json:"held_quota"`
	UpstreamRequestID  string                            `json:"upstream_request_id,omitempty"`
	UpstreamTaskID     string                            `json:"upstream_task_id,omitempty"`
	OutcomeUnknownAt   int64                             `json:"outcome_unknown_at,omitempty"`
	NextAttemptAt      int64                             `json:"next_attempt_at"`
	TaskDeadlineAt     int64                             `json:"task_deadline_at"`
	ManualRecoveryAt   int64                             `json:"manual_recovery_at,omitempty"`
	ManualRecoveryBy   int                               `json:"manual_recovery_by,omitempty"`
	ManualRecoveryNote string                            `json:"manual_recovery_note,omitempty"`
	CreatedAt          int64                             `json:"created_at"`
	UpdatedAt          int64                             `json:"updated_at"`
}

func ListTaskCreateAttemptsForRecovery(
	status TaskCreateAttemptStatus,
	limit, offset int,
) ([]TaskCreateAttemptRecoveryView, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	query := DB.Model(&TaskCreateAttempt{})
	if status == "" {
		query = query.Where("status IN ?", []TaskCreateAttemptStatus{
			TaskCreateAttemptSending,
			TaskCreateAttemptUnknown,
			TaskCreateAttemptUpstreamSucceeded,
		})
	} else {
		query = query.Where("status = ?", status)
	}
	var attempts []TaskCreateAttemptRecoveryView
	err := query.Order("id DESC").Limit(limit).Offset(offset).Scan(&attempts).Error
	return attempts, err
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}
