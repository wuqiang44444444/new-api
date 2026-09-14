package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// migrateCustomerContractEntityRuleIndex drops the historical
// (contract_id, public_model) unique index so AutoMigrate can create the
// (contract_id, public_model, channel_id) unique index that allows several
// same-discount channel rules of one public model inside one contract.
// Existing single-channel rule rows stay valid under the new index and are
// never rewritten. Fresh databases are a no-op: AutoMigrate creates the table
// with the new unique index directly. The drop is idempotent, so repeated
// starts and the three supported databases converge without a marker row.
func migrateCustomerContractEntityRuleIndex(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("migrate customer contract entity rule index: database is nil")
	}
	if !db.Migrator().HasTable(&CustomerContractEntityRule{}) {
		return nil
	}
	if !db.Migrator().HasIndex(&CustomerContractEntityRule{}, "idx_cc_entity_rule_contract_model") {
		return nil
	}
	if err := db.Migrator().DropIndex(&CustomerContractEntityRule{}, "idx_cc_entity_rule_contract_model"); err != nil {
		return fmt.Errorf("drop legacy customer contract entity rule index: %w", err)
	}
	common.SysLog("customer contract entity rule index migration: dropped legacy index idx_cc_entity_rule_contract_model")
	return nil
}