package model

import (
	"slices"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// GetContractModelMetadata projects only already-authorized sources. It reuses
// native endpoint and media definitions without publishing internal identities.
func GetContractModelMetadata(rules []ContractEntityRule) (map[string]dto.OpenAIModels, error) {
	result := make(map[string]dto.OpenAIModels)
	routes := make(map[string]map[int]string)
	names := []string{}
	seedance := make(map[string]bool)
	for _, rule := range rules {
		channel, err := GetChannelById(rule.ChannelId, false)
		if err != nil {
			return nil, err
		}
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
	return result, nil
}
