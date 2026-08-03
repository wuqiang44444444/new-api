package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func GetRequestedTaskCancellations(limit int) []*Task {
	if limit <= 0 {
		return nil
	}
	var tasks []*Task
	err := DB.Where("cancellation_state = ? AND status IN ?",
		TaskCancellationStateRequested,
		[]TaskStatus{
			TaskStatusNotStart,
			TaskStatusSubmitted,
			TaskStatusQueued,
			TaskStatusInProgress,
			TaskStatusUnknown,
			TaskStatusReconciliationRequired,
		},
	).Order("cancellation_requested_at, id").Limit(limit).Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func ClaimRequestedTaskCancellation(id int64) (*Task, bool, error) {
	if id <= 0 {
		return nil, false, errors.New("task cancellation is required")
	}
	var task Task
	claimed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&task, "id = ?", id).Error; err != nil {
			return err
		}
		if task.CancellationState != TaskCancellationStateRequested || !task.Status.IsActive() {
			return nil
		}
		now := common.GetTimestamp()
		update := tx.Model(&Task{}).
			Where("id = ? AND cancellation_state = ?", task.ID, TaskCancellationStateRequested).
			Updates(map[string]any{
				"cancellation_state": TaskCancellationStateUnknown,
				"cancellation_error": "",
				"updated_at":         now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return nil
		}
		task.CancellationState = TaskCancellationStateUnknown
		task.CancellationError = ""
		task.UpdatedAt = now
		claimed = true
		return nil
	})
	return &task, claimed, err
}
