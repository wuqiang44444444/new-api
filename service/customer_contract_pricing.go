package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const CustomerContractPublicPricingGroup = "contract"

type CustomerContractPricingView struct {
	model.Pricing
	GroupRatio map[string]float64 `json:"group_ratio"`
}

func BuildCustomerContractPricing(snapshot *model.CustomerContractSnapshot, userGroup string) ([]CustomerContractPricingView, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("contract snapshot is nil")
	}
	adminRules, err := BuildCustomerContractAdminRules(snapshot, userGroup)
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
		item.VendorID = 0
		item.EnableGroup = []string{CustomerContractPublicPricingGroup}
		item.Available = rule.Available
		if rule.Available {
			item.Availability = "available"
		} else {
			item.Availability = "restricted"
		}
		result = append(result, CustomerContractPricingView{
			Pricing: item,
			GroupRatio: map[string]float64{
				CustomerContractPublicPricingGroup: effective.InexactFloat64(),
			},
		})
	}
	return result, nil
}
