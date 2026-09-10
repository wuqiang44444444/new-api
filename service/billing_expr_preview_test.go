package service

import (
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 管理员只读试算回归：用户原始 / 6.71 表达式无需改写即可试算；分时条件
// 真实参与执行；模拟时刻不受测试运行当天影响；OpenAI/Anthropic 用量归一
// 与后端结算一致；quota 为服务器统一取整。

const flashPreviewExpr = `tier("base", p * 1.5 + cr * 0.05 + c * 4.5) * (
  weekday("Asia/Shanghai") >= 1 &&
  weekday("Asia/Shanghai") <= 5 &&
  (
    (hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) ||
    (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18)
  )
  ? 2 : 1
) / 6.71`

func flashPreviewSample(pricingTime string) *BillingExprPreviewSample {
	// OpenAI 总输入语义：prompt_tokens 已包含 200k 缓存读取。
	return &BillingExprPreviewSample{
		UsageSemantic:    "openai",
		PromptTokens:     1_000_000,
		CompletionTokens: 100_000,
		CacheReadTokens:  200_000,
		PricingTime:      pricingTime,
	}
}

func TestPreviewBillingExpressionIdleAndPeak(t *testing.T) {
	// 2026-08-31 是周一：07:59 空闲，10:00 高峰。
	idleResult := mustEvaluatePreview(t, flashPreviewExpr, flashPreviewSample("2026-08-31T07:59:59+08:00"))
	peakResult := mustEvaluatePreview(t, flashPreviewExpr, flashPreviewSample("2026-08-31T10:00:00+08:00"))
	idle, peak := idleResult.Evaluation, peakResult.Evaluation

	assert.Equal(t, "base", idle.MatchedTier)
	assert.InDelta(t, 1.66/6.71, idle.RawCostUSD, 1e-6)
	assert.InDelta(t, 2*idle.RawCostUSD, peak.RawCostUSD, 1e-6)
	assert.False(t, idle.Saturated)

	// quota 与服务器统一换算一致（GroupRatio=1）。
	expectedQuota := common.QuotaRound(idle.RawCostUSD * common.QuotaPerUnit)
	assert.Equal(t, expectedQuota, idle.Quota)

	// 投影单价与引擎输出逐项对照：同一 rawCost 必须能由投影单价复现。
	require.NotNil(t, idleResult.Projection)
	require.Equal(t, billingexpr.DisplayStatusExact, idleResult.Projection.Status)
	tier := idleResult.Projection.Tiers[0]
	reproduced := tier.UnitPrices["p"]*800_000 + tier.UnitPrices["cr"]*200_000 + tier.UnitPrices["c"]*100_000
	assert.InDelta(t, idle.RawCostUSD, reproduced/1_000_000, 1e-6)

	// 空闲时倍率规则未命中；高峰时命中且倍率为 2。
	assert.False(t, findPreviewRuleMatched(idle))
	assert.True(t, findPreviewRuleMatched(peak))
}

func findPreviewRuleMatched(evaluation *BillingExprPreviewEvaluation) bool {
	for _, rule := range evaluation.RequestRules {
		if rule.Matched {
			return true
		}
	}
	return false
}

func TestPreviewBillingExpressionTimeBoundaries(t *testing.T) {
	cases := []struct {
		time string
		peak bool
	}{
		{"2026-08-31T08:59:59+08:00", false},
		{"2026-08-31T09:00:00+08:00", true},
		{"2026-08-31T11:59:59+08:00", true},
		{"2026-08-31T12:00:00+08:00", false},
		{"2026-08-31T13:59:59+08:00", false},
		{"2026-08-31T14:00:00+08:00", true},
		{"2026-08-31T17:59:59+08:00", true},
		{"2026-08-31T18:00:00+08:00", false},
		{"2026-09-05T10:00:00+08:00", false}, // 周六
		{"2026-09-06T10:00:00+08:00", false}, // 周日
	}
	var idleCost, peakCost float64
	for _, tt := range cases {
		evaluation := mustEvaluatePreview(t, flashPreviewExpr, flashPreviewSample(tt.time)).Evaluation
		if tt.peak {
			if peakCost == 0 {
				peakCost = evaluation.RawCostUSD
			}
			assert.InDelta(t, peakCost, evaluation.RawCostUSD, 1e-9, tt.time)
		} else {
			if idleCost == 0 {
				idleCost = evaluation.RawCostUSD
			}
			assert.InDelta(t, idleCost, evaluation.RawCostUSD, 1e-9, tt.time)
		}
	}
	assert.Greater(t, peakCost, idleCost)
}

func TestPreviewBillingExpressionAnthropicSemanticNormalization(t *testing.T) {
	// Anthropic 语义：prompt 为纯文本输入，缓存另计；len = 文本 + 缓存。
	sample := &BillingExprPreviewSample{
		UsageSemantic:    "anthropic",
		PromptTokens:     1_000_000,
		CompletionTokens: 100_000,
		CacheReadTokens:  200_000,
		PricingTime:      "2026-08-31T07:59:59+08:00",
	}
	evaluation := mustEvaluatePreview(t, flashPreviewExpr, sample).Evaluation
	assert.Equal(t, float64(1_000_000), evaluation.Normalized.P)
	assert.Equal(t, float64(200_000), evaluation.Normalized.CR)
	assert.Equal(t, float64(1_200_000), evaluation.Normalized.Len)
	// Anthropic 语义下 p 不扣除缓存：body = 1,000,000*1.5 + 200,000*0.05 + 100,000*4.5。
	assert.InDelta(t, 1.96/6.71, evaluation.RawCostUSD, 1e-6)
}

func TestPreviewBillingExpressionProjectionWithoutSample(t *testing.T) {
	results := PreviewBillingExpressions([]BillingExprPreviewItem{
		{Key: "pro", Expression: `tier("base", p * 4.5 + cr * 0.15 + c * 13.5) / 6.71`},
	})
	require.Len(t, results, 1)
	require.Empty(t, results[0].Error)
	require.NotNil(t, results[0].Projection)
	assert.Equal(t, billingexpr.DisplayStatusExact, results[0].Projection.Status)
	assert.Nil(t, results[0].Evaluation)
	assert.InDelta(t, 4.5/6.71, results[0].Projection.Tiers[0].UnitPrices["p"], 1e-15)
}

func TestPreviewBillingExpressionValidButOpaqueStillEvaluates(t *testing.T) {
	// 合法但不可展开的表达式：评估仍可用，投影不得给出伪单价。
	results := PreviewBillingExpressions([]BillingExprPreviewItem{
		{Key: "square", Expression: `tier("base", p * p)`, Sample: flashPreviewSample("2026-08-31T10:00:00+08:00")},
	})
	require.Len(t, results, 1)
	require.Empty(t, results[0].Error)
	require.NotNil(t, results[0].Projection)
	assert.Equal(t, billingexpr.DisplayStatusOpaque, results[0].Projection.Status)
	assert.Equal(t, billingexpr.DisplayReasonNonlinearPricing, results[0].Projection.Reason)
	require.NotNil(t, results[0].Evaluation)
	assert.Greater(t, results[0].Evaluation.RawCostUSD, float64(0))
}

func TestPreviewBillingExpressionRejectsTaskUsageAndBadInput(t *testing.T) {
	results := PreviewBillingExpressions([]BillingExprPreviewItem{
		{Key: "task", Expression: `tier("base", u("seconds") * 0.4)`},
		{Key: "syntax", Expression: `tier("base", p * )`},
		{Key: "bad-time", Expression: `tier("base", p * 1)`, Sample: &BillingExprPreviewSample{PricingTime: "not-a-time"}},
		{Key: "bad-semantic", Expression: `tier("base", p * 1)`, Sample: &BillingExprPreviewSample{UsageSemantic: "gemini"}},
	})
	require.Len(t, results, 4)
	for _, result := range results {
		require.NotEmpty(t, result.Error, result.Key)
	}
	// 任务表达式单位合同不同，明确拒绝而不是猜测换算。
	assert.Contains(t, results[0].Error, "task usage")
}

func mustEvaluatePreview(t *testing.T, expression string, sample *BillingExprPreviewSample) *BillingExprPreviewItemResult {
	t.Helper()
	results := PreviewBillingExpressions([]BillingExprPreviewItem{
		{Key: "item", Expression: expression, Sample: sample},
	})
	require.Len(t, results, 1)
	require.Empty(t, results[0].Error, "preview item %q failed", expression)
	require.NotNil(t, results[0].Evaluation)
	// 返回的模拟时刻必须与请求一致，不能静默改为当前时间。
	if sample.PricingTime != "" {
		requested, err := time.Parse(time.RFC3339, sample.PricingTime)
		require.NoError(t, err)
		returned, err := time.Parse(time.RFC3339, results[0].Evaluation.PricingTime)
		require.NoError(t, err)
		assert.True(t, requested.Equal(returned), "pricing time must round-trip")
	}
	assert.Equal(t, sample.UsageSemantic, results[0].Evaluation.UsageSemantic)
	assert.False(t, math.IsNaN(results[0].Evaluation.RawCostUSD))
	return &results[0]
}
