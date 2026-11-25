package service

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ArkSessionInferenceHold bounds the wallet precharge for a text-only Session.
func ArkSessionInferenceHold(info *relaycommon.RelayInfo, inputLimit, outputLimit int) (int, error) {
	if snapshot := info.TieredBillingSnapshot; snapshot != nil {
		// Use the largest bounded charge across all linear tiers. The native
		// estimate considers input only and does not bound the output charge.
		projection, err := billingexpr.DisplayProjectionForWithRate(snapshot.ExprString, snapshot.UsdExchangeRate)
		if err != nil || projection.Status != billingexpr.DisplayStatusExact || len(projection.Rules) != 0 || len(projection.Tiers) == 0 {
			return 0, errors.New("Ark Session requires a bounded text pricing expression")
		}
		var maximum float64
		for _, tier := range projection.Tiers {
			for quantity, rate := range tier.UnitPrices {
				if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
					return 0, model.ErrArkCacheFunds
				}
				switch quantity {
				case "p", "cr", "len", "c":
				case "cc", "cc1h", "cc12h", "img", "img_cr", "img_o", "ai", "ao":
					// Ark Session's text receipt does not report these dimensions.
				default:
					if rate != 0 {
						return 0, errors.New("Ark Session requires bounded text pricing quantities")
					}
				}
			}
			if tier.Constant < 0 {
				return 0, model.ErrArkCacheFunds
			}
			amount := (float64(inputLimit)*(max(tier.UnitPrices["p"], tier.UnitPrices["cr"])+tier.UnitPrices["len"]) +
				float64(outputLimit)*tier.UnitPrices["c"]) / 1_000_000
			maximum = max(maximum, amount+tier.Constant)
		}
		if projection.ConstantCharge != nil {
			if *projection.ConstantCharge < 0 {
				return 0, model.ErrArkCacheFunds
			}
			maximum += *projection.ConstantCharge
		}
		quota := maximum * snapshot.QuotaPerUnit * snapshot.GroupRatio
		if math.IsNaN(quota) || math.IsInf(quota, 0) || quota < 0 {
			return 0, model.ErrArkCacheFunds
		}
		charge, err := ApplyCustomerContractRatio(decimal.NewFromFloat(quota), info.ContractBillingFact)
		if err != nil {
			return 0, err
		}
		return common.QuotaFromDecimalStrict(charge.Ceil())
	}
	price := info.PriceData
	if price.UsePrice || len(price.OtherRatios()) != 0 {
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
	var tieredResult *billingexpr.TieredResult
	if snap := info.TieredBillingSnapshot; snap != nil {
		params := BuildTieredTokenParams(usage, false, billingexpr.UsedVarsByHash(snap.ExprString, snap.ExprHash))
		ok, quota, result := TryTieredSettle(info, params)
		if !ok || result == nil {
			return 0, "", model.ErrArkCacheFunds
		}
		summary.Quota, tieredResult = quota, result
		summary.FixedPriceBilling = isFixedPriceSettlement(info, result)
	}
	info.BillingSource = BillingSourceWallet
	other := GenerateTextOtherInfo(c, info, summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio,
		summary.CacheTokens, summary.CacheRatio, summary.ModelPrice, info.PriceData.GroupRatioInfo.GroupSpecialRatio)
	other.SetPublic("input_tokens_total", usage.PromptTokens)
	other.SetPublic("billing_source", BillingSourceWallet)
	other.SetAdmin("gateway_request_id", c.GetString(common.RequestIdKey))
	appendCustomerContractBillingInfo(other, info.ContractBillingFact)
	appendUsageBillingPathForLog(other, false, usage)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, info, tieredResult)
	}
	appendSynchronousStatementOriginal(other, info.QuotaClamp == nil, summary.hasBillableUsage(), tieredResult != nil, tieredResult, summary.OriginalQuota)
	appendBillingStatementSnapshot(info, other)
	appendTextStatementUsage(other, info, usage, summary)
	attachQuotaSaturation(c, info, other)
	if info.QuotaClamp != nil {
		return 0, "", model.ErrArkCacheFunds
	}
	return summary.Quota, other.JSONString(), nil
}
