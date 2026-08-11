package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// GetEnabledSeedanceChannel resolves the management-approved Seedance channel
// without publishing it into NEWAPI's native Ability distribution pool.
// Save/enable validation guarantees one customer model maps to one enabled
// Seedance channel, so this request path intentionally performs no candidate
// weighting, retry selection, or duplicate repair.
func GetEnabledSeedanceChannel(group, customerModel string, channelID int) (*Channel, error) {
	group = strings.TrimSpace(group)
	customerModel = strings.TrimSpace(customerModel)
	if group == "" || customerModel == "" {
		return nil, nil
	}
	var channels []Channel
	query := DB.Where("type = ? AND status = ?", constant.ChannelTypeSeedanceLink, common.ChannelStatusEnabled)
	query = ApplyChannelGroupFilter(query, group)
	if channelID > 0 {
		query = query.Where("channels.id = ?", channelID)
	}
	if err := query.Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	for i := range channels {
		if channelContainsModel(&channels[i], customerModel) {
			return &channels[i], nil
		}
	}
	return nil, nil
}

func channelContainsModel(channel *Channel, customerModel string) bool {
	if channel == nil {
		return false
	}
	for _, modelName := range channel.GetModels() {
		if strings.TrimSpace(modelName) == customerModel {
			return true
		}
	}
	return false
}

func enabledSeedanceChannels(tx *gorm.DB) ([]Channel, error) {
	if tx == nil {
		tx = DB
	}
	var channels []Channel
	err := tx.Where("type = ? AND status = ?", constant.ChannelTypeSeedanceLink, common.ChannelStatusEnabled).
		Order("id").Find(&channels).Error
	return channels, err
}

func enabledSeedanceAbilityViews(tx *gorm.DB) ([]AbilityWithChannel, error) {
	channels, err := enabledSeedanceChannels(tx)
	if err != nil {
		return nil, err
	}
	abilities := make([]AbilityWithChannel, 0)
	for i := range channels {
		for _, group := range channels[i].GetGroups() {
			for _, modelName := range channels[i].GetModels() {
				modelName = strings.TrimSpace(modelName)
				if group == "" || modelName == "" {
					continue
				}
				abilities = append(abilities, AbilityWithChannel{
					Ability:     Ability{Group: group, Model: modelName, ChannelId: channels[i].Id, Enabled: true},
					ChannelType: constant.ChannelTypeSeedanceLink,
				})
			}
		}
	}
	return abilities, nil
}

func appendEnabledSeedanceModels(models []string, group string) []string {
	channels, err := enabledSeedanceChannels(DB)
	if err != nil {
		return models
	}
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		seen[modelName] = struct{}{}
	}
	for i := range channels {
		if group != "" && !common.StringsContains(channels[i].GetGroups(), group) {
			continue
		}
		for _, modelName := range channels[i].GetModels() {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			if _, exists := seen[modelName]; exists {
				continue
			}
			seen[modelName] = struct{}{}
			models = append(models, modelName)
		}
	}
	return models
}
