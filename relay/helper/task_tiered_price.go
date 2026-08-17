package helper

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type TaskBillingProbeProvider interface {
	BuildTaskBillingProbe(c *gin.Context, info *relaycommon.RelayInfo) (map[string]any, error)
}

// ModelPriceHelperTaskTiered evaluates a task expression with an
// administrator-configured maximum billable-token estimate. The expression,
// trusted request probe and estimate are frozen on RelayInfo for settlement.
func ModelPriceHelperTaskTiered(c *gin.Context, info *relaycommon.RelayInfo, adaptor any) (types.PriceData, error) {
	exprString, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return types.PriceData{}, modelPriceNotConfiguredError(info.OriginModelName, info.UserId)
	}
	// 异步任务 tiered 表达式禁止非确定性函数（P1-B）：预扣与终态结算分别求值同一表达式，
	// 而快照只冻结 _task body 与 token，不冻结请求头与求值时间，header()/hour() 等会导致两次
	// 求值结果不一致。同步请求只求值一次，不受此约束（走 ModelPriceHelper 路径）。
	if err := validateAsyncExprDeterminism(exprString); err != nil {
		return types.PriceData{}, err
	}
	estimatedTokens, ok := billing_setting.GetTaskPreConsumeTokens(info.OriginModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s task pre-consume token upper bound is not configured", info.OriginModelName)
	}

	probe := map[string]any{}
	if provider, supported := adaptor.(TaskBillingProbeProvider); supported {
		var err error
		probe, err = provider.BuildTaskBillingProbe(c, info)
		if err != nil {
			return types.PriceData{}, err
		}
	}
	probeBody, err := common.Marshal(map[string]any{"_task": probe})
	if err != nil {
		return types.PriceData{}, fmt.Errorf("marshal task billing probe: %w", err)
	}
	requestInput := billingexpr.RequestInput{
		Headers: cloneStringMap(info.RequestHeaders),
		Body:    probeBody,
	}

	groupRatioInfo := HandleGroupRatio(c, info)
	rawCost, trace, err := billingexpr.RunExprWithRequest(exprString, billingexpr.TokenParams{
		C: float64(estimatedTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s task tiered expr run failed: %w", info.OriginModelName, err)
	}
	if rawCost < 0 {
		return types.PriceData{}, fmt.Errorf("model %s task tiered expr returned negative cost", info.OriginModelName)
	}

	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	estimatedQuota, err := applyCustomerContractToFloat(quotaBeforeGroup*groupRatioInfo.GroupRatio, info)
	if err != nil {
		return types.PriceData{}, err
	}
	preConsumedQuota, err := billingexpr.QuotaRoundStrict(estimatedQuota)
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
		EstimatedCompletionTokens: estimatedTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprString),
	}
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:      freeModel,
		GroupRatioInfo: groupRatioInfo,
		Quota:          preConsumedQuota,
	}
	info.PriceData = priceData
	return priceData, nil
}

// asyncForbiddenExprVars 列出异步任务 tiered 表达式禁止使用的非确定性标识符。
// 异步任务预扣与终态结算分别求值同一表达式，而 BillingSnapshot 只冻结 _task body 探针和
// token 估算，不冻结请求头（header）与求值时间（hour/minute/weekday/month/day）。
// 允许这些函数会使预扣价与结算价不一致，违背确定性复算不变量（P1-B）。
var asyncForbiddenExprVars = map[string]bool{
	"header":  true,
	"hour":    true,
	"minute":  true,
	"weekday": true,
	"month":   true,
	"day":     true,
}

// validateAsyncExprDeterminism 拒绝异步任务 tiered 表达式使用非确定性函数。同步请求只求值一次，
// header()/hour() 合法；异步任务预扣与结算两次求值上下文不同，必须禁用以保证价格一致。
func validateAsyncExprDeterminism(exprString string) error {
	for name := range billingexpr.UsedVars(exprString) {
		if asyncForbiddenExprVars[name] {
			return fmt.Errorf("model task tiered expr must not use non-deterministic function %q: header/time are not frozen across pre-consume and settlement", name)
		}
	}
	return nil
}
