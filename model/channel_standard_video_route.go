package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// MiniMaxLinkTaskPlatform is the numeric task platform identity of MiniMax
// Link tasks (the channel type as a string, shared with the typed video
// task conventions).
func MiniMaxLinkTaskPlatform() string {
	return strconv.Itoa(constant.ChannelTypeMiniMaxLink)
}

// GetEnabledMiniMaxChannel resolves the management-approved MiniMax Link
// channel without publishing it into NEWAPI's native Ability distribution
// pool. Save/enable validation guarantees one customer model maps to one
// enabled typed channel, so this request path intentionally performs no
// candidate weighting, retry selection, or duplicate repair.
func GetEnabledMiniMaxChannel(group, customerModel string, channelID int) (*Channel, error) {
	group = strings.TrimSpace(group)
	customerModel = strings.TrimSpace(customerModel)
	if group == "" || customerModel == "" {
		return nil, nil
	}
	var channels []Channel
	query := DB.Where("type = ? AND status = ?", constant.ChannelTypeMiniMaxLink, common.ChannelStatusEnabled)
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

// MiniMaxLinkChannelServesModel is the cheap existence probe used by the
// standard-entry resolver to route between the typed channels. It is
// deliberately group-free: the request group may be "auto" or contract-
// resolved only inside the resolver, and cross-type uniqueness at save/enable
// guarantees that an enabled MiniMax channel owning the model cannot coexist
// with an enabled Seedance channel for it. Group/token authorization still
// runs inside the resolver, which produces the same model_not_found or
// authorization errors the Seedance path would.
func MiniMaxLinkChannelServesModel(customerModel string) bool {
	customerModel = strings.TrimSpace(customerModel)
	if customerModel == "" {
		return false
	}
	var channels []Channel
	if err := DB.Select("id", "models").
		Where("type = ? AND status = ?", constant.ChannelTypeMiniMaxLink, common.ChannelStatusEnabled).
		Order("id").Find(&channels).Error; err != nil {
		return false
	}
	for i := range channels {
		if channelContainsModel(&channels[i], customerModel) {
			return true
		}
	}
	return false
}

// enabledMiniMaxAbilityViews mirrors enabledSeedanceAbilityViews for the
// pricing/projection ability reads; MiniMax Link rows never enter the native
// abilities table.
func enabledMiniMaxAbilityViews(tx *gorm.DB) ([]AbilityWithChannel, error) {
	var channels []Channel
	err := tx.Where("type = ? AND status = ?", constant.ChannelTypeMiniMaxLink, common.ChannelStatusEnabled).
		Order("id").Find(&channels).Error
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
					ChannelType: constant.ChannelTypeMiniMaxLink,
				})
			}
		}
	}
	return abilities, nil
}

// appendEnabledMiniMaxModels feeds group-enabled model lists the same way
// the Seedance route helper does; MiniMax Link models stay out of the native
// Ability pool and only join the standard video projections.
func appendEnabledMiniMaxModels(models []string, group string) []string {
	var channels []Channel
	query := DB.Where("type = ? AND status = ?", constant.ChannelTypeMiniMaxLink, common.ChannelStatusEnabled).Order("id")
	if err := query.Find(&channels).Error; err != nil {
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
