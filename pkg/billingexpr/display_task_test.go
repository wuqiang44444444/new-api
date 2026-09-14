package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var taskDisplayTestFields = map[string]TaskUsageFieldInfo{
	"tokens":           {Unit: "token"},
	"duration_seconds": {Unit: "second"},
	"resolution":       {},
	"has_video_input":  {},
	"generate_audio":   {},
}

func requireTaskExact(t *testing.T, p *DisplayProjection) {
	t.Helper()
	require.NotNil(t, p)
	require.Equal(t, DisplayStatusExact, p.Status)
	require.Equal(t, DisplayUnitTaskUsage, p.Unit)
}

func findTaskTier(t *testing.T, p *DisplayProjection, label string) DisplayTier {
	t.Helper()
	requireTaskExact(t, p)
	for _, tier := range p.Tiers {
		if tier.Label == label {
			return tier
		}
	}
	t.Fatalf("tier %q not found", label)
	return DisplayTier{}
}

func TestBuildTaskDisplayProjectionSimpleUnitPrice(t *testing.T) {
	expression := `tier("base", u("duration_seconds") * 0.25)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 1)
	tier := projection.Tiers[0]
	assert.Equal(t, "base", tier.Label)
	assert.InEpsilon(t, 0.25, tier.UnitPrices["duration_seconds"], 1e-12)
}

func TestBuildTaskDisplayProjectionTokenScaling(t *testing.T) {
	// u("tokens") * 5 / 1000000：表达式系数是每 token 5e-6 USD；展示单价按
	// 前端任务编辑器约定换算为 USD/每百万 = 5。
	expression := `tier("base", u("tokens") * 5 / 1000000)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 1)
	assert.InEpsilon(t, 5.0, projection.Tiers[0].UnitPrices["tokens"], 1e-12)
}

func TestBuildTaskDisplayProjectionFixedPlusUsageKeepsUSDConstant(t *testing.T) {
	expression := `tier("base", u("tokens") * 5 / 1000000 + 0.02)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 1)
	tier := projection.Tiers[0]
	assert.InEpsilon(t, 5.0, tier.UnitPrices["tokens"], 1e-12)
	require.True(t, tier.HasConstant)
	assert.InEpsilon(t, 0.02, tier.Constant, 1e-12)
	// 档位内常量保留在档位上；聚合 ConstantCharge 只来自顶层固定项。
	assert.Nil(t, projection.ConstantCharge)
}

func TestBuildTaskDisplayProjectionNestedBothSideTernaries(t *testing.T) {
	// 该嵌套形状是 21 个 Seedance 卡片显示原式的根因:通用 token 投影要求
	// 成立分支直接为 tier();任务投影两侧递归展开并保留前序条件否定。
	expression := `u("has_video_input") ? (u("resolution") == "4k" ? tier("4k", u("tokens") * 6 / 1000000) : tier("paid", u("tokens") * 3 / 1000000)) : tier("no_video", u("tokens") * 1 / 1000000)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 3)

	tier4k := findTaskTier(t, projection, "4k")
	assert.InEpsilon(t, 6.0, tier4k.UnitPrices["tokens"], 1e-12)
	leaves := collectUsageLeaves(t, tier4k.Condition)
	require.Len(t, leaves, 2)
	assert.Equal(t, "has_video_input", leaves[0].Path)
	assert.Equal(t, "resolution", leaves[1].Path)
	assert.Equal(t, "==", leaves[1].CompareOp)
	assert.Equal(t, "4k", leaves[1].Value)

	tierPaid := findTaskTier(t, projection, "paid")
	assert.InEpsilon(t, 3.0, tierPaid.UnitPrices["tokens"], 1e-12)
	paidLeaves := collectUsageLeaves(t, tierPaid.Condition)
	require.Len(t, paidLeaves, 2)
	assert.Equal(t, "has_video_input", paidLeaves[0].Path)
	assert.Equal(t, "resolution", paidLeaves[1].Path)
	assert.Contains(t, tierPaid.ConditionText, "!")
	assert.Contains(t, tierPaid.ConditionText, "4k")

	tierNoVideo := findTaskTier(t, projection, "no_video")
	assert.InEpsilon(t, 1.0, tierNoVideo.UnitPrices["tokens"], 1e-12)
	noVideoLeaves := collectUsageLeaves(t, tierNoVideo.Condition)
	require.Len(t, noVideoLeaves, 1)
	assert.Equal(t, "has_video_input", noVideoLeaves[0].Path)
	assert.Contains(t, tierNoVideo.ConditionText, "!")

	total, _, err := RunExprWithRequest(expression, TokenParams{}, RequestInput{Usage: map[string]any{
		"tokens": float64(2_000_000), "has_video_input": true, "resolution": "4k",
	}})
	require.NoError(t, err)
	assert.InDelta(t, 12.0, total, 1e-9)
}

