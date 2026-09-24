package model

import (
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
	if err := db.Where("type IN ?", typedStandardVideoChannelTypes).Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	selected := SeedancePricingChannels(channels)[modelName]
	if len(selected) > 0 {
		nativeModels, err := oppositeSeedancePricingModels(db, constant.ChannelTypeSeedanceLink, 0)
		if err != nil {
			return nil, err
		}
		if nativeModels[modelName] {
			return nil, seedancePricingOwnershipError(modelName)
		}
	}
	return selected, nil
}
