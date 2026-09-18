package service

import "github.com/QuantumNous/new-api/model"

// Keep every agreed model visible, including disabled or unusable sources.
// Customer-facing status carries no channel/provider/group identifiers.
func BuildContractEntityUserScopeViews(snapshots []model.ContractEntitySnapshot, userGroup string) ([]ContractEntityUserView, error) {
	views, err := BuildContractEntityUserViews(snapshots)
	if err != nil {
		return nil, err
	}
	groups := GetUserUsableGroups(userGroup)
	for i, snapshot := range snapshots {
		available := make(map[string]bool)
		permitted := make(map[string]bool)
		if snapshot.Enabled {
			rules, err := EffectiveContractRules(&snapshot, userGroup)
			if err != nil {
				return nil, err
			}
			for _, rule := range rules {
				available[rule.PublicModel] = true
			}
			for _, rule := range snapshot.Rules {
				if _, ok := groups[rule.RouteGroup]; ok {
					permitted[rule.PublicModel] = true
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
			case !permitted[row.Model]:
				row.Availability = "group_denied"
			default:
				row.Availability = "unavailable"
			}
		}
	}
	return views, nil
}
