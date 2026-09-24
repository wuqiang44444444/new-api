package helper

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
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
//
// Seedance Link customer models price through the upstream u() engine with the
// controlled usage facts built from the same typed probe: u("tokens") carries
// the administrator budget at submission and is replaced by accepted actual
// usage at settlement, while every other declared field stays a frozen request
// condition. Other task channel types retain their existing token contract.
func ModelPriceHelperTaskTiered(c *gin.Context, info *relaycommon.RelayInfo, adaptor any) (types.PriceData, error) {
	exprString, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return types.PriceData{}, modelPriceNotConfiguredError(info.OriginModelName, info.UserId)
	}
	// 异步任务 tiered 表达式禁止非确定性函数（P1-B）：预扣与终态结算分别求值同一表达式，
	// 而快照只冻结 _task body 与 token，不冻结请求头与求值时间，header()/hour() 等会导致两次
	// 求值结果不一致。同步请求只求值一次，不受此约束（走 ModelPriceHelper 路径）。
	if err := billingexpr.ValidateAsyncDeterminism(exprString); err != nil {
		return types.PriceData{}, err
	}
	estimatedTokens, ok := billing_setting.GetTaskPreConsumeTokens(info.OriginModelName)
	// 已登记的 FunCloud/Synlink 纯冻结参数表达式例外：无实测用量依赖时不强制预扣预算；
	// 依赖 u("tokens") 的新 Seedance 表达式仍然要求有效预算（不随迁移放宽既有协议规则）。
	// MiniMax Link 的 jdcloud 协议同样是宿主探针参数计费：schema 只含宿主事实，
	// 不含 tokens，credit 证据永远不是客户计价乘数。
	isMinimaxLinkChannel := info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeMiniMaxLink
	taskUsageBilling := info.ChannelMeta != nil &&
		(info.ChannelType == constant.ChannelTypeSeedanceLink || isMinimaxLinkChannel)
	if taskUsageBilling {
		exprSchema := model.StandardVideoBillingFields(info.ChannelType, info.ChannelOtherSettings.VideoUpstreamProtocol)
		if err := seedancebilling.ValidateTaskExpressionInputs(exprString, exprSchema); err != nil {
			return types.PriceData{}, err
		}
	} else if vars := billingexpr.UsedVars(exprString); vars["u"] {
		return types.PriceData{}, fmt.Errorf("model %s task usage expression requires the Seedance Link channel contract", info.OriginModelName)
	}
	if !ok && (!taskUsageBilling || (!isMinimaxLinkChannel && seedancebilling.RequiresTokenBudget(info.ChannelOtherSettings.VideoUpstreamProtocol, exprString))) {
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

	params := billingexpr.TokenParams{}
	var usageFacts map[string]any
	if taskUsageBilling {
		usageFacts, err = seedancebilling.ControlledFacts(probeBody, estimatedTokens)
		if err != nil {
			return types.PriceData{}, err
		}
		requestInput.Usage = usageFacts
	} else {
		params = billingexpr.TokenParams{C: float64(estimatedTokens)}
	}

	if err := AttachFrozenExchangeRate(exprString, &requestInput, info.TieredBillingSnapshot); err != nil {
		return types.PriceData{}, fmt.Errorf("model %s: %w", info.OriginModelName, err)
	}

	groupRatioInfo := HandleGroupRatio(c, info)
	rawCost, trace, err := billingexpr.RunExprWithRequest(exprString, params, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s task tiered expr run failed: %w", info.OriginModelName, err)
	}
	if rawCost < 0 {
		return types.PriceData{}, fmt.Errorf("model %s task tiered expr returned negative cost", info.OriginModelName)
	}

	// u() 任务表达式已经输出美元（上游引擎 TaskUsageBilling 合同）；c/_task 旧表达式
	// 系数为 $/1M tokens，沿用宿主百万换算。单位由快照冻结标记分派，结算与补查一致。
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	if taskUsageBilling {
		quotaBeforeGroup = rawCost * common.QuotaPerUnit
	}
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

	snapshot := &billingexpr.BillingSnapshot{
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
		TaskUsageBilling:          taskUsageBilling,
		UsageFacts:                usageFacts,
		UsdExchangeRate:           requestInput.ExchangeRate,
	}
	if taskUsageBilling {
		schema := model.StandardVideoBillingFields(info.ChannelType, info.ChannelOtherSettings.VideoUpstreamProtocol)
		snapshot.UsageUnits = seedancebilling.UsageUnitsForSchema(schema)
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:      freeModel,
		GroupRatioInfo: groupRatioInfo,
		Quota:          preConsumedQuota,
	}
	info.PriceData = priceData
	return priceData, nil
}
