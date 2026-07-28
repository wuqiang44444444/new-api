package helper

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// ModelPriceHelperMediaImageTaskTiered reserves the configured async-task
// upper bound across every token dimension used by the frozen expression.
// This is intentionally conservative: an accepted upstream task must never
// outlive an unsafe, underfunded synchronous estimate.
func ModelPriceHelperMediaImageTaskTiered(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	exprString, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return types.PriceData{}, modelPriceNotConfiguredError(info.OriginModelName, info.UserId)
	}
	if err := validateAsyncExprDeterminism(exprString); err != nil {
		return types.PriceData{}, err
	}
	upperBound, ok := billing_setting.GetTaskPreConsumeTokens(info.OriginModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s task pre-consume token upper bound is not configured", info.OriginModelName)
	}

	probe := map[string]any{}
	if request, ok := info.Request.(*dto.ImageRequest); ok {
		count := uint(1)
		if request.N != nil {
			count = *request.N
		}
		probe["size"] = request.Size
		probe["n"] = count
	}
	probeBody, err := common.Marshal(map[string]any{"_task": probe})
	if err != nil {
		return types.PriceData{}, fmt.Errorf("marshal media image billing probe: %w", err)
	}
	requestInput := billingexpr.RequestInput{Body: probeBody}
	upper := float64(upperBound)
	params := billingexpr.TokenParams{
		P: upper, C: upper, Len: upper, CR: upper, CC: upper,
		CC1h: upper, Img: upper, ImgO: upper, AI: upper, AO: upper,
	}
	rawCost, trace, err := billingexpr.RunExprWithRequest(exprString, params, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s media image tiered expr run failed: %w", info.OriginModelName, err)
	}
	if rawCost < 0 {
		return types.PriceData{}, fmt.Errorf("model %s media image tiered expr returned negative cost", info.OriginModelName)
	}

	groupRatioInfo := HandleGroupRatio(c, info)
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatioInfo.GroupRatio)
	if err != nil {
		return types.PriceData{}, err
	}
	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && groupRatioInfo.GroupRatio == 0 {
		preConsumedQuota = 0
		freeModel = true
	}

	info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprString,
		ExprHash:                  billingexpr.ExprHashString(exprString),
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedCompletionTokens: upperBound,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprString),
	}
	info.BillingRequestInput = &requestInput
	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		Quota:             preConsumedQuota,
		QuotaToPreConsume: preConsumedQuota,
	}
	info.PriceData = priceData
	return priceData, nil
}
