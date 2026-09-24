package model

import (
	"fmt"
	"slices"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// GetContractModelMetadata projects only already-authorized sources. It reuses
// native endpoint and media definitions without publishing internal identities.
func GetContractModelMetadata(rules []ContractEntityRule) (map[string]dto.OpenAIModels, error) {
	channels, err := GetContractSourceChannels(rules)
	if err != nil {
		return nil, err
	}
	result := make(map[string]dto.OpenAIModels)
	routes := make(map[string]map[int]string)
	names := []string{}
	seedance := make(map[string]bool)
	minimax := make(map[string]bool)
	for _, rule := range rules {
		channel := channels[rule.ChannelId]
		if channel.Type == constant.ChannelTypeAzureBatch {
			continue
		}
		if routes[rule.PublicModel] == nil {
			routes[rule.PublicModel] = make(map[int]string)
			names = append(names, rule.PublicModel)
		}
		routes[rule.PublicModel][rule.ChannelId] = rule.RouteGroup
		item := result[rule.PublicModel]
		endpoints := getPricingEndpointTypesForAbility(AbilityWithChannel{Ability: Ability{Model: rule.PublicModel, ChannelId: rule.ChannelId}, ChannelType: channel.Type}, map[int]*dto.AdvancedCustomConfig{channel.Id: channel.GetOtherSettings().AdvancedCustom})
		for _, endpoint := range endpoints {
			if !slices.Contains(item.SupportedEndpointTypes, endpoint) {
				item.SupportedEndpointTypes = append(item.SupportedEndpointTypes, endpoint)
			}
		}
		if channel.Type == constant.ChannelTypeSeedanceLink {
			seedance[rule.PublicModel] = true
		}
		if channel.Type == constant.ChannelTypeMiniMaxLink {
			minimax[rule.PublicModel] = true
		}
		result[rule.PublicModel] = item
	}
	apis, err := GetPublicMediaModelAPIs(names, nil, routes)
	if err != nil {
		return nil, err
	}
	for name, api := range apis {
		item := result[name]
		item.API = api
		result[name] = item
	}
	if len(seedance) > 0 {
		catalog, err := GetConfiguredSeedancePublicModels()
		if err != nil {
			return nil, err
		}
		for _, entry := range catalog {
			if !seedance[entry.ModelName] {
				continue
			}
			item := result[entry.ModelName]
			api := entry.API
			item.API = &api
			result[entry.ModelName] = item
		}
	}
	if len(minimax) > 0 {
		catalog, err := GetConfiguredMiniMaxPublicModels()
		if err != nil {
			return nil, err
		}
		for _, entry := range catalog {
			if !minimax[entry.ModelName] {
				continue
			}
			item := result[entry.ModelName]
			api := entry.API
			item.API = &api
			result[entry.ModelName] = item
		}
	}
	return result, nil
}

// GetContractSourceChannels reads projection metadata once per distinct source.
// A disappearing source fails the projection rather than publishing partial data.
func GetContractSourceChannels(rules []ContractEntityRule) (map[int]Channel, error) {
	result := make(map[int]Channel)
	ids := make([]int, 0, len(rules))
	seen := make(map[int]bool)
	for _, rule := range rules {
		if !seen[rule.ChannelId] {
			ids = append(ids, rule.ChannelId)
			seen[rule.ChannelId] = true
		}
	}
	for start := 0; start < len(ids); start += 200 {
		var channels []Channel
		batch := ids[start:min(start+200, len(ids))]
		if err := DB.Select("id", "type", "settings").Where("id IN ?", batch).Find(&channels).Error; err != nil {
			return nil, err
		}
		for _, channel := range channels {
			result[channel.Id] = channel
		}
	}
	if len(result) != len(ids) {
		return nil, fmt.Errorf("contract source channel is unavailable")
	}
	return result, nil
}
