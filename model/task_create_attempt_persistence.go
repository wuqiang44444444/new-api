package model

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type taskAttemptRecoverySnapshot struct {
	Task        Task            `json:"task"`
	PrivateData TaskPrivateData `json:"private_data"`
}

func RecordTaskCreateAttemptUpstreamSuccess(id int64, task *Task) error {
	if id == 0 {
		return errors.New("task create attempt is required")
	}
	if task == nil || strings.TrimSpace(task.TaskID) == "" ||
		strings.TrimSpace(task.PrivateData.UpstreamTaskID) == "" {
		return errors.New("task attempt recovery snapshot is incomplete")
	}
	privateData := task.PrivateData
	privateData.ClientRequest.Prompt = ""
	taskCopy := *task
	taskCopy.PrivateData = privateData
	snapshot, err := common.Marshal(taskAttemptRecoverySnapshot{
		Task:        taskCopy,
		PrivateData: privateData,
	})
	if err != nil {
		return err
	}
	result := DB.Model(&TaskCreateAttempt{}).
		Where("id = ? AND status IN ? AND billing_hold_state = ?",
			id,
			[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown},
			TaskCreateAttemptBillingHeld).
		Updates(map[string]any{
			"status":              TaskCreateAttemptUpstreamSucceeded,
			"upstream_task_id":    task.PrivateData.UpstreamTaskID,
			"upstream_request_id": task.PrivateData.UpstreamRequestID,
			"recovery_snapshot":   json.RawMessage(snapshot),
			"next_attempt_at":     common.GetTimestamp() + 30,
			"updated_at":          common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task create attempt is no longer sending")
	}
	return nil
}

func InsertTaskWithCreateAttempt(task *Task, idempotencyID, attemptID int64) error {
	if task == nil || attemptID == 0 {
		return errors.New("task and create attempt are required")
	}
	var transfer taskAttemptTransferBillingResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "id = ?", attemptID).Error; err != nil {
			return err
		}
		var err error
		transfer, err = settleTaskCreateAttemptTransferTx(tx, &attempt, task)
		if err != nil {
			return err
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		attemptResult := tx.Model(&TaskCreateAttempt{}).
			Where("id = ? AND status = ? AND billing_hold_state = ?",
				attemptID, TaskCreateAttemptUpstreamSucceeded, TaskCreateAttemptBillingHeld).
			Updates(map[string]any{
				"status":                     TaskCreateAttemptComplete,
				"billing_hold_state":         TaskCreateAttemptBillingTransferred,
				"frozen_connection_snapshot": nil,
				"recovery_snapshot":          nil,
				"next_attempt_at":            0,
				"updated_at":                 common.GetTimestamp(),
			})
		if attemptResult.Error != nil {
			return attemptResult.Error
		}
		if attemptResult.RowsAffected != 1 {
			return errors.New("task create attempt transfer was lost")
		}
		if err := tx.Model(&TaskAssetAuthorization{}).
			Where("attempt_id = ? AND state = ?", attempt.AttemptID, TaskAssetAuthorizationReserved).
			Updates(map[string]any{
				"task_id":    task.TaskID,
				"state":      TaskAssetAuthorizationTaskBound,
				"updated_at": common.GetTimestamp(),
			}).Error; err != nil {
			return err
		}
		if idempotencyID != 0 {
			result := tx.Model(&TaskCreateIdempotency{}).
				Where("id = ? AND attempt_id = ? AND status IN ?", idempotencyID, attempt.AttemptID, []string{
					TaskCreateIdempotencyCreating,
					TaskCreateIdempotencyUpstreamSucceeded,
					TaskCreateIdempotencyUnknown,
				}).
				Updates(map[string]any{
					"status":            TaskCreateIdempotencyComplete,
					"task_id":           task.TaskID,
					"upstream_task_id":  task.PrivateData.UpstreamTaskID,
					"channel_id":        task.ChannelId,
					"recovery_snapshot": nil,
					"updated_at":        common.GetTimestamp(),
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("task idempotency claim transfer was lost")
			}
		}
		return nil
	})
	if err == nil {
		syncTaskCreateAttemptTransferCache(transfer)
	}
	return err
}

func RecoverTaskCreateAttempt(id int64) (*Task, error) {
	var recovered *Task
	var transfer taskAttemptTransferBillingResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "id = ?", id).Error; err != nil {
			return err
		}
		if attempt.Status == TaskCreateAttemptComplete {
			var task Task
			if err := tx.Where("user_id = ? AND task_id = ? AND client_protocol = ?",
				attempt.UserID, attempt.PublicTaskID, attempt.ClientProtocol).First(&task).Error; err != nil {
				return err
			}
			recovered = &task
			return nil
		}
		if attempt.Status != TaskCreateAttemptUpstreamSucceeded || len(attempt.RecoverySnapshot) == 0 {
			return errors.New("task create attempt is not recoverable")
		}
		var snapshot taskAttemptRecoverySnapshot
		if err := common.Unmarshal(attempt.RecoverySnapshot, &snapshot); err != nil {
			return err
		}
		task := snapshot.Task
		task.ID = 0
		task.PrivateData = snapshot.PrivateData
		if task.UserId != attempt.UserID || task.TaskID != attempt.PublicTaskID ||
			task.ClientProtocol != attempt.ClientProtocol ||
			task.PrivateData.UpstreamTaskID != attempt.UpstreamTaskID {
			return errors.New("task recovery snapshot does not match attempt")
		}
		var err error
		transfer, err = settleTaskCreateAttemptTransferTx(tx, &attempt, &task)
		if err != nil {
			return err
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		result := tx.Model(&TaskCreateAttempt{}).
			Where("id = ? AND status = ? AND billing_hold_state = ?",
				attempt.ID, TaskCreateAttemptUpstreamSucceeded, TaskCreateAttemptBillingHeld).
			Updates(map[string]any{
				"status":                     TaskCreateAttemptComplete,
				"billing_hold_state":         TaskCreateAttemptBillingTransferred,
				"frozen_connection_snapshot": nil,
				"recovery_snapshot":          nil,
				"next_attempt_at":            0,
				"updated_at":                 common.GetTimestamp(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("task create attempt recovery lost its journal")
		}
		if err := tx.Model(&TaskAssetAuthorization{}).
			Where("attempt_id = ? AND state = ?", attempt.AttemptID, TaskAssetAuthorizationReserved).
			Updates(map[string]any{
				"task_id":    task.TaskID,
				"state":      TaskAssetAuthorizationTaskBound,
				"updated_at": common.GetTimestamp(),
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&TaskCreateIdempotency{}).
			Where("attempt_id = ? AND status IN ?", attempt.AttemptID, []string{
				TaskCreateIdempotencyCreating,
				TaskCreateIdempotencyUpstreamSucceeded,
				TaskCreateIdempotencyUnknown,
			}).
			Updates(map[string]any{
				"status":            TaskCreateIdempotencyComplete,
				"task_id":           task.TaskID,
				"upstream_task_id":  task.PrivateData.UpstreamTaskID,
				"channel_id":        task.ChannelId,
				"recovery_snapshot": nil,
				"updated_at":        common.GetTimestamp(),
			}).Error; err != nil {
			return err
		}
		recovered = &task
		return nil
	})
	if err == nil && recovered != nil {
		syncTaskCreateAttemptTransferCache(transfer)
	}
	return recovered, err
}
