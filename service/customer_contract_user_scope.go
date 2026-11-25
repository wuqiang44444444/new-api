package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Keep every agreed model visible, including disabled or unusable sources.
// Customer-facing status carries no channel/provider/group identifiers.
func BuildContractEntityUserScopeViews(snapshots []model.ContractEntitySnapshot) ([]ContractEntityUserView, error) {
	views, err := BuildContractEntityUserViews(snapshots)
	if err != nil {
		return nil, err
	}
	pilot := ArkCachePilot()
	var pilotContractID int
	if pilot != nil && pilot.Mode == "observe" && len(snapshots) != 0 && snapshots[0].UserId == pilot.UserID {
		var tokens []model.Token
		if err := model.DB.Select("contract_id").Where("id = ? AND user_id = ? AND status = ? AND (expired_time = -1 OR expired_time > ?)",
			pilot.TokenID, pilot.UserID, common.TokenStatusEnabled, common.GetTimestamp()).Limit(1).Find(&tokens).Error; err != nil {
			return nil, err
		}
		if len(tokens) == 1 {
			pilotContractID = tokens[0].ContractId
		}
	}
	for i, snapshot := range snapshots {
		available := make(map[string]bool)
		pilotBound := pilot != nil && pilotContractID > 0 && snapshot.Id == pilotContractID && snapshot.UserId == pilot.UserID
		if snapshot.Enabled {
			rules, err := effectiveContractRulesDetailed(&snapshot)
			if err != nil {
				return nil, err
			}
			for _, rule := range rules {
				if rule.Available && (!rule.ArkCacheOnly || pilotBound) {
					available[rule.PublicModel] = true
				}
			}
		}
		for j := range views[i].Models {
			row := &views[i].Models[j]
			switch {
			case !snapshot.Enabled:
				row.Availability = "disabled"
			case available[row.Model]:
				row.Availability = "available"
			default:
				row.Availability = "unavailable"
			}
		}
	}
	return views, nil
}
