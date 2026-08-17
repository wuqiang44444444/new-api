package helper

import (
	"fmt"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

func customerContractRatio(info *relaycommon.RelayInfo) (decimal.Decimal, error) {
	if info == nil || info.ContractBillingFact == nil {
		return decimal.NewFromInt(1), nil
	}
	ratio := info.ContractBillingFact.RatioDecimal()
	if ratio.IsZero() {
		return decimal.Zero, fmt.Errorf("invalid frozen customer contract ratio")
	}
	return ratio, nil
}

func applyCustomerContractToFloat(value float64, info *relaycommon.RelayInfo) (float64, error) {
	if info == nil || info.ContractBillingFact == nil {
		return value, nil
	}
	ratio, err := customerContractRatio(info)
	if err != nil {
		return 0, err
	}
	return decimal.NewFromFloat(value).Mul(ratio).InexactFloat64(), nil
}
