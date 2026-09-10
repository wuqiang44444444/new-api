package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// GetSeedanceChannelsForBillingValidation uses the active contract when one
// exists. Otherwise every configured inactive contract must accept the price;
// disabling a channel does not turn its customer model into a native plugin.
func GetSeedanceChannelsForBillingValidation(modelName string) ([]Channel, error) {
	return getSeedanceChannelsForBillingValidation(DB, modelName)
}

func getSeedanceChannelsForBillingValidation(db *gorm.DB, modelName string) ([]Channel, error) {
	var channels []Channel
	if err := db.Where("type = ?", constant.ChannelTypeSeedanceLink).Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	var enabled, disabled []Channel
	for _, channel := range channels {
		if !channelContainsModel(&channel, modelName) {
			continue
		}
		if channel.Status == common.ChannelStatusEnabled {
			enabled = append(enabled, channel)
		} else {
			disabled = append(disabled, channel)
		}
	}
	if len(enabled)+len(disabled) > 0 {
		nativeModels, err := oppositeSeedancePricingModels(db, constant.ChannelTypeSeedanceLink, 0)
		if err != nil {
			return nil, err
		}
		if nativeModels[modelName] {
			return nil, seedancePricingOwnershipError(modelName)
		}
	}
	if len(enabled) > 0 {
		return enabled, nil
	}
	return disabled, nil
}
