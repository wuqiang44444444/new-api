package service

import (
	"encoding/base64"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// 只读投影的响应装配：为已经过权限过滤与脱敏的响应附加冻结表达式的展示
// 投影。同一装配逻辑服务公开定价、合同定价与日志响应；投影缺失时前端按
// 「暂无法展开」处理，不回退本地猜价。

// AttachPricingBillingDisplayOne 为一个已通过可见性过滤的表达式定价模型
// 附加只读投影。任务用量表达式由既有 schema 驱动展示负责，不在此投影。
func AttachPricingBillingDisplayOne(item *model.Pricing) {
	expression := item.BillingExpr
	if strings.TrimSpace(expression) == "" {
		return
	}
	projection, err := billingexpr.DisplayProjectionFor(expression)
	if err != nil {
		return
	}
	item.BillingDisplay = projection
}

// AttachPricingBillingDisplay 为响应中可见的表达式定价模型附加只读投影。
func AttachPricingBillingDisplay(items []model.Pricing) {
	for i := range items {
		AttachPricingBillingDisplayOne(&items[i])
	}
}

// AttachLogsBillingDisplay 在权限过滤与脱敏之后，为携带冻结表达式的日志
// 附加响应级投影。只修改响应对象，不更新数据库日志；历史日志金额以日志
// 自身记录为准，投影仅解释当次冻结的表达式。
func AttachLogsBillingDisplay(logs []*model.Log) {
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
