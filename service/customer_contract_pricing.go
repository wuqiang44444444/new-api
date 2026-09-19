package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

// CustomerContractPricingView is one native pricing entry with an optional
// contract discount overlay. GroupRatio, when set, carries the per-group
// effective ratio (native group ratio × contract discount) for contracted
// models. Uncontracted models must not enter the projection.
type CustomerContractPricingView struct {
	model.Pricing
	GroupRatio       map[string]float64 `json:"group_ratio,omitempty"`
	ContractDiscount string             `json:"contract_discount,omitempty"`
}

// ContractDiscountsFromSnapshot reduces one contract snapshot to an
// exact-model → ratio_units map with one entry per public model. The save
// boundary enforces one identical discount per model; any residual mismatch
// is a hard error.
func ContractDiscountsFromSnapshot(snapshot *model.ContractEntitySnapshot) (map[string]int64, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("%w: contract snapshot is nil", ErrCustomerContractUnavailable)
	}
	discounts := make(map[string]int64, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		if rule.RatioUnits <= 0 || rule.RatioUnits > hosttypes.CustomerContractRatioScale {
			return nil, fmt.Errorf("%w: invalid discount for model %q", ErrCustomerContractUnavailable, rule.PublicModel)
		}
		units, exists := discounts[rule.PublicModel]
		if exists && units != rule.RatioUnits {
			return nil, fmt.Errorf("%w: inconsistent discount for model %q", ErrCustomerContractUnavailable, rule.PublicModel)
		}
		discounts[rule.PublicModel] = rule.RatioUnits
	}
	return discounts, nil
}

// ApplyContractDiscountOverlay marks every contracted model's pricing entry
// with the contract discount and a per-group effective ratio (native group
// ratio × contract discount). Uncontracted models are rejected.
func ApplyContractDiscountOverlay(pricing []model.Pricing, userGroup string, snapshot *model.ContractEntitySnapshot) ([]CustomerContractPricingView, error) {
	discounts, err := ContractDiscountsFromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	result := make([]CustomerContractPricingView, 0, len(pricing))
	for _, item := range pricing {
		units, contracted := discounts[item.ModelName]
		if !contracted {
			return nil, ErrCustomerContractScope
		}
		discount := decimal.NewFromInt(units).Div(decimal.NewFromInt(hosttypes.CustomerContractRatioScale))
		view := CustomerContractPricingView{Pricing: item, ContractDiscount: discount.String()}
		perGroup := make(map[string]float64)
		for _, group := range item.EnableGroup {
			nativeRatio, _ := ResolveCustomerContractNativeGroupRatio(userGroup, group)
			perGroup[group] = decimal.NewFromFloat(nativeRatio).Mul(discount).InexactFloat64()
		}
		view.GroupRatio = perGroup
		result = append(result, view)
	}
	return result, nil
}
