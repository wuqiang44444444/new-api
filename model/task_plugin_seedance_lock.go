package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// SeedancePluginInUseError carries only administrator-visible references.
type SeedancePluginInUseError struct {
	Tasks    []SeedancePluginTaskRef
	Attempts []SeedancePluginAttemptRef
}

func (*SeedancePluginInUseError) Error() string { return "task plugin is still in use" }

// Both admission and deletion take this lock before reading dependencies or
// creating a prepared attempt. No lock spans HTTP, JS execution, or funds hold.
func lockSeedancePluginVersion(tx *gorm.DB, key, version string) error {
	if version == "" {
		return errors.New("seedance plugin version is required")
	}
	identity := TaskPlugin{Key: key, Version: version}
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		// SQLite has no row locks. Acquire its write reservation before any read,
		// so a deferred transaction cannot validate against a stale read snapshot.
		if err := tx.Model(&TaskPlugin{}).Where(&identity).
			UpdateColumn("source_hash", gorm.Expr("source_hash")).Error; err != nil {
			return err
		}
	}
	var plugin TaskPlugin
	return lockForUpdate(tx).Select("id").Where(&identity).First(&plugin).Error
}

func lockSeedancePluginAttempt(tx *gorm.DB, pin *TaskPluginSnapshot, frozen []byte) error {
	if pin == nil || !SeedancePluginExecutionUsageApplies(pin.Key) {
		return nil
	}
	var persisted seedanceFrozenConnectionPin
	if err := common.Unmarshal(frozen, &persisted); err != nil {
		return err
	}
	if persisted.PluginKey != pin.Key || persisted.PluginVersion != pin.Version {
		return errors.New("seedance prepared plugin identity does not match its frozen reference")
	}
	return lockSeedancePluginVersion(tx, pin.Key, pin.Version)
}

func guardSeedancePluginDeletion(tx *gorm.DB, key, version string) error {
	if !SeedancePluginExecutionUsageApplies(key) {
		return nil
	}
	if err := lockSeedancePluginVersion(tx, key, version); err != nil {
		return err
	}
	tasks, attempts, err := seedancePluginExecutionUsage(tx, key, version)
	if err != nil {
		return err
	}
	if len(tasks) != 0 || len(attempts) != 0 {
		return &SeedancePluginInUseError{Tasks: tasks, Attempts: attempts}
	}
	return nil
}
