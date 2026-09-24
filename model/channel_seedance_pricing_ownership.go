package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// Prices are keyed by customer model, so Link and native contracts cannot
// share a key even while a channel is disabled. Provider mappings are irrelevant.
func oppositeSeedancePricingModels(tx *gorm.DB, channelType, excludeID int) (map[string]bool, error) {
	if tx == nil {
		tx = DB
	}
	query := tx.Model(&Channel{}).Select("models").Where("id <> ?", excludeID)
	if channelType == constant.ChannelTypeSeedanceLink || channelType == constant.ChannelTypeMiniMaxLink {
		query = query.Where("type NOT IN ?", typedStandardVideoChannelTypes)
	} else {
		query = query.Where("type IN ?", typedStandardVideoChannelTypes)
	}
	var channels []Channel
	if err := query.Find(&channels).Error; err != nil {
		return nil, err
	}
	models := make(map[string]bool)
	for _, channel := range channels {
		for _, name := range channel.GetModels() {
			if name = strings.TrimSpace(name); name != "" {
				models[name] = true
			}
		}
	}
	return models, nil
}

func seedancePricingOwnershipError(modelName string) error {
	return fmt.Errorf("model %q is configured in both Seedance Link and native channels; use distinct customer model names for their pricing contracts", modelName)
}

func validateSeedancePricingOwnership(tx *gorm.DB, channel *Channel) error {
	otherModels, err := oppositeSeedancePricingModels(tx, channel.Type, channel.Id)
	if err != nil {
		return err
	}
	for _, name := range channel.GetModels() {
		name = strings.TrimSpace(name)
		if otherModels[name] {
			return seedancePricingOwnershipError(name)
		}
	}
	return nil
}

// ValidateSeedancePricingModelNames protects option writes, including raw JSON
// edits that do not use the pricing sheet. Removing a price key remains possible.
func ValidateSeedancePricingModelNames(modelNames []string) error {
	if len(modelNames) == 0 {
		return nil
	}
	linkModels, err := oppositeSeedancePricingModels(DB, constant.ChannelTypeDoubaoVideo, 0)
	if err != nil || len(linkModels) == 0 {
		return err
	}
	nativeModels, err := oppositeSeedancePricingModels(DB, constant.ChannelTypeSeedanceLink, 0)
	if err != nil {
		return err
	}
	for _, name := range modelNames {
		if linkModels[name] && nativeModels[name] {
			return seedancePricingOwnershipError(name)
		}
	}
	return nil
}
