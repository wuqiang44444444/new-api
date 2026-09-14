package service

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const flashDisplayExpr = `tier("base", p * 1.5 + cr * 0.05 + c * 4.5) * (
  weekday("Asia/Shanghai") >= 1 &&
  weekday("Asia/Shanghai") <= 5 &&
  (
    (hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) ||
    (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18)
  )
  ? 2 : 1
) / 6.71`

func TestAttachPricingBillingDisplay(t *testing.T) {
	items := []model.Pricing{
		{ModelName: "flash", BillingMode: "tiered_expr", BillingExpr: flashDisplayExpr},
		{ModelName: "static", QuotaType: 0, BillingExpr: ""},
		{ModelName: "task", BillingMode: "tiered_expr", BillingExpr: `tier("base", u("seconds") * 0.4)`},
	}
	AttachPricingBillingDisplay(items)

	require.NotNil(t, items[0].BillingDisplay)
	assert.Equal(t, billingexpr.DisplayStatusExact, items[0].BillingDisplay.Status)
	assert.InDelta(t, 1.5/6.71, items[0].BillingDisplay.Tiers[0].UnitPrices["p"], 1e-15)

	// 非表达式模型不附加投影。
	assert.Nil(t, items[1].BillingDisplay)
	// 任务表达式投影整体 opaque，不输出局部价格。
	require.NotNil(t, items[2].BillingDisplay)
	assert.Equal(t, billingexpr.DisplayStatusOpaque, items[2].BillingDisplay.Status)
	assert.Equal(t, billingexpr.DisplayReasonTaskUsageExpression, items[2].BillingDisplay.Reason)
}

func TestAttachPricingBillingDisplayTaskSchema(t *testing.T) {
	items := []model.Pricing{
		{
			ModelName:   "seedance-task",
			BillingMode: "tiered_expr",
			BillingExpr: `tier("base", u("tokens") * 9.8 / 1000000 + 0.2)`,
			BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{
				"tokens": {Type: "number", Unit: "token"},
			},
			BillingUsageExamples: []jsplugin.UsageExample{
				{Label: "sample", Facts: map[string]any{"tokens": float64(1000000)}},
			},
		},
		{
			// 用量字段合同缺 seconds：表达式引用未声明字段必须失败关闭。
			ModelName:   "bad-schema",
			BillingMode: "tiered_expr",
			BillingExpr: `tier("base", u("seconds") * 0.4)`,
			BillingUsageSchema: map[string]jsplugin.UsageFieldSchema{
				"tokens": {Type: "number", Unit: "token"},
			},
		},
	}
	AttachPricingBillingDisplay(items)

	// 携带类型化字段合同的任务表达式按任务 USD 单位投影，token 字段展示
	// 单价换算为 USD/每百万。
	require.NotNil(t, items[0].BillingDisplay)
	assert.Equal(t, billingexpr.DisplayStatusExact, items[0].BillingDisplay.Status)
	assert.Equal(t, billingexpr.DisplayUnitTaskUsage, items[0].BillingDisplay.Unit)
	assert.InDelta(t, 9.8, items[0].BillingDisplay.Tiers[0].UnitPrices["tokens"], 1e-12)
	assert.InDelta(t, 0.2, items[0].BillingDisplay.Tiers[0].Constant, 1e-12)
	// 示例金额由装配阶段求值：9.8 + 0.2。
	require.Len(t, items[0].BillingUsageExamples, 1)
	assert.InDelta(t, 10.0, items[0].BillingUsageExamples[0].Total, 1e-9)

	require.NotNil(t, items[1].BillingDisplay)
	assert.Equal(t, billingexpr.DisplayStatusOpaque, items[1].BillingDisplay.Status)
	assert.Empty(t, items[1].BillingUsageExamples)
}

func TestAttachLogsBillingDisplay(t *testing.T) {
	exprB64 := base64.StdEncoding.EncodeToString([]byte(flashDisplayExpr))
	other := `{"billing_mode":"tiered_expr","expr_b64":"` + exprB64 + `","quota":123}`
	logs := []*model.Log{
		{Other: other},
		{Other: `{"billing_mode":"default"}`},
		{Other: `not-json`},
		{Other: ``},
	}
	AttachLogsBillingDisplay(logs)

	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &payload))
	_, hasDisplay := payload["billing_display"]
	require.True(t, hasDisplay, "tiered log must gain billing_display")
	// 已发生的费用字段保持不变。
	assert.Equal(t, float64(123), payload["quota"])
	assert.Equal(t, "tiered_expr", payload["billing_mode"])

	assert.NotContains(t, logs[1].Other, "billing_display")
	assert.Equal(t, "not-json", logs[2].Other)
	assert.Equal(t, "", logs[3].Other)
}

func TestAttachPricingBillingDisplayDoesNotMutateSharedExampleSlice(t *testing.T) {
	shared := []jsplugin.UsageExample{{Label: "sample", Facts: map[string]any{"tokens": float64(1000000)}}}
	item := model.Pricing{
		ModelName:            "seedance-task",
		BillingMode:          "tiered_expr",
		BillingExpr:          `tier("base", u("tokens") * 9.8 / 1000000)`,
		BillingUsageSchema:   map[string]jsplugin.UsageFieldSchema{"tokens": {Type: "number", Unit: "token"}},
		BillingUsageExamples: shared,
	}
	items := []model.Pricing{item}
	AttachPricingBillingDisplay(items)

	require.Len(t, items[0].BillingUsageExamples, 1)
	assert.InDelta(t, 9.8, items[0].BillingUsageExamples[0].Total, 1e-9)
	// 与定价缓存共享底层数组的原始切片必须保持未写入。
	assert.Zero(t, shared[0].Total)
}
