package model

import "gorm.io/gorm"

// RequireBillingStatementMaintenanceTx fences offline source repairs against
// confirmation and maintenance exit in the same database transaction. Do not
// silently admit an unmigrated database or infer maintenance from a closed switch.
func RequireBillingStatementMaintenanceTx(tx *gorm.DB, expectedGeneration int64) error {
	maintenance, err := lockBillingStatementMaintenanceTx(tx.Statement.Context, tx)
	if err != nil {
		return err
	}
	if expectedGeneration <= 0 || !maintenance.Enabled || maintenance.Generation != expectedGeneration {
		return ErrBillingStatementVersionConflict
	}
	enabled, err := billingStatementVersionEnabledFromDBTx(tx.Statement.Context, tx)
	if err != nil {
		return err
	}
	if enabled {
		return ErrBillingStatementVersionConflict
	}
	return nil
}
