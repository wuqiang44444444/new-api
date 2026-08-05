package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// RejectPreparedTaskCreateAttempt closes a prepared attempt that never
// acquired a billing hold and therefore never gained permission to send.
// The bound idempotency claim is removed in the same transaction so the
// customer can safely retry the request.
func RejectPreparedTaskCreateAttempt(id int64) (bool, error) {
	if id == 0 {
		return false, errors.New("task create attempt is required")
	}
	rejected := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "id = ?", id).Error; err != nil {
			return err
		}
		if attempt.Status != TaskCreateAttemptPrepared || attempt.BillingHoldState != TaskCreateAttemptBillingUnheld {
			return nil
		}
		now := common.GetTimestamp()
		updated := tx.Model(&TaskCreateAttempt{}).
			Where("id = ? AND status = ? AND billing_hold_state = ?", id, TaskCreateAttemptPrepared, TaskCreateAttemptBillingUnheld).
			Updates(map[string]any{
				"status":                     TaskCreateAttemptRejected,
				"billing_hold_state":         TaskCreateAttemptBillingReleased,
				"frozen_connection_snapshot": nil,
				"recovery_snapshot":          nil,
				"next_attempt_at":            0,
				"updated_at":                 now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("task create attempt rejection lost its state")
		}
		if err := tx.Where("attempt_id = ? AND status = ?", attempt.AttemptID, TaskCreateIdempotencyCreating).
			Delete(&TaskCreateIdempotency{}).Error; err != nil {
			return err
		}
		rejected = true
		return nil
	})
	return rejected, err
}
