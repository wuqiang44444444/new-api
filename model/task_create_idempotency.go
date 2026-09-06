package model

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskCreateIdempotencyCreating          = "creating"
	TaskCreateIdempotencyUpstreamSucceeded = "upstream_succeeded"
	TaskCreateIdempotencyComplete          = "complete"
	TaskCreateIdempotencyCompletedNoReplay = "completed_no_replay"
	TaskCreateIdempotencyUnknown           = "unknown"
)

var ErrTaskCreateIdempotencyConflict = errors.New("idempotency key was already used with a different request")

type TaskCreateIdempotency struct {
	ID               int64           `json:"-" gorm:"primaryKey"`
	UserID           int             `json:"-" gorm:"uniqueIndex:idx_task_create_idempotency_scope"`
	Protocol         string          `json:"-" gorm:"type:varchar(32);uniqueIndex:idx_task_create_idempotency_scope"`
	KeyHash          string          `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_task_create_idempotency_scope"`
	RequestHash      string          `json:"-" gorm:"type:varchar(64)"`
	AttemptID        string          `json:"-" gorm:"type:varchar(64);index"`
	Status           string          `json:"-" gorm:"type:varchar(20);index"`
	TaskID           string          `json:"-" gorm:"type:varchar(191);index"`
	UpstreamTaskID   string          `json:"-" gorm:"type:varchar(191);index"`
	ChannelID        int             `json:"-" gorm:"index"`
	RecoverySnapshot json.RawMessage `json:"-" gorm:"type:json"`
	ExpiresAt        int64           `json:"-" gorm:"bigint;index"`
	CreatedAt        int64           `json:"-" gorm:"bigint"`
	UpdatedAt        int64           `json:"-" gorm:"bigint"`
}

func ClaimTaskCreateIdempotency(userID int, protocol, keyHash, requestHash string, expiresAt int64) (*TaskCreateIdempotency, bool, error) {
	now := common.GetTimestamp()
	claim := &TaskCreateIdempotency{
		UserID: userID, Protocol: strings.TrimSpace(protocol),
		KeyHash: keyHash, RequestHash: requestHash,
		Status: TaskCreateIdempotencyCreating, ExpiresAt: expiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(claim)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return claim, true, nil
	}
	var existing TaskCreateIdempotency
	if err := DB.Where("user_id = ? AND protocol = ? AND key_hash = ?", userID, claim.Protocol, keyHash).First(&existing).Error; err != nil {
		return nil, false, err
	}
	// 图片协议的 claim 到期重置必须先确认绑定任务已终态：排队、执行、
	// 待核实及未完成交付/结算期间保留绑定（方案 §3.7，评审 S7）。
	retainForImage := existing.Protocol == TaskClientProtocolImageOpenAIV1 &&
		ImageTaskRequiresIdempotencyRetention(userID, existing.TaskID)
	if existing.ExpiresAt <= now && !retainForImage &&
		(existing.Status == TaskCreateIdempotencyComplete || existing.Status == TaskCreateIdempotencyCompletedNoReplay) {
		result = DB.Model(&TaskCreateIdempotency{}).
			Where("id = ? AND expires_at <= ? AND status IN ?", existing.ID, now, []string{
				TaskCreateIdempotencyComplete,
				TaskCreateIdempotencyCompletedNoReplay,
			}).
			Updates(map[string]any{
				"request_hash":      requestHash,
				"attempt_id":        "",
				"status":            TaskCreateIdempotencyCreating,
				"task_id":           "",
				"upstream_task_id":  "",
				"channel_id":        0,
				"recovery_snapshot": nil,
				"expires_at":        expiresAt,
				"created_at":        now,
				"updated_at":        now,
			})
		if result.Error != nil {
			return nil, false, result.Error
		}
		if result.RowsAffected == 1 {
			existing.RequestHash = requestHash
			existing.AttemptID = ""
			existing.Status = TaskCreateIdempotencyCreating
			existing.TaskID = ""
			existing.UpstreamTaskID = ""
			existing.ChannelID = 0
			existing.RecoverySnapshot = nil
			existing.ExpiresAt = expiresAt
			existing.CreatedAt = now
			existing.UpdatedAt = now
			return &existing, true, nil
		}
		if err := DB.Where("id = ?", existing.ID).First(&existing).Error; err != nil {
			return nil, false, err
		}
	}
	if existing.RequestHash != requestHash {
		return nil, false, ErrTaskCreateIdempotencyConflict
	}
	return &existing, false, nil
}

func BindTaskCreateIdempotencyAttempt(id int64, attemptID string) error {
	return bindTaskCreateIdempotencyAttempt(DB, id, attemptID)
}

func bindTaskCreateIdempotencyAttempt(tx *gorm.DB, id int64, attemptID string) error {
	if id == 0 {
		return nil
	}
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" {
		return errors.New("task create attempt id is required")
	}
	result := tx.Model(&TaskCreateIdempotency{}).
		Where("id = ? AND status = ? AND (attempt_id = ? OR attempt_id = '')",
			id,
			TaskCreateIdempotencyCreating,
			attemptID,
		).
		Updates(map[string]any{"attempt_id": attemptID, "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task idempotency claim is no longer active")
	}
	return nil
}

type taskCreateRecoverySnapshot struct {
	Task        Task            `json:"task"`
	PrivateData TaskPrivateData `json:"private_data"`
}

// RecordTaskCreateUpstreamSuccess durably records the provider outcome before
// local settlement and task persistence. A repeated create request can then
// finish the local transaction without submitting a second upstream job.
func RecordTaskCreateUpstreamSuccess(id int64, task *Task) error {
	if id == 0 {
		return nil
	}
	if task == nil || strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(task.PrivateData.UpstreamTaskID) == "" {
		return errors.New("task recovery snapshot is incomplete")
	}
	snapshot, err := common.Marshal(taskCreateRecoverySnapshot{
		Task:        *task,
		PrivateData: task.PrivateData,
	})
	if err != nil {
		return err
	}
	result := DB.Model(&TaskCreateIdempotency{}).
		Where("id = ? AND status = ?", id, TaskCreateIdempotencyCreating).
		Updates(map[string]any{
			"status":            TaskCreateIdempotencyUpstreamSucceeded,
			"task_id":           task.TaskID,
			"upstream_task_id":  task.PrivateData.UpstreamTaskID,
			"channel_id":        task.ChannelId,
			"recovery_snapshot": json.RawMessage(snapshot),
			"updated_at":        common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task idempotency claim is no longer active")
	}
	return nil
}

// RecoverTaskCreateIdempotency completes local persistence from a previously
// journaled provider success. It never calls the provider.
func RecoverTaskCreateIdempotency(id int64) (*Task, error) {
	var recovered *Task
	err := DB.Transaction(func(tx *gorm.DB) error {
		var claim TaskCreateIdempotency
		if err := lockForUpdate(tx).Where("id = ?", id).First(&claim).Error; err != nil {
			return err
		}
		if claim.Status == TaskCreateIdempotencyComplete && claim.TaskID != "" {
			var existing Task
			if err := tx.Where("user_id = ? AND task_id = ? AND client_protocol = ?", claim.UserID, claim.TaskID, claim.Protocol).First(&existing).Error; err != nil {
				return err
			}
			recovered = &existing
			return nil
		}
		if claim.Status != TaskCreateIdempotencyUpstreamSucceeded || len(claim.RecoverySnapshot) == 0 {
			return errors.New("task create outcome is not recoverable")
		}
		var snapshot taskCreateRecoverySnapshot
		if err := common.Unmarshal(claim.RecoverySnapshot, &snapshot); err != nil {
			return err
		}
		task := snapshot.Task
		task.ID = 0
		task.PrivateData = snapshot.PrivateData
		task.BillingState = deriveBillingState(task.PrivateData)
		if task.UserId != claim.UserID || task.TaskID != claim.TaskID || task.ClientProtocol != claim.Protocol ||
			task.PrivateData.UpstreamTaskID != claim.UpstreamTaskID || task.ChannelId != claim.ChannelID {
			return errors.New("task recovery snapshot does not match its idempotency journal")
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		result := tx.Model(&TaskCreateIdempotency{}).
			Where("id = ? AND status = ?", claim.ID, TaskCreateIdempotencyUpstreamSucceeded).
			Updates(map[string]any{
				"status":            TaskCreateIdempotencyComplete,
				"recovery_snapshot": nil,
				"updated_at":        common.GetTimestamp(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("task idempotency recovery lost its journal")
		}
		recovered = &task
		return nil
	})
	return recovered, err
}

func MarkTaskCreateIdempotencyUnknown(id int64) error {
	return DB.Model(&TaskCreateIdempotency{}).
		Where("id = ? AND status = ?", id, TaskCreateIdempotencyCreating).
		Updates(map[string]any{"status": TaskCreateIdempotencyUnknown, "updated_at": common.GetTimestamp()}).Error
}

func MarkTaskCreateIdempotencyCompletedNoReplay(id int64) error {
	if id == 0 {
		return nil
	}
	result := DB.Model(&TaskCreateIdempotency{}).
		Where("id = ? AND status IN ?", id, []string{
			TaskCreateIdempotencyCreating,
			TaskCreateIdempotencyUpstreamSucceeded,
		}).
		Updates(map[string]any{
			"status":            TaskCreateIdempotencyCompletedNoReplay,
			"recovery_snapshot": nil,
			"updated_at":        common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("task idempotency claim is no longer active")
	}
	return nil
}

func ReleaseTaskCreateIdempotency(id int64) error {
	return DB.Where("id = ? AND status = ?", id, TaskCreateIdempotencyCreating).
		Delete(&TaskCreateIdempotency{}).Error
}

func InsertTaskWithIdempotency(task *Task, idempotencyID int64) error {
	if task == nil {
		return errors.New("task is required")
	}
	task.BillingState = deriveBillingState(task.PrivateData)
	if idempotencyID == 0 {
		return DB.Create(task).Error
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		result := tx.Model(&TaskCreateIdempotency{}).
			Where("id = ? AND status IN ?", idempotencyID, []string{
				TaskCreateIdempotencyCreating,
				TaskCreateIdempotencyUpstreamSucceeded,
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
			return errors.New("task idempotency claim is no longer active")
		}
		return nil
	})
}
