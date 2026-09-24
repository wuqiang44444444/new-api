package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// Both model and customer amounts share the same actual evaluation. Discount is
// applied to the unrounded decimal, exactly as in the original per-line path.
func computeBatchLineCalculation(frozen *model.BatchFrozenSnapshot, usage BatchLineUsage) (int, int, *common.QuotaClamp, *billingexpr.Calculation, error) {
	value, c, err := batchLineModelCost(frozen, usage)
	if err != nil {
		return 0, 0, nil, nil, err
	}
	modelQuota, clamp := common.QuotaFromDecimalChecked(value)
	if frozen.ContractFact != nil {
		before := value
		value, err = ApplyCustomerContractRatio(value, frozen.ContractFact)
		if err != nil {
			return 0, 0, nil, nil, err
		}
		c.Add("contract_ratio", "quota", value.String(), before.String(), frozen.ContractFact.RatioString())
	}
	quota, finalClamp := common.QuotaFromDecimalChecked(value)
	if finalClamp != nil {
		clamp = finalClamp
	}
	c.Add("truncate", "quota", quota, value.String())
	return modelQuota, quota, clamp, c.Finish(quota), nil
}
