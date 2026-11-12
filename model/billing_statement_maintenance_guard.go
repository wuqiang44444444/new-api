package model

import (
	"context"

	"gorm.io/gorm"
)

// RequireBillingStatementMaintenanceTx fences offline source repairs against
// confirmation and maintenance exit in the same database transaction. Do not
// silently admit an unmigrated database.
func RequireBillingStatementMaintenanceTx(tx *gorm.DB, expectedGeneration int64) error {
	maintenance, err := lockBillingStatementMaintenanceTx(tx.Statement.Context, tx)
	if err != nil {
		return err
	}
	if expectedGeneration <= 0 || !maintenance.Enabled || maintenance.Generation != expectedGeneration {
		return ErrBillingStatementVersionConflict
	}
	return nil
}

func requireBillingStatementWritableTx(ctx context.Context, tx *gorm.DB) error {
	maintenance, err := lockBillingStatementMaintenanceTx(ctx, tx)
	if err != nil {
		return err
	}
	if maintenance.Enabled {
		return ErrBillingStatementMaintenanceActive
	}
	return nil
}
