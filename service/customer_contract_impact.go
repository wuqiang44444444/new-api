package service

import (
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

type CustomerContractRatioImpact struct {
	AffectedContracts int      `json:"affected_contracts"`
	AffectedRules     int      `json:"affected_rules"`
	AffectedGroups    []string `json:"affected_groups"`
}

func PreviewCustomerContractRatioImpact(groupRatioJSON string, groupGroupRatioJSON string) (*CustomerContractRatioImpact, error) {
	var nextGroupRatios map[string]float64
	if err := common.UnmarshalJsonStr(groupRatioJSON, &nextGroupRatios); err != nil {
		return nil, fmt.Errorf("invalid group ratios: %w", err)
	}
	var nextSpecialRatios map[string]map[string]float64
	if err := common.UnmarshalJsonStr(groupGroupRatioJSON, &nextSpecialRatios); err != nil {
		return nil, fmt.Errorf("invalid inter-group ratios: %w", err)
	}
	rules, err := model.ListActiveCustomerContractRules()
	if err != nil {
		return nil, err
	}
	users := make(map[int]struct{})
	groups := make(map[string]struct{})
	affectedRules := 0
	for _, rule := range rules {
		currentRatio, _ := ResolveCustomerContractNativeGroupRatio(rule.UserGroup, rule.RouteGroup)
		nextRatio, exists := nextGroupRatios[rule.RouteGroup]
		if !exists {
			nextRatio = 1
		}
		if specialByTarget, ok := nextSpecialRatios[rule.UserGroup]; ok {
			if special, exists := specialByTarget[rule.RouteGroup]; exists {
				nextRatio = special
			}
		}
		if decimal.NewFromFloat(currentRatio).Equal(decimal.NewFromFloat(nextRatio)) {
			continue
		}
		affectedRules++
		users[rule.UserId] = struct{}{}
		groups[rule.RouteGroup] = struct{}{}
	}
	affectedGroups := make([]string, 0, len(groups))
	for group := range groups {
		affectedGroups = append(affectedGroups, group)
	}
	sort.Strings(affectedGroups)
	return &CustomerContractRatioImpact{
		AffectedContracts: len(users), AffectedRules: affectedRules, AffectedGroups: affectedGroups,
	}, nil
}
