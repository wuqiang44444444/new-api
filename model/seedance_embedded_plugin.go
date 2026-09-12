package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/hashicorp/go-version"
	"gorm.io/gorm"
)

// PromoteEmbeddedSeedancePlugin advances the bundled artifact using the existing
// publication lock and Channel validation. Old nodes cannot undo a newer
// deployment. Exact-version history is neither rewritten nor deleted.
func PromoteEmbeddedSeedancePlugin(embeddedVersion, sourceHash string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockSeedancePluginConfiguration(tx, jsplugin.SeedancePluginKey); err != nil {
			return err
		}
		var target TaskPlugin
		if err := tx.Where(&TaskPlugin{Key: jsplugin.SeedancePluginKey, Version: embeddedVersion}).First(&target).Error; err != nil {
			return err
		}
		if target.SourceHash != sourceHash {
			return errors.New("embedded Seedance version conflicts with its stored artifact; publish a new version")
		}
		var current TaskPlugin
		err := tx.Where(&TaskPlugin{Key: jsplugin.SeedancePluginKey, Active: true}).First(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			currentVersion, currentErr := version.NewVersion(current.Version)
			nextVersion, nextErr := version.NewVersion(target.Version)
			if currentErr != nil || nextErr != nil {
				return errors.New("cannot compare Seedance deployment versions")
			}
			if !nextVersion.GreaterThan(currentVersion) {
				return nil // Preserve an administrator's explicit disable switch too.
			}
		}
		if err := validateSeedancePluginConfigurationActivation(tx, &target); err != nil {
			return fmt.Errorf("cannot activate embedded Seedance version: %w", err)
		}
		if err := tx.Model(&TaskPlugin{}).Where(&TaskPlugin{Key: target.Key}).Update("active", false).Error; err != nil {
			return err
		}
		return tx.Model(&target).Updates(map[string]any{"active": true, "enabled": true}).Error
	})
}
