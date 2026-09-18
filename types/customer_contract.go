package types

import "github.com/shopspring/decimal"

const CustomerContractRatioScale int64 = 100_000_000

// ContractBillingFact is the immutable customer discount fact resolved by
// exact public model before billing. It carries only discount facts: the
// actual channel, group and native ratios always come from the real execution
// context after scope validation. ContractId identifies the
// owning contract entity (0 for facts frozen before contract entities
// existed). ContractName is the customer-readable name frozen at resolution
// time so later renames cannot backfill history.
type ContractBillingFact struct {
	UserId          int    `json:"user_id"`
	ContractId      int    `json:"contract_id,omitempty"`
	ContractVersion int64  `json:"contract_version"`
	ContractName    string `json:"contract_name,omitempty"`
	PublicModel     string `json:"public_model"`
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
