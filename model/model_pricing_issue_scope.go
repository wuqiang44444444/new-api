package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
)

// servedTaskPluginPricingModels attributes a declared model to a plugin only
// when a native ability reaches a channel that can run that plugin.
func servedTaskPluginPricingModels(generation *jsplugin.RoutingGeneration, pluginKey string) (map[string]map[string]bool, error) {
	filters := make(map[string]dto.ChannelFilter)
	for _, plugin := range generation.Plugins() {
		key := plugin.Meta.Key
		if !plugin.Meta.HasTaskPart() || (pluginKey != "" && key != pluginKey) {
			continue
		}
		filters[key] = dto.ChannelFilter{
			Kind:                   dto.FilterTaskPluginIdentity,
			TaskPluginKey:          key,
			TaskPluginChannelTypes: plugin.Meta.ChannelTypes,
		}
	}
	served := make(map[string]map[string]bool, len(filters))
	if len(filters) == 0 {
		return served, nil
	}

	var routes []struct {
		Model string
		Channel
	}
	err := DB.Table("abilities").
		Select("DISTINCT abilities.model, channels.id, channels.type, channels.status, channels.setting").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities.enabled = ? AND channels.status = ?", true, common.ChannelStatusEnabled).
		Scan(&routes).Error
	if err != nil {
		return nil, err
	}
	for i := range routes {
		route := &routes[i]
		if channelSkipsGenericAbilities(route.Type) {
			continue
		}
		if (route.Type == constant.ChannelTypeTaskPlugin || route.Type == constant.ChannelTypeNewAPI) &&
			route.Setting != nil && *route.Setting != "" {
			// GetSetting repairs malformed persisted JSON. Keep this admin read
			// side-effect free by rejecting that input before calling the filter.
			var settings relaydto.ChannelSettings
			if err := common.UnmarshalJsonStr(*route.Setting, &settings); err != nil {
				continue
			}
		}
		model, canonical := generation.CanonicalModel(route.Model)
		if !canonical {
			target, found := ResolveTaskModelAlias(generation, route.Model)
			if !found || target.Declared == "" {
				continue
			}
			model = target.Declared
		}
		for _, plugin := range generation.PluginsByModel(model) {
			key := plugin.Meta.Key
			filter, eligible := filters[key]
			if !eligible || served[key][model] ||
				!channelMatchesFilter(&route.Channel, route.Model, filter) {
				continue
			}
			if served[key] == nil {
				served[key] = make(map[string]bool)
			}
			served[key][model] = true
		}
	}
	return served, nil
}
