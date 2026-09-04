package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const customerModelContractUniquenessKey = "migration.customer_model_contract_uniqueness_v1"

type customerModelContractDuplicateKey struct {
	UserId      int
	PublicModel string
}

// migrateCustomerModelContractUniqueness removes historical duplicate
// (user_id, public_model) rows so AutoMigrate can create the unique index
// required by the one-contract-per-user-model rule. The most recently updated
// row of each duplicate group wins, matching the admin's latest intent.
// Fresh databases (table not created yet) are a no-op: AutoMigrate creates the
// table with the unique index directly.
func migrateCustomerModelContractUniqueness(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("migrate customer model contract uniqueness: database is nil")
	}
	if !db.Migrator().HasTable(&CustomerModelContract{}) {
		return nil
	}

	var marker Option
	err := db.Where(&Option{Key: customerModelContractUniquenessKey}).First(&marker).Error
	if err == nil && marker.Value == "done" {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("query customer model contract uniqueness marker: %w", err)
	}

	var duplicates []customerModelContractDuplicateKey
	if err := db.Model(&CustomerModelContract{}).
		Select("user_id, public_model").
		Group("user_id, public_model").
		Having("COUNT(*) > 1").
		Scan(&duplicates).Error; err != nil {
		return fmt.Errorf("find duplicate customer model contracts: %w", err)
	}
	if len(duplicates) == 0 {
		return db.Save(&Option{Key: customerModelContractUniquenessKey, Value: "done"}).Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, duplicate := range duplicates {
			var keep CustomerModelContract
			if err := tx.Where("user_id = ? AND public_model = ?", duplicate.UserId, duplicate.PublicModel).
				Order("updated_at DESC, id DESC").
				First(&keep).Error; err != nil {
				return fmt.Errorf("select surviving customer model contract for user %d model %q: %w",
					duplicate.UserId, duplicate.PublicModel, err)
			}
			result := tx.Where("user_id = ? AND public_model = ? AND id <> ?",
				duplicate.UserId, duplicate.PublicModel, keep.Id).
				Delete(&CustomerModelContract{})
			if result.Error != nil {
				return fmt.Errorf("delete duplicate customer model contracts for user %d model %q: %w",
					duplicate.UserId, duplicate.PublicModel, result.Error)
			}
			common.SysLog(fmt.Sprintf(
				"customer model contract uniqueness migration: user %d model %q kept id %d, removed %d duplicates",
				duplicate.UserId, duplicate.PublicModel, keep.Id, result.RowsAffected,
			))
		}
		return tx.Save(&Option{Key: customerModelContractUniquenessKey, Value: "done"}).Error
	})
}
