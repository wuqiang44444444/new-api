package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/asset_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AssetJobPending   = "pending"
	AssetJobRunning   = "running"
	AssetJobSucceeded = "succeeded"
	AssetJobFailed    = "failed"
	AssetJobDead      = "dead"
)

type AssetOperationJob struct {
	ID              int64  `json:"id" gorm:"primaryKey"`
	IdempotencyKey  string `json:"-" gorm:"type:varchar(191);uniqueIndex"`
	Kind            string `json:"kind" gorm:"type:varchar(64);index"`
	AssetID         *int64 `json:"-" gorm:"index"`
	BindingID       *int64 `json:"-" gorm:"index"`
	GroupBindingID  *int64 `json:"-" gorm:"index"`
	AuthorizationID *int64 `json:"-" gorm:"index"`
	ChannelID       *int   `json:"-" gorm:"index"`
	Status          string `json:"status" gorm:"type:varchar(32);index"`
	AttemptCount    int    `json:"attempt_count"`
	MaxAttempts     int    `json:"max_attempts"`
	NextAttemptAt   int64  `json:"next_attempt_at" gorm:"bigint;index"`
	LockedBy        string `json:"-" gorm:"type:varchar(128);index"`
	LockedUntil     int64  `json:"-" gorm:"bigint;index"`
	LastError       string `json:"-" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint;index"`
}

func (j *AssetOperationJob) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if j.CreatedAt == 0 {
		j.CreatedAt = now
	}
	j.UpdatedAt = now
	if j.Status == "" {
		j.Status = AssetJobPending
	}
	if j.MaxAttempts <= 0 {
		j.MaxAttempts = asset_setting.Current().JobMaxAttempts
	}
	return nil
}

// EnsureAssetOperationJob creates an idempotent job and can revive a terminal
// attempt when the user explicitly repeats the operation.
func EnsureAssetOperationJob(tx *gorm.DB, job *AssetOperationJob, requeueTerminal bool) (*AssetOperationJob, error) {
	create := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(job)
	if create.Error != nil {
		return nil, create.Error
	}
	if create.RowsAffected == 1 {
		return job, nil
	}
	var existing AssetOperationJob
	if lookupErr := tx.Where("idempotency_key = ?", job.IdempotencyKey).First(&existing).Error; lookupErr != nil {
		return nil, lookupErr
	}
	if requeueTerminal && (existing.Status == AssetJobDead || existing.Status == AssetJobSucceeded) {
		now := common.GetTimestamp()
		updates := map[string]any{
			"kind": job.Kind, "status": AssetJobPending,
			"attempt_count": 0, "max_attempts": job.MaxAttempts, "next_attempt_at": int64(0),
			"locked_by": "", "locked_until": int64(0), "last_error": "", "updated_at": now,
		}
		if result := tx.Model(&AssetOperationJob{}).Where("id = ? AND status IN ?", existing.ID, []string{AssetJobDead, AssetJobSucceeded}).Updates(updates); result.Error != nil {
			return nil, result.Error
		} else if result.RowsAffected == 1 {
			existing.Kind = job.Kind
			existing.Status = AssetJobPending
			existing.AttemptCount = 0
			existing.MaxAttempts = job.MaxAttempts
			existing.NextAttemptAt = 0
			existing.LockedBy = ""
			existing.LockedUntil = 0
			existing.LastError = ""
		}
	}
	return &existing, nil
}

func ClaimNextAssetOperationJob(runnerID string, leaseSeconds int64, allowedKinds []string) (*AssetOperationJob, error) {
	var claimed AssetOperationJob
	now := common.GetTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var job AssetOperationJob
		eligible := tx.Where("((status IN ? AND next_attempt_at <= ?) OR (status = ? AND locked_until < ?))", []string{AssetJobPending, AssetJobFailed}, now, AssetJobRunning, now)
		if len(allowedKinds) > 0 {
			eligible = eligible.Where("kind IN ?", allowedKinds)
		}
		err := lockForUpdate(eligible).Order("id asc").First(&job).Error
		if err != nil {
			return err
		}
		updates := map[string]any{"status": AssetJobRunning, "locked_by": runnerID, "locked_until": now + leaseSeconds, "updated_at": now}
		claim := tx.Model(&AssetOperationJob{}).Where("id = ? AND ((status IN ? AND next_attempt_at <= ?) OR (status = ? AND locked_until < ?))", job.ID, []string{AssetJobPending, AssetJobFailed}, now, AssetJobRunning, now).Updates(updates)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		claimed = job
		claimed.Status = AssetJobRunning
		claimed.LockedBy = runnerID
		claimed.LockedUntil = now + leaseSeconds
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &claimed, err
}

func FinishAssetOperationJob(id int64, runnerID string) error {
	return FinishAssetOperationJobTx(DB, id, runnerID)
}

func FinishAssetOperationJobTx(tx *gorm.DB, id int64, runnerID string) error {
	now := common.GetTimestamp()
	result := tx.Model(&AssetOperationJob{}).Where("id = ? AND status = ? AND locked_by = ?", id, AssetJobRunning, runnerID).Updates(map[string]any{
		"status":       AssetJobSucceeded,
		"locked_by":    "",
		"locked_until": 0,
		"last_error":   "",
		"updated_at":   now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func RetryAssetOperationJob(job *AssetOperationJob, operationErr error) error {
	now := common.GetTimestamp()
	attempts := job.AttemptCount + 1
	status := AssetJobFailed
	if attempts >= job.MaxAttempts {
		status = AssetJobDead
	}
	delay := time.Duration(1<<min(attempts, 8)) * time.Second
	result := DB.Model(&AssetOperationJob{}).Where("id = ? AND status = ? AND locked_by = ?", job.ID, AssetJobRunning, job.LockedBy).Updates(map[string]any{
		"status":          status,
		"attempt_count":   attempts,
		"next_attempt_at": now + int64(delay.Seconds()),
		"locked_by":       "",
		"locked_until":    0,
		"last_error":      operationErr.Error(),
		"updated_at":      now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func RescheduleAssetOperationJob(id int64, runnerID, kind string, delaySeconds int64) error {
	return RescheduleAssetOperationJobTx(DB, id, runnerID, kind, delaySeconds)
}

func RescheduleAssetOperationJobTx(tx *gorm.DB, id int64, runnerID, kind string, delaySeconds int64) error {
	now := common.GetTimestamp()
	result := tx.Model(&AssetOperationJob{}).Where("id = ? AND status = ? AND locked_by = ?", id, AssetJobRunning, runnerID).Updates(map[string]any{
		"kind":            kind,
		"status":          AssetJobPending,
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"next_attempt_at": now + delaySeconds,
		"locked_by":       "",
		"locked_until":    0,
		"updated_at":      now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func ScheduleNextAssetOperationJob(id int64, runnerID string, delaySeconds int64) error {
	now := common.GetTimestamp()
	result := DB.Model(&AssetOperationJob{}).Where("id = ? AND status = ? AND locked_by = ?", id, AssetJobRunning, runnerID).Updates(map[string]any{
		"status":          AssetJobPending,
		"attempt_count":   0,
		"next_attempt_at": now + delaySeconds,
		"locked_by":       "",
		"locked_until":    0,
		"last_error":      "",
		"updated_at":      now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
