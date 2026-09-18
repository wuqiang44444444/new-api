package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// createLog may run after money has already settled. A known missing source row
// must fence confirmation, including revision failures that abort the entire
// database transaction. This observes failure; it never retries or refunds.
func trackBillingStatementLogWriteFailure(log *Log, writeErr error) error {
	if writeErr == nil || log == nil || !BillingStatementVersionTopologyOK() ||
		(log.Type != LogTypeConsume && log.Type != LogTypeRefund) ||
		isNativeChannelTestLog(log.Type, log.TokenId, log.TokenName, log.Content) {
		return writeErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Invalidate in-flight scans even when no retention row exists yet. Existing
	// verified months also depend on cross-period evidence, so fence the customer.
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockBillingStatementMaintenanceTx(ctx, tx); err != nil {
			return err
		}
		if err := tx.Model(&BillingStatementMaintenance{}).Where("id = ?", billingStatementMaintenanceRowID).
			UpdateColumn("generation", gorm.Expr("generation + 1")).Error; err != nil {
			return err
		}
		return tx.Model(&BillingStatementRetention{}).Where("user_id = ?", log.UserId).
			Updates(map[string]interface{}{"status": BillingStatementRetentionPartial, "detail": "source log write failed; verification required", "updated_at": nowSeconds()}).Error
	})
	if err != nil {
		common.SysError("billing statement source failure fence could not be persisted; source verification required")
		return errors.Join(writeErr, err)
	}
	return writeErr
}
