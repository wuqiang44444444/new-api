package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func CompleteTaskCreateAttemptWithoutTask(id int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var attempt TaskCreateAttempt
		if err := lockForUpdate(tx).First(&attempt, "id = ?", id).Error; err != nil {
			return err
		}
		if attempt.Status == TaskCreateAttemptComplete &&
			attempt.BillingHoldState == TaskCreateAttemptBillingTransferred {
			return nil
		}
		if (attempt.Status != TaskCreateAttemptSending && attempt.Status != TaskCreateAttemptUnknown) ||
			attempt.BillingHoldState != TaskCreateAttemptBillingHeld {
			return errors.New("synchronous task create attempt is no longer sending")
		}
		now := common.GetTimestamp()
		result := tx.Model(&TaskCreateAttempt{}).
			Where("id = ? AND status IN ? AND billing_hold_state = ?",
				id,
				[]TaskCreateAttemptStatus{TaskCreateAttemptSending, TaskCreateAttemptUnknown},
				TaskCreateAttemptBillingHeld).
			Updates(map[string]any{
				"status":                     TaskCreateAttemptComplete,
				"billing_hold_state":         TaskCreateAttemptBillingTransferred,
				"frozen_connection_snapshot": nil,
				"recovery_snapshot":          nil,
				"next_attempt_at":            0,
				"updated_at":                 now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("synchronous task create attempt completion lost its state")
		}
		return nil
	})
}
