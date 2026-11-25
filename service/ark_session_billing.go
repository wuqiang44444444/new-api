package service

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ArkSessionInferenceHold bounds the wallet precharge for the first Session
// pilot. Its text-only contract runs the same token pricing as native Responses.
func ArkSessionInferenceHold(info *relaycommon.RelayInfo, inputLimit, outputLimit int) (int, error) {
	price := info.PriceData
	if price.UsePrice || info.TieredBillingSnapshot != nil || len(price.OtherRatios()) != 0 {
		return 0, errors.New("Ark Session pilot requires text token-ratio pricing")
	}
	for _, ratio := range []float64{price.ModelRatio, price.CompletionRatio, price.CacheRatio, price.GroupRatioInfo.GroupRatio} {
		if math.IsInf(ratio, 0) || math.IsNaN(ratio) || ratio < 0 {
			return 0, model.ErrArkCacheFunds
		}
	}
	input := decimal.NewFromInt(int64(inputLimit)).Mul(decimal.NewFromFloat(max(price.CacheRatio, 1)))
	output := decimal.NewFromInt(int64(outputLimit)).Mul(decimal.NewFromFloat(price.CompletionRatio))
	quota := input.Add(output).Mul(decimal.NewFromFloat(price.ModelRatio)).Mul(decimal.NewFromFloat(price.GroupRatioInfo.GroupRatio))
	quota, err := ApplyCustomerContractRatio(quota, info.ContractBillingFact)
	if err != nil {
		return 0, err
	}
	return common.QuotaFromDecimalStrict(quota.Ceil())
}

func ArkSessionInferenceFacts(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage) (int, string, error) {
	summary := calculateTextQuotaSummary(c, info, usage)
	info.BillingSource = BillingSourceWallet
	other := GenerateTextOtherInfo(c, info, summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio,
		summary.CacheTokens, summary.CacheRatio, summary.ModelPrice, info.PriceData.GroupRatioInfo.GroupSpecialRatio)
	other.SetPublic("input_tokens_total", usage.PromptTokens)
	other.SetPublic("billing_source", BillingSourceWallet)
	other.SetAdmin("gateway_request_id", c.GetString(common.RequestIdKey))
	appendCustomerContractBillingInfo(other, info.ContractBillingFact)
	appendUsageBillingPathForLog(other, false, usage)
	appendSynchronousStatementOriginal(other, info.QuotaClamp == nil, summary.hasBillableUsage(), false, nil, summary.OriginalQuota)
	appendBillingStatementSnapshot(info, other)
	appendTextStatementUsage(other, info, usage, summary)
	attachQuotaSaturation(c, info, other)
	if info.QuotaClamp != nil {
		return 0, "", model.ErrArkCacheFunds
	}
	return summary.Quota, other.JSONString(), nil
}