type polarityPred struct {
	path  string
	value string
	neg   bool
}

func collectPolarityPreds(t *testing.T, rule *DisplayRule, neg bool) []polarityPred {
	t.Helper()
	require.NotNil(t, rule)
	require.False(t, rule.TextOnly)
	if rule.Op == "not" {
		require.Len(t, rule.Children, 1)
		return collectPolarityPreds(t, &rule.Children[0], !neg)
	}
	if rule.Op == "and" || rule.Op == "or" {
		var preds []polarityPred
		for _, child := range rule.Children {
			preds = append(preds, collectPolarityPreds(t, &child, neg)...)
		}
		return preds
	}
	require.Equal(t, "usage", rule.Source)
	require.Equal(t, "==", rule.CompareOp)
	return []polarityPred{{path: rule.Path, value: rule.Value, neg: neg}}
}

func collectUsageLeaves(t *testing.T, rule *DisplayRule) []DisplayRule {
	t.Helper()
	require.NotNil(t, rule)
	if rule.TextOnly || len(rule.Children) == 0 {
		if rule.Source == "usage" {
			return []DisplayRule{*rule}
		}
		return nil
	}
	var leaves []DisplayRule
	for _, child := range rule.Children {
		leaves = append(leaves, collectUsageLeaves(t, &child)...)
	}
	return leaves
}

func TestBuildTaskDisplayProjectionOpaqueFailClosed(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		reason     string
	}{
		{name: "undeclared field", expression: `tier("base", u("secret") * 1)`, reason: DisplayReasonNonlinearPricing},
		{name: "squared usage", expression: `tier("base", u("tokens") * u("tokens"))`, reason: DisplayReasonNonlinearPricing},
		{name: "dynamic key", expression: `tier("base", u("tok"+"ens") * 1)`, reason: DisplayReasonNonlinearPricing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projection, err := BuildTaskDisplayProjection(tc.expression, taskDisplayTestFields)
			require.NoError(t, err)
			assert.Equal(t, DisplayStatusOpaque, projection.Status)
			assert.Equal(t, tc.reason, projection.Reason)
			assert.Empty(t, projection.Tiers)
		})
	}
}

func TestTaskDisplayProjectionCacheContextSeparation(t *testing.T) {
	expression := `tier("base", u("tokens") * 5 / 1000000)`
	tokenUnit := map[string]TaskUsageFieldInfo{"tokens": {Unit: "token"}}
	secondUnit := map[string]TaskUsageFieldInfo{"tokens": {Unit: "second"}}
	tokenProjection, err := TaskDisplayProjectionFor(expression, tokenUnit)
	require.NoError(t, err)
	secondProjection, err := TaskDisplayProjectionFor(expression, secondUnit)
	require.NoError(t, err)
	requireTaskExact(t, tokenProjection)
	requireTaskExact(t, secondProjection)
	assert.InEpsilon(t, 5.0, tokenProjection.Tiers[0].UnitPrices["tokens"], 1e-12)
	assert.InEpsilon(t, 5e-06, secondProjection.Tiers[0].UnitPrices["tokens"], 1e-12)
}

