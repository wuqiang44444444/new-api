package model

import (
	"slices"
	"sort"

	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

func contractChannelRoutes(filters []dto.ChannelFilter) map[int]string {
	for _, filter := range filters {
		if filter.Kind == dto.FilterContractRoutes {
			return filter.Routes
		}
	}
	return nil
}

// Candidate acquisition is the only extension to the native selector. Priority,
// retry tiers and weighted sampling stay in GetChannel/GetRandomSatisfiedChannel.
func channelRouteAbilityQuery(group, modelName string, filters []dto.ChannelFilter) *gorm.DB {
	query := DB.Where("model = ? AND enabled = ?", modelName, true)
	routes := contractChannelRoutes(filters)
	if routes == nil {
		return query.Where(map[string]any{"group": group})
	}
	allowed := DB.Where("1 = 0")
	for id, routeGroup := range routes {
		allowed = allowed.Or(map[string]any{"group": routeGroup, "channel_id": id})
	}
	return query.Where(allowed)
}

// Caller holds channelSyncLock. Each model/channel belongs to exactly one
// contract group, so a channel is never counted twice in weighted sampling.
func channelRouteCandidateIDs(group, modelName string, filters []dto.ChannelFilter) []int {
	routes := contractChannelRoutes(filters)
	if routes == nil {
		return group2model2channels[group][modelName]
	}
	ids := make([]int, 0, len(routes))
	for id, routeGroup := range routes {
		if slices.Contains(group2model2channels[routeGroup][modelName], id) {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}

// ValidateContractRoute shares the management/runtime eligibility boundary.
// Typed channels use their own group/model facts, never generic Ability rows.
func ValidateContractRoute(rule ContractEntityRule) error {
	return validateCustomerContractEntityChannel(DB, rule.ChannelId, rule.RouteGroup, rule.PublicModel)
}
