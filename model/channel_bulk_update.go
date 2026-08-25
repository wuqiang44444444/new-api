package model

import (
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// updateChannelsAndAbilitiesByTag keeps channel changes and Ability rows in
// one transaction for bulk model, mapping, and group edits.
func updateChannelsAndAbilitiesByTag(tag, updatedTag string, updateData Channel, actorID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Channel{}).Where("tag = ?", tag).Updates(updateData).Error; err != nil {
			return err
		}
		var channels []Channel
		if err := tx.Where("tag = ?", updatedTag).Find(&channels).Error; err != nil {
			return err
		}
		for i := range channels {
			if channels[i].Type == constant.ChannelTypeSeedanceLink {
				settings, err := parsedChannelOtherSettings(&channels[i])
				if err != nil {
					return err
				}
				if err := validateSeedanceChannelSettingsTx(tx, &channels[i], &settings); err != nil {
					return err
				}
			}
			if err := channels[i].UpdateAbilitiesWithActor(tx, actorID); err != nil {
				return err
			}
		}
		return nil
	})
}