func TestBuildTaskDisplayProjectionConditionalMultiplierScenarios(t *testing.T) {
	expression := `(u("generate_audio") ? 1.5 : 1) * tier("base", u("duration_seconds") * 0.2 + 0.01)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Rules, 1)
	assert.Equal(t, "usage", projection.Rules[0].Source)
	assert.Equal(t, "generate_audio", projection.Rules[0].Path)
	assert.InDelta(t, 1.5, projection.Rules[0].Multiplier, 1e-12)
	assert.InDelta(t, 1.0, projection.Rules[0].Fallback, 1e-12)
	require.Len(t, projection.Scenarios, 2)
	// matched=false 使用 fallback 1。
	assert.InDelta(t, 0.2, projection.Scenarios[0].Tiers[0].UnitPrices["duration_seconds"], 1e-12)
	assert.InDelta(t, 0.01, projection.Scenarios[0].Tiers[0].Constant, 1e-12)
	// matched=true 使用 multiplier 1.5,固定项同步缩放。
	assert.InDelta(t, 0.3, projection.Scenarios[1].Tiers[0].UnitPrices["duration_seconds"], 1e-12)
	assert.InDelta(t, 0.015, projection.Scenarios[1].Tiers[0].Constant, 1e-12)
}

func TestBuildTaskDisplayProjectionTopLevelConstantIsUSD(t *testing.T) {
	expression := `tier("base", u("duration_seconds") * 0.2) + 0.05`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 1)
	assert.InDelta(t, 0.2, projection.Tiers[0].UnitPrices["duration_seconds"], 1e-12)
	require.NotNil(t, projection.ConstantCharge)
	// 顶层固定项是 USD,不再做百万换算。
	assert.InDelta(t, 0.05, *projection.ConstantCharge, 1e-12)
}

func TestBuildTaskDisplayProjectionRejectsNonUsageMultiplierCondition(t *testing.T) {
	// 条件倍率引用 param():不可证明,整体 opaque,不输出局部价格。
	expression := `(param("premium") == "true" ? 1.5 : 1) * tier("base", u("duration_seconds") * 0.2)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusOpaque, projection.Status)
	assert.Empty(t, projection.Tiers)
}

func TestBuildTaskDisplayProjectionConditionalUnitPriceInsideTier(t *testing.T) {
	// tier() 正文内按分辨率选择单价:这是线上 seedance-2-5-s 类公式的形状,
	// 之前 collectLinear 只接受固定系数而整体不可展开。
	expression := `u("has_video_input") ? tier("base", u("resolution") == "1080p" ? u("tokens") * 6.762 / 1000000 : u("tokens") * 6.174 / 1000000) : tier("base", u("resolution") == "1080p" ? u("tokens") * 11.319 / 1000000 : u("tokens") * 10.29 / 1000000)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	requireTaskExact(t, projection)
	require.Len(t, projection.Tiers, 4)

	// 极性感知的谓词收集:not 子树反转内部叶子的极性,四象限互不冲突。
	seen := map[string]float64{}
	got := map[[2]bool]float64{}
	for _, tier := range projection.Tiers {
		require.NotContains(t, seen, tier.ConditionText, "duplicate branch")
		seen[tier.ConditionText] = tier.UnitPrices["tokens"]
		preds := collectPolarityPreds(t, tier.Condition, false)
		require.Len(t, preds, 2)
		key := [2]bool{}
		for _, pred := range preds {
			switch pred.path {
			case "has_video_input":
				key[0] = (pred.value == "true") != pred.neg
			case "resolution":
				key[1] = (pred.value == "1080p") != pred.neg
			}
		}
		got[key] = tier.UnitPrices["tokens"]
	}
	require.Len(t, got, 4)
	assert.InEpsilon(t, 6.762, got[[2]bool{true, true}], 1e-12)
	assert.InEpsilon(t, 6.174, got[[2]bool{true, false}], 1e-12)
	assert.InEpsilon(t, 11.319, got[[2]bool{false, true}], 1e-12)
	assert.InEpsilon(t, 10.29, got[[2]bool{false, false}], 1e-12)

	// 每个分支的展示单价与原生引擎金额等价。
	total, _, err := RunExprWithRequest(expression, TokenParams{}, RequestInput{Usage: map[string]any{
		"tokens": float64(1_000_000), "has_video_input": true, "resolution": "4k",
	}})
	require.NoError(t, err)
	assert.InDelta(t, 6.174, total, 1e-9)
}

func TestBuildTaskDisplayProjectionDepthLimitIsNotTruncated(t *testing.T) {
	expression := `u("has_video_input") ? (u("resolution") == "4k" ? (u("generate_audio") ? (u("resolution") == "1080p" ? tier("a", u("tokens") * 1 / 1000000) : tier("b", u("tokens") * 1 / 1000000)) : tier("c", u("tokens") * 1 / 1000000)) : tier("d", u("tokens") * 1 / 1000000)) : tier("e", u("tokens") * 1 / 1000000)`
	projection, err := BuildTaskDisplayProjection(expression, taskDisplayTestFields)
	require.NoError(t, err)
	// 深度可控的嵌套仍完整展开,不截断。
	requireTaskExact(t, projection)
	assert.Len(t, projection.Tiers, 5)
}
