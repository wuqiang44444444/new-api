package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 只读展示投影回归：用户原始表达式（含 / 6.71 换算与嵌套分时条件）、
// 等价排版变体、非线性与任务表达式必须整体 opaque，绝不输出局部猜价。

const flashExprOriginal = `tier("base", p * 1.5 + cr * 0.05 + c * 4.5) * (
  weekday("Asia/Shanghai") >= 1 &&
  weekday("Asia/Shanghai") <= 5 &&
  (
    (hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) ||
    (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18)
  )
  ? 2 : 1
) / 6.71`

func TestDisplayProjectionOriginalUserExpression(t *testing.T) {
	projection, err := DisplayProjectionFor(flashExprOriginal)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, projection.Status)
	assert.Equal(t, "", projection.Reason)
	assert.Equal(t, DisplayUnitUSDPerMillionTokens, projection.Unit)
	assert.Equal(t, DisplayProjectionVersion, projection.DisplayVersion)
	assert.Equal(t, ExprHashString(flashExprOriginal), projection.ExpressionHash)

	require.Len(t, projection.Tiers, 1)
	tier := projection.Tiers[0]
	assert.Equal(t, "base", tier.Label)
	// 标量乘除传递到整个价格子树：外层 / 6.71 必须参与单价。
	assert.InDelta(t, 1.5/6.71, tier.UnitPrices["p"], 1e-15)
	assert.InDelta(t, 0.05/6.71, tier.UnitPrices["cr"], 1e-15)
	assert.InDelta(t, 4.5/6.71, tier.UnitPrices["c"], 1e-15)

	// 完整布尔层级（&& 左结合）：and(and(weekday>=1, weekday<=5), or(hour 窗口, hour 窗口))。
	require.Len(t, projection.Rules, 1)
	rule := projection.Rules[0]
	assert.Equal(t, 2.0, rule.Multiplier)
	assert.Equal(t, 1.0, rule.Fallback)
	assert.Equal(t, "and", rule.Op)
	require.Len(t, rule.Children, 2)
	weekdayPair := rule.Children[0]
	assert.Equal(t, "and", weekdayPair.Op)
	require.Len(t, weekdayPair.Children, 2)
	weekday := weekdayPair.Children[0]
	assert.Equal(t, "weekday", weekday.TimeFunc)
	assert.Equal(t, "Asia/Shanghai", weekday.Timezone)
	assert.Equal(t, ">=", weekday.CompareOp)
	assert.Equal(t, "1", weekday.Value)
	assert.False(t, weekday.TextOnly)
	assert.Equal(t, "or", rule.Children[1].Op)
	require.Len(t, rule.Children[1].Children, 2)
	hourWindow := rule.Children[1].Children[0]
	assert.Equal(t, "and", hourWindow.Op)
	require.Len(t, hourWindow.Children, 2)
	assert.Equal(t, "hour", hourWindow.Children[0].TimeFunc)
	assert.Equal(t, "Asia/Shanghai", hourWindow.Children[0].Timezone)
	assert.Equal(t, ">=", hourWindow.Children[0].CompareOp)
	assert.Equal(t, "9", hourWindow.Children[0].Value)
	assert.False(t, hourWindow.Children[0].TextOnly)
}

func TestDisplayProjectionLayoutVariantsStayEquivalent(t *testing.T) {
	base, err := DisplayProjectionFor(flashExprOriginal)
	require.NoError(t, err)

	singleLine := `tier("base", p*1.5+cr*0.05+c*4.5)*(weekday("Asia/Shanghai")>=1 && weekday("Asia/Shanghai")<=5 && ((hour("Asia/Shanghai")>=9 && hour("Asia/Shanghai")<12)||(hour("Asia/Shanghai")>=14 && hour("Asia/Shanghai")<18))?2:1)/6.71`
	variant, err := DisplayProjectionFor(singleLine)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, variant.Status)
	require.Len(t, variant.Tiers, len(base.Tiers))
	for name, value := range base.Tiers[0].UnitPrices {
		assert.InDelta(t, value, variant.Tiers[0].UnitPrices[name], 1e-15)
	}
	require.Len(t, variant.Rules, len(base.Rules))

	// 除法移入 tier 内部：等价语义，等价展示。
	innerDivision := `tier("base", (p*1.5 + cr*0.05 + c*4.5)/6.71)`
	inner, err := DisplayProjectionFor(innerDivision)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, inner.Status)
	assert.InDelta(t, 1.5/6.71, inner.Tiers[0].UnitPrices["p"], 1e-15)

	// 外层括号包住整个乘除链。
	wrapped := `(tier("base", p * 1.5 + cr * 0.05 + c * 4.5) / 6.71)`
	wrappedProjection, err := DisplayProjectionFor(wrapped)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, wrappedProjection.Status)
	assert.InDelta(t, 4.5/6.71, wrappedProjection.Tiers[0].UnitPrices["c"], 1e-15)
}

