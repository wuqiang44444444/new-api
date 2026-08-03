package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// migrateSQLiteVerificationTokenHash works around SQLite rejecting
// ALTER TABLE ... ADD COLUMN ... UNIQUE for an existing session table.
func migrateSQLiteVerificationTokenHash() error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}

	session := &RealPersonVerificationSession{}
	if !DB.Migrator().HasTable(session) {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		migrator := tx.Migrator()
		if !migrator.HasColumn(session, "VerificationTokenHash") {
			if err := tx.Exec(
				"ALTER TABLE `real_person_verification_sessions` ADD COLUMN `verification_token_hash` varchar(64)",
			).Error; err != nil {
				return fmt.Errorf("add SQLite verification token hash column: %w", err)
			}
		}
		if !migrator.HasIndex(session, "VerificationTokenHash") {
			if err := migrator.CreateIndex(session, "VerificationTokenHash"); err != nil {
				return fmt.Errorf("create SQLite verification token hash index: %w", err)
			}
		}
		return nil
	})
}
