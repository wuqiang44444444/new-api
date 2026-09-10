package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const CustomerContractPublicPricingGroup = "contract"

type CustomerContractPricingView struct {
	model.Pricing
	GroupRatio    map[string]float64 `json:"group_ratio"`
	ExecutionMode string             `json:"execution_mode,omitempty"`
}

// BuildContractEntityPricing builds the public pricing projection of one
// contract entity. Each rule becomes exactly one entry; availability comes
// from the snapshot's already refreshed rules.
func BuildContractEntityPricing(snapshot *model.ContractEntitySnapshot, userGroup string) ([]CustomerContractPricingView, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("contract snapshot is nil")
	}
	adminRules, err := buildContractEntityRuleViews(snapshot, userGroup)
	if err != nil {
		return nil, err
	}
	pricing := customerContractPricingIndex()
	result := make([]CustomerContractPricingView, 0, len(adminRules))
	for _, rule := range adminRules {
		item, exists := pricing[rule.Model]
		if !exists {
			item = model.Pricing{ModelName: rule.Model}
		}
		effective, err := decimal.NewFromString(rule.EffectiveMultiplier)
		if err != nil {
			return nil, err
		}
		item.OwnerBy = "new-api"
		executionMode := ""
		if rule.Price.BillingMode == "batch_expr" {
			// A same-named synchronous model must not supply Batch prices.
			item = model.Pricing{ModelName: rule.Model, OwnerBy: "new-api", BillingMode: "tiered_expr", BillingExpr: rule.Price.BillingExpr}
			executionMode = "batch"
		}
		item.VendorID = 0
		item.EnableGroup = []string{CustomerContractPublicPricingGroup}
		item.Available = rule.Available
		if rule.Available {
			item.Availability = "available"
		} else {
			item.Availability = "restricted"
		}
		result = append(result, CustomerContractPricingView{
			Pricing:       item,
			ExecutionMode: executionMode,
			GroupRatio: map[string]float64{
				CustomerContractPublicPricingGroup: effective.InexactFloat64(),
			},
		})
	}
	return result, nil
}