func TestDisplayProjectionLenTiersAndRepeatedVars(t *testing.T) {
	multiTier := `len <= 200000 ? tier("standard", p * 3 + c * 15 + cr * 0.3) : tier("long", p * 6 + c * 22.5 + cr * 0.6)`
	projection, err := DisplayProjectionFor(multiTier)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, projection.Status)
	require.Len(t, projection.Tiers, 2)
	assert.Equal(t, "standard", projection.Tiers[0].Label)
	require.Len(t, projection.Tiers[0].Conditions, 1)
	assert.Equal(t, "len", projection.Tiers[0].Conditions[0].Var)
	assert.Equal(t, "<=", projection.Tiers[0].Conditions[0].Op)
	assert.Equal(t, float64(200000), projection.Tiers[0].Conditions[0].Value)
	assert.Equal(t, "long", projection.Tiers[1].Label)
	assert.Empty(t, projection.Tiers[1].Conditions)

	// 同一变量多次出现必须合并，而不是取第一次匹配。
	merged := `tier("base", p * 2 + p * 3 + c * 4)`
	mergedProjection, err := DisplayProjectionFor(merged)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, mergedProjection.Status)
	assert.InDelta(t, 5.0, mergedProjection.Tiers[0].UnitPrices["p"], 1e-15)
}

func TestDisplayProjectionFixedChargeAndFallbackMultiplier(t *testing.T) {
	expr := `tier("base", p * 2 + c * 3) * (header("x-fast") != "" ? 1.5 : 1) + 0.01`
	projection, err := DisplayProjectionFor(expr)
	require.NoError(t, err)
	assert.Equal(t, DisplayStatusExact, projection.Status)
	require.Len(t, projection.Rules, 1)
	assert.Equal(t, 1.5, projection.Rules[0].Multiplier)
	assert.Equal(t, "header", projection.Rules[0].Source)
	assert.Equal(t, "!=", projection.Rules[0].CompareOp)
	assert.Equal(t, "", projection.Rules[0].Value)
	assert.Equal(t, "x-fast", projection.Rules[0].Path)
	require.NotNil(t, projection.ConstantCharge)
	assert.InDelta(t, 0.01/1_000_000, *projection.ConstantCharge, 1e-15)

	// 非 1 的另一分支不能丢失。
	both := `tier("base", p * 2) * (param("service_tier") == "fast" ? 6 : 1)`
	bothProjection, err := DisplayProjectionFor(both)
	require.NoError(t, err)
	require.Len(t, bothProjection.Rules, 1)
	assert.Equal(t, 6.0, bothProjection.Rules[0].Multiplier)
	assert.Equal(t, 1.0, bothProjection.Rules[0].Fallback)
	assert.Equal(t, "param", bothProjection.Rules[0].Source)
	assert.Equal(t, "==", bothProjection.Rules[0].CompareOp)
	assert.Equal(t, "fast", bothProjection.Rules[0].Value)
}

func TestDisplayProjectionOpaqueCases(t *testing.T) {
	cases := []struct {
		name       string
		expr       string
		wantReason string
	}{
		{"nonlinear p*p", `tier("base", p * p + c * 2)`, DisplayReasonNonlinearPricing},
		{"variable divisor", `tier("base", p * 2 / (c + 1))`, DisplayReasonNonlinearPricing},
		{"unknown function", `tier("base", unknownFn(p) * 2)`, DisplayReasonNonlinearPricing},
		{"task usage expression", `tier("base", u("seconds") * 0.4)`, DisplayReasonTaskUsageExpression},
		{"unrecognized factor", `tier("base", p * 2) * max(p, c)`, DisplayReasonUnrecognizedFactor},
		{"two price subtrees", `tier("base", p * 2) + tier("other", c * 3)`, DisplayReasonUnsupportedShape},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			projection, err := DisplayProjectionFor(tt.expr)
			require.NoError(t, err)
			assert.Equal(t, DisplayStatusOpaque, projection.Status)
			assert.Equal(t, tt.wantReason, projection.Reason)
			assert.Empty(t, projection.Tiers, "opaque projection must not expose partial prices")
		})
	}
}

func TestDisplayProjectionInvalidExpressionReturnsError(t *testing.T) {
	_, err := DisplayProjectionFor(`tier("base", p * )`)
	require.Error(t, err)
	_, err = DisplayProjectionFor("")
	require.Error(t, err)
	_, err = DisplayProjectionFor("   ")
	require.Error(t, err)
}

func TestDisplayProjectionCacheReturnsSameIdentity(t *testing.T) {
	first, err := DisplayProjectionFor(flashExprOriginal)
	require.NoError(t, err)
	second, err := DisplayProjectionFor(flashExprOriginal)
	require.NoError(t, err)
	assert.Same(t, first, second)
}
