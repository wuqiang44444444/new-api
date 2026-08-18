package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	TaskClientProtocolModelArkV3 = "modelark_v3"
	TaskClientProtocolKlingV1    = "kling_v1"
	TaskClientProtocolJimeng     = "jimeng_official"

	TaskCancellationStateRequested = "requested"
	TaskCancellationStateUnknown   = "unknown"
	TaskCancellationStateConfirmed = "confirmed"
	TaskCancellationStateRejected  = "rejected"
)

type TaskClientRequestSnapshot struct {
	Prompt             string `json:"prompt,omitempty"`
	Seconds            string `json:"seconds,omitempty"`
	Size               string `json:"size,omitempty"`
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"`
	ServiceTier        string `json:"service_tier,omitempty"`
}

func (s TaskStatus) IsActive() bool {
	switch s {
	case TaskStatusNotStart, TaskStatusSubmitted, TaskStatusQueued, TaskStatusInProgress, TaskStatusUnknown, TaskStatusReconciliationRequired:
		return true
	default:
		return false
	}
}

func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusSuccess, TaskStatusFailure, TaskStatusProviderContractFailure, TaskStatusCancelled, TaskStatusExpired:
		return true
	default:
		return false
	}
}

func (s TaskStatus) CanRequestCancellation() bool {
	return s == TaskStatusNotStart || s == TaskStatusSubmitted || s == TaskStatusQueued
}

func (s TaskStatus) ShouldRefundOnTerminal() bool {
	return s == TaskStatusFailure || s == TaskStatusProviderContractFailure || s == TaskStatusCancelled || s == TaskStatusExpired
}

func TerminalTaskStatuses() []TaskStatus {
	return []TaskStatus{TaskStatusSuccess, TaskStatusFailure, TaskStatusProviderContractFailure, TaskStatusCancelled, TaskStatusExpired}
}

func IsLinkVideoTaskClientProtocol(protocol string) bool {
	switch protocol {
	case TaskClientProtocolModelArkV3, TaskClientProtocolKlingV1, TaskClientProtocolJimeng:
		return true
	default:
		return false
	}
}

func GetVideoTaskForProtocol(userID, appID int, taskID, protocol string, includeDeleted bool) (*Task, bool, error) {
	if userID <= 0 || appID <= 0 || strings.TrimSpace(taskID) == "" {
		return nil, false, nil
	}
	query := DB.Where("user_id = ? AND app_id = ? AND task_id = ?", userID, appID, strings.TrimSpace(taskID))
	if protocol != "" {
		query = query.Where("client_protocol = ?", protocol)
	}
	if !includeDeleted {
		query = query.Where("client_deleted_at = ?", 0)
	}
	var task Task
	err := query.First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func MarkVideoTaskClientDeleted(userID, appID int, taskID, protocol string) (bool, error) {
	now := common.GetTimestamp()
	result := DB.Model(&Task{}).
		Where("user_id = ? AND app_id = ? AND task_id = ? AND client_protocol = ? AND client_deleted_at = ?", userID, appID, taskID, protocol, 0).
		Update("client_deleted_at", now)
	return result.RowsAffected == 1, result.Error
}

type TaskCancellationBeginResult struct {
	Task           *Task
	ShouldCall     bool
	AlreadyPending bool
}

func BeginTaskCancellation(userID, appID int, taskID, protocol string) (*TaskCancellationBeginResult, error) {
	result := &TaskCancellationBeginResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var task Task
		query := lockForUpdate(tx).Where("user_id = ? AND app_id = ? AND task_id = ? AND client_protocol = ? AND client_deleted_at = ?", userID, appID, taskID, protocol, 0)
		if err := query.First(&task).Error; err != nil {
			return err
		}
		result.Task = &task
		if !task.Status.CanRequestCancellation() {
			return nil
		}
		if task.CancellationState == TaskCancellationStateRequested || task.CancellationState == TaskCancellationStateUnknown {
			result.AlreadyPending = true
			return nil
		}
		now := common.GetTimestamp()
		if err := tx.Model(&task).Updates(map[string]any{
			"cancellation_state":        TaskCancellationStateRequested,
			"cancellation_requested_at": now,
			"cancellation_error":        "",
			"updated_at":                now,
		}).Error; err != nil {
			return err
		}
		task.CancellationState = TaskCancellationStateRequested
		task.CancellationRequestedAt = now
		task.CancellationError = ""
		result.ShouldCall = true
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return result, err
}

func CompleteTaskCancellation(taskID int64, confirmed bool, rejected bool, operationErr string) (*Task, bool, error) {
	var saved Task
	wonTerminal := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", taskID).First(&saved).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		state := TaskCancellationStateUnknown
		if confirmed && saved.Status.IsTerminal() && saved.Status != TaskStatusCancelled {
			confirmed = false
			rejected = true
			if operationErr == "" {
				operationErr = "task already reached a different terminal state"
			}
		}
		if confirmed {
			state = TaskCancellationStateConfirmed
		} else if rejected {
			state = TaskCancellationStateRejected
		}
		updates := map[string]any{
			"cancellation_state": state,
			"cancellation_error": operationErr,
			"updated_at":         now,
		}
		if confirmed && saved.Status.IsActive() {
			updates["status"] = TaskStatusCancelled
			updates["progress"] = "100%"
			updates["finish_time"] = now
			updates["cancellation_completed_at"] = now
			saved.Status = TaskStatusCancelled
			saved.Progress = "100%"
			saved.FinishTime = now
			saved.CancellationCompletedAt = now
			wonTerminal = true
		}
		if err := tx.Model(&saved).Updates(updates).Error; err != nil {
			return err
		}
		saved.CancellationState = state
		saved.CancellationError = operationErr
		saved.UpdatedAt = now
		return nil
	})
	return &saved, wonTerminal, err
}
