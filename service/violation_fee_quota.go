package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Shared by native charging and frozen task settlement, including zero ratios.
func calcViolationFeeQuotaChecked(amount, groupRatio float64) (int, *common.QuotaClamp) {
	if amount <= 0 || groupRatio <= 0 {
		return 0, nil
	}
	return common.QuotaFromDecimalChecked(decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(groupRatio)).Round(0))
}
