package service

import (
	"encoding/base64"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 只读投影的响应装配：为已经过权限过滤与脱敏的响应附加冻结表达式的展示
// 投影。同一装配逻辑服务公开定价、合同定价与日志响应；投影缺失时前端按
// 「暂无法展开」处理，不回退本地猜价。

// currentDisplayExchangeRate 解析当前系统汇率用于「本次响应绑定」的展示
// 投影。设置缺失或非法时返回 nil：展示按 exchange_rate_unresolved 明确
// 不可展开处理，不猜价；资金路径的失败关闭行为不受影响。
func currentDisplayExchangeRate() *billingexpr.ExchangeRateContext {
	rate, err := operation_setting.CurrentUsdExchangeRateContext()
	if err != nil {
		return nil
	}
	return rate
}

// frozenLogExchangeRate 提取历史日志记录的冻结汇率事实。旧日志从未记录
// 汇率时返回 nil，投影明确不可展开，禁止用当前设置反算历史价格。
func frozenLogExchangeRate(other map[string]any) *billingexpr.ExchangeRateContext {
	raw, _ := other["usd_exchange_rate"].(map[string]any)
	return billingexpr.ParseExchangeRateFact(raw)
}

// AttachPricingBillingDisplayOne 为一个已通过可见性过滤的表达式定价模型
// 附加只读投影。携带用量字段合同的任务表达式按任务 USD 单位投影；其余
// 按通用 token 单位投影。单位由调用方传入的类型化字段事实决定，不以表达
// 式文本中是否出现 u() 判断。
func AttachPricingBillingDisplayOne(item *model.Pricing) {
	expression := item.BillingExpr
	if strings.TrimSpace(expression) == "" {
		return
	}
	if len(item.BillingUsageSchema) > 0 {
		fields := make(map[string]billingexpr.TaskUsageFieldInfo, len(item.BillingUsageSchema))
		for name, field := range item.BillingUsageSchema {
			fields[name] = billingexpr.TaskUsageFieldInfo{Unit: field.Unit}
		}
		rate := currentDisplayExchangeRate()
		projection, err := billingexpr.TaskDisplayProjectionForWithRate(expression, fields, rate)
		if err != nil {
			return
		}
		item.BillingDisplay = projection
		item.BillingUsageExamples = pricedUsageExamples(expression, rate, item.BillingUsageExamples)
		return
	}
	projection, err := billingexpr.DisplayProjectionForWithRate(expression, currentDisplayExchangeRate())
	if err != nil {
		return
	}
	item.BillingDisplay = projection
}

// pricedUsageExamples 返回带 USD 金额的新示例切片。调用方传入的切片可能
// 与定价缓存共享底层数组，必须写时复制，不得就地修改共享缓存。示例金额
// 以本次响应绑定的汇率上下文求值。
func pricedUsageExamples(expression string, rate *billingexpr.ExchangeRateContext, examples []jsplugin.UsageExample) []jsplugin.UsageExample {
	if len(examples) == 0 {
		return examples
	}
	result := make([]jsplugin.UsageExample, len(examples))
	copy(result, examples)
	for i := range result {
		total, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: result[i].Facts, ExchangeRate: rate})
		if err != nil || total < 0 || math.IsNaN(total) || math.IsInf(total, 0) {
			continue
		}
		result[i].Total = total
	}
	return result
}

// AttachPricingBillingDisplay 为响应中可见的表达式定价模型附加只读投影。
func AttachPricingBillingDisplay(items []model.Pricing) {
	for i := range items {
		AttachPricingBillingDisplayOne(&items[i])
	}
}

// AttachLogsBillingDisplay 在权限过滤与脱敏之后，为携带冻结表达式的日志
// 附加响应级投影。只修改响应对象，不更新数据库日志；历史日志金额以日志
// 自身记录为准，投影仅解释当次冻结的表达式。历史 `_task` 快照沿用既有
// 历史合同解释，不按新任务单位投影重写。
func AttachLogsBillingDisplay(logs []*model.Log) {
	attachCustomerBillingExplanations(logs)
	for i := range logs {
		other := logs[i].Other
		if !strings.Contains(other, "expr_b64") {
			continue
		}
		var payload map[string]any
		if err := common.UnmarshalJsonStr(other, &payload); err != nil {
			continue
		}
		if payload["billing_mode"] != "tiered_expr" {
			continue
		}
		encoded, _ := payload["expr_b64"].(string)
		if encoded == "" {
			continue
		}
		expression, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		frozenRate := frozenLogExchangeRate(payload)
		projection, err := billingexpr.DisplayProjectionForWithRate(string(expression), frozenRate)
		if err != nil {
			continue
		}
		payload["billing_display"] = projection
		if encoded, err := common.Marshal(payload); err == nil {
			logs[i].Other = string(encoded)
		}
	}
}

// AttachModelPricingBillingDisplay uses the same projection for the rc36 editor API.
func AttachModelPricingBillingDisplay(snapshot *model.ModelPricingSnapshot) {
	for i := range snapshot.Entries {
		expression, _ := snapshot.Entries[i].Effective["billing_setting.billing_expr"].(string)
		if expression == "" {
			continue
		}
		projection, err := billingexpr.DisplayProjectionForWithRate(expression, currentDisplayExchangeRate())
		if err == nil {
			snapshot.Entries[i].BillingDisplay = projection
		}
	}
}
