package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
)

func appendSynchronousStatementOriginal(other *model.LogOther, funded, hasUsage, tiered bool, result *billingexpr.TieredResult, original *int) {
	if !funded {
		return
	}
	if tiered {
		if result == nil || result.Clamp != nil || result.ActualQuotaBeforeGroup < 0 {
			return
		}
		quota, err := common.QuotaRoundStrict(result.ActualQuotaBeforeGroup)
		if err != nil {
			return
		}
		original = &quota
	}
	if original == nil || *original < 0 {
		return
	}
	if !hasUsage {
		other.SetAdmin(model.StatementOriginalQuotaLogKey, 0)
		return
	}
	other.SetAdmin(model.StatementOriginalQuotaLogKey, *original)
}

func calculateAudioQuota(info QuotaInfo) (int, *common.QuotaClamp, error) {
	_, amount, err := audioQuotaAmounts(info)
	if err != nil {
		return 0, nil, err
	}
	quota, clamp := common.QuotaFromDecimalChecked(amount)
	return quota, clamp, nil
}

func calculateAudioQuotaWithOriginal(info QuotaInfo) (int, *common.QuotaClamp, *int, error) {
	original, amount, err := audioQuotaAmounts(info)
	if err != nil {
		return 0, nil, nil, err
	}
	quota, clamp := common.QuotaFromDecimalChecked(amount)
	if clamp != nil || original.IsNegative() {
		return quota, clamp, nil, nil
	}
	if !info.UsePrice && info.ModelRatio > 0 && original.IsZero() {
		original = decimal.NewFromInt(1)
	}
	originalQuota, originalErr := common.QuotaFromDecimalStrict(original)
	if originalErr != nil {
		return quota, clamp, nil, nil
	}
	return quota, clamp, &originalQuota, nil
}

func audioQuotaAmounts(info QuotaInfo) (decimal.Decimal, decimal.Decimal, error) {
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		original := modelPrice.Mul(quotaPerUnit)
		quota := original.Mul(groupRatio)
		quota, err := ApplyCustomerContractRatio(quota, info.ContractFact)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		return original, quota, nil
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	ratio := groupRatio.Mul(modelRatio)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))

	original := quota.Mul(modelRatio)
	quota = quota.Mul(ratio)
	quota, err := ApplyCustomerContractRatio(quota, info.ContractFact)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return original, quota, nil
}
