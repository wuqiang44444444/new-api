package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ChannelTestPricing is a read-only native price result, with the canonical
// usage and auxiliary price facts needed by its log. It never settles funds.
type ChannelTestPricing struct {
	Quota   int
	Result  *billingexpr.TieredResult
	Usage   *dto.Usage
	summary textQuotaSummary
}

// CalculateChannelTestQuota excludes customer group/contract discounts (even
// free groups) without mutating the caller's snapshot or touching wallet/counters.
func CalculateChannelTestQuota(ctx *gin.Context, info *relaycommon.RelayInfo, prices hosttypes.PriceData, usage *dto.Usage) ChannelTestPricing {
	pricingInfo := *info
	pricingInfo.PriceData = prices
	pricingInfo.PriceData.GroupRatioInfo.GroupRatio = 1
	pricingInfo.ContractBillingFact = nil
	pricingInfo.QuotaClamp = nil
	billingUsage := effectiveBillingUsage(usage)
	var quote ChannelTestPricing
	if info.TieredBillingSnapshot != nil && info.TieredBillingSnapshot.BillingMode == "tiered_expr" {
		snapshot := *info.TieredBillingSnapshot
		snapshot.GroupRatio = 1
		pricingInfo.TieredBillingSnapshot = &snapshot
		// Expressions are self-contained; do not evaluate an unused ratio price.
		quote.summary = textQuotaSummary{ModelName: info.GetBillingModelName(), GroupRatio: 1, UsageSemantic: usageSemanticFromUsage(info, billingUsage)}
		quote.summary.IsClaudeUsageSemantic = quote.summary.UsageSemantic == dto.BillingUsageSemanticAnthropic
		quote.summary.ToolCallSurchargeQuota = calculateTextToolCallSurcharge(ctx, &pricingInfo, &quote.summary)
		_, quote.Quota, quote.Result = TryTieredSettle(&pricingInfo, BuildTieredTokenParams(billingUsage, quote.summary.IsClaudeUsageSemantic, billingexpr.UsedVars(snapshot.ExprString)))
		if quote.Result != nil {
			quote.Quota = composeTieredTextQuota(&pricingInfo, quote.summary, quote.Quota, quote.Result)
		}
	} else {
		quote.summary = calculateTextQuotaSummary(ctx, &pricingInfo, billingUsage)
		quote.Quota = quote.summary.Quota
		if prices.UsePrice && !quote.summary.hasBillableUsage() {
			// A successful fixed-price test is one call without a token meter too.
			amount := prices.ApplyOtherRatiosToDecimal(decimal.NewFromFloat(prices.ModelPrice).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
			var clamp *common.QuotaClamp
			quote.Quota, clamp = common.QuotaFromDecimalChecked(amount)
			noteQuotaClamp(&pricingInfo, clamp)
		}
	}
	noteQuotaClamp(info, pricingInfo.QuotaClamp)
	pricedUsage := *billingUsage
	pricedUsage.UsageSemantic = quote.summary.UsageSemantic
	quote.Usage = &pricedUsage
	return quote
}

// AppendLogInfo freezes the same auxiliary facts used by native pricing and
// keeps estimates and saturated results auditable without storing response bodies.
func (quote ChannelTestPricing) AppendLogInfo(ctx *gin.Context, info *relaycommon.RelayInfo, other *model.LogOther, originUsage *dto.Usage) {
	appendUsageBillingPathForLog(other, common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens), originUsage)
	appendToolSurchargeLogInfo(other, quote.summary.ToolSurchargeItems)
	if quote.summary.AudioInputPrice > 0 && quote.summary.AudioTokens > 0 {
		other.SetPublic("audio_input_seperate_price", true)
		other.SetPublic("audio_input_token_count", quote.summary.AudioTokens)
		other.SetPublic("audio_input_price", quote.summary.AudioInputPrice)
	}
	if quote.summary.ImageTokens > 0 {
		other.SetPublic("image", true)
		other.SetPublic("image_ratio", quote.summary.ImageRatio)
		other.SetPublic("image_output", quote.summary.ImageTokens)
	}
	attachQuotaSaturation(ctx, info, other)
}
