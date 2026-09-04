package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const (
	// seedanceTypeRenumberMigrationKey guards the one-shot renumber that moved
	// the local Seedance Link and async image channel types above the upstream
	// Task Plugin type introduced in v1.0.0-rc.31 (61).
	seedanceTypeRenumberMigrationKey = "migration.seedance_asyncimage_type_renumber_v1"

	// legacyMoxingChannelTypeAfterRenumber is the retired slot for pre-renumber
	// Moxing image channels. It sits past ChannelTypeDummy so it can never be
	// allocated to a real channel type again.
	legacyMoxingChannelTypeAfterRenumber = 65
)

// migrateSeedanceChannelTypeRenumber renumbers persisted channel rows from the
// pre-rc.31 local numbering (61 Seedance Link, 62 async image, 63 retired
// Moxing) onto the post-rc.31 numbering (62, 63, 65). Updates run in
// descending order so each old value is vacated before the next step reuses
// it. Upstream Task Plugin channels (61) cannot exist in a database that has
// not run this migration, because that type only ships with the runtime that
// also runs this migration before serving traffic.
func migrateSeedanceChannelTypeRenumber() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var marker Option
		err := tx.Where(&Option{Key: seedanceTypeRenumberMigrationKey}).First(&marker).Error
		if err == nil && marker.Value == "done" {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		steps := []struct {
			from int
			to   int
		}{
			{from: 63, to: legacyMoxingChannelTypeAfterRenumber},
			{from: 62, to: 63},
			{from: 61, to: 62},
		}
		for _, step := range steps {
			result := tx.Model(&Channel{}).Where("type = ?", step.from).Update("type", step.to)
			if result.Error != nil {
				return fmt.Errorf("channel type renumber %d -> %d: %w", step.from, step.to, result.Error)
			}
		}
		return tx.Save(&Option{Key: seedanceTypeRenumberMigrationKey, Value: "done"}).Error
	})
}

// verifySeedanceChannelTypeRenumberState is a read-only startup gate: no node
// may serve traffic before the master has finished the channel type renumber,
// because the old and new numberings interpret type 61/62 differently.
func verifySeedanceChannelTypeRenumberState() error {
	var marker Option
	if err := DB.Where(&Option{Key: seedanceTypeRenumberMigrationKey}).First(&marker).Error; err != nil {
		return fmt.Errorf("channel type renumber migration is not complete: %w", err)
	}
	if marker.Value != "done" {
		return fmt.Errorf("channel type renumber migration is not complete")
	}
	return nil
}

// verifyLinkChannelMigrationState aggregates the local channel migration
// startup gates so request wiring stays a single narrow call.
func verifyLinkChannelMigrationState() error {
	if err := verifySeedanceChannelTypeRenumberState(); err != nil {
		return err
	}
	return verifyImageRelayMigrationState()
}
