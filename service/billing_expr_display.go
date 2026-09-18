package service

import (
	"encoding/base64"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

// 只读投影的响应装配：为已经过权限过滤与脱敏的响应附加冻结表达式的展示
// 投影。同一装配逻辑服务公开定价、合同定价与日志响应；投影缺失时前端按
// 「暂无法展开」处理，不回退本地猜价。

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
		projection, err := billingexpr.TaskDisplayProjectionFor(expression, fields)
		if err != nil {
			return
		}
		item.BillingDisplay = projection
		item.BillingUsageExamples = pricedUsageExamples(expression, item.BillingUsageExamples)
		return
	}
	projection, err := billingexpr.DisplayProjectionFor(expression)
	if err != nil {
		return
	}
	item.BillingDisplay = projection
}

// pricedUsageExamples 返回带 USD 金额的新示例切片。调用方传入的切片可能
// 与定价缓存共享底层数组，必须写时复制，不得就地修改共享缓存。
func pricedUsageExamples(expression string, examples []jsplugin.UsageExample) []jsplugin.UsageExample {
	if len(examples) == 0 {
		return examples
	}
	result := make([]jsplugin.UsageExample, len(examples))
	copy(result, examples)
	for i := range result {
		total, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: result[i].Facts})
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
	projections := make(map[string]*billingexpr.DisplayProjection)
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
		projection, seen := projections[string(expression)]
		if !seen {
			projection, err = billingexpr.DisplayProjectionFor(string(expression))
			if err != nil {
				continue
			}
			projections[string(expression)] = projection
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
		projection, err := billingexpr.DisplayProjectionFor(expression)
		if err == nil {
			snapshot.Entries[i].BillingDisplay = projection
		}
	}
}
