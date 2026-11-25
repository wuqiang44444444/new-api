package model

import (
	"context"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
)

// NativeArkSessionChannel recognizes the provider version supported by the
// Session meter from an existing model binding on a direct Ark channel.
func NativeArkSessionChannel(channel *Channel, publicModel string) bool {
	if channel == nil || channel.Status != common.ChannelStatusEnabled ||
		!slices.Contains(channel.GetModels(), publicModel) || !arkCacheTransportCompatible(channel) {
		return false
	}
	upstream := publicModel
	if raw := channel.GetModelMapping(); raw != "" {
		var mappings map[string]string
		if common.UnmarshalJsonStr(raw, &mappings) != nil {
			return false
		}
		if mapped := mappings[publicModel]; mapped != "" {
			upstream = mapped
		}
	}
	return upstream == setting.ArkPrefixCacheModelVersion
}

// NativeArkSessionRoutes narrows the existing model abilities to channels the
// Session transport can use. The caller passes these to the existing selector.
func NativeArkSessionRoutes(ctx context.Context, publicModel, group string, contractRoutes map[int]string) (map[int]string, error) {
	tx := DB.WithContext(ctx)
	var abilities []Ability
	query := tx.Model(&Ability{}).Where("model = ? AND enabled = ?", publicModel, true)
	if contractRoutes == nil {
		query = query.Where(map[string]any{"group": group})
	} else {
		ids := make([]int, 0, len(contractRoutes))
		for id := range contractRoutes {
			ids = append(ids, id)
		}
		query = query.Where("channel_id IN ?", ids)
	}
	if err := query.Find(&abilities).Error; err != nil || len(abilities) == 0 {
		return nil, err
	}
	ids := make([]int, 0, len(abilities))
	groups := make(map[int]string, len(abilities))
	for _, ability := range abilities {
		if contractRoutes != nil && contractRoutes[ability.ChannelId] != ability.Group {
			continue
		}
		ids = append(ids, ability.ChannelId)
		groups[ability.ChannelId] = ability.Group
	}
	var channels []Channel
	if err := tx.Where("id IN ? AND type = ? AND status = ?", ids, constant.ChannelTypeVolcEngine, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	routes := make(map[int]string, len(channels))
	for _, channel := range channels {
		if NativeArkSessionChannel(&channel, publicModel) && slices.Contains(channel.GetGroups(), groups[channel.Id]) {
			routes[channel.Id] = groups[channel.Id]
		}
	}
	return routes, nil
}
