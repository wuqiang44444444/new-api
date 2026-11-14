package model

import (
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ImportLegacyEvidenceRetention runs on startup before any evidence worker.
// Nodes sharing the main database agree on the persisted policy.
func ImportLegacyEvidenceRetention() error {
	const key = system_setting.TaskRequestEvidenceRetentionOptionKey
	raw := strings.TrimSpace(os.Getenv("TASK_REQUEST_EVIDENCE_RETENTION_DAYS"))
	days, err := strconv.Atoi(raw)
	var saved Option
	if err != nil || days < 1 || days > 7 {
		// Load the authoritative value even if the initial bulk option load
		// failed. A database error must never start cleanup with defaults.
		if err := DB.Where(&Option{Key: key}).Find(&saved).Error; err != nil {
			return err
		}
		if _, err := system_setting.ParseTaskRequestEvidenceRetentionDays(saved.Value); err == nil {
			return updateOptionMap(key, saved.Value)
		}
		return nil
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&Option{Key: key, Value: strconv.Itoa(days)}).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where(&Option{Key: key}).Take(&saved).Error; err != nil {
			return err
		}
		if _, err := system_setting.ParseTaskRequestEvidenceRetentionDays(saved.Value); err == nil {
			return nil
		}
		saved.Value = strconv.Itoa(days)
		return tx.Model(&Option{}).Where(&Option{Key: key}).Update("value", saved.Value).Error
	})
	if err != nil {
		return err
	}
	return updateOptionMap(key, saved.Value)
}
