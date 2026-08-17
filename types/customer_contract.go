package types

import "github.com/shopspring/decimal"

const CustomerContractRatioScale int64 = 100_000_000

// ContractBillingFact is the immutable customer-price and route fact captured
// before channel selection and billing. It contains no provider identity.
type ContractBillingFact struct {
	UserId          int    `json:"user_id"`
	ContractVersion int64  `json:"contract_version"`
	PublicModel     string `json:"public_model"`
	RouteGroup      string `json:"route_group"`
	RatioUnits      int64  `json:"ratio_units"`
}

func (f *ContractBillingFact) RatioDecimal() decimal.Decimal {
	if f == nil || f.RatioUnits <= 0 || f.RatioUnits > CustomerContractRatioScale {
		return decimal.Zero
	}
	return decimal.NewFromInt(f.RatioUnits).Div(decimal.NewFromInt(CustomerContractRatioScale))
}

func (f *ContractBillingFact) RatioString() string {
	return f.RatioDecimal().String()
}
