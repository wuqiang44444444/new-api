package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalGeminiTestsDoNotRequireUnusedCacheWrites(t *testing.T) {
	for _, tc := range []struct {
		name, semantic string
		write          any
		known          bool
		amount         int64
	}{
		{name: "frozen Gemini conversion", known: true, amount: 200},
		{name: "explicit Gemini semantic", semantic: "gemini", known: true, amount: 200},
		{name: "explicit zero", write: 0, known: true, amount: 200},
		{name: "malformed write remains unknown", write: "bad"},
		{name: "positive write needs its own price", write: 10},
		{name: "OpenAI cache creation cannot be guessed", semantic: "openai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := map[string]any{"model_ratio": 1, "completion_ratio": 2, "cache_ratio": 0.1, "model_price": -1, "group_ratio": 1, "cache_tokens": 0, "request_path": "/v1/chat/completions", "request_conversion": []string{"OpenAI Compatible", "Google Gemini"}}
			if tc.semantic != "" {
				facts["usage_semantic"] = tc.semantic
			}
			if tc.write != nil {
				facts["cache_write_tokens"] = tc.write
			}
			raw, err := common.Marshal(facts)
			require.NoError(t, err)
			log := testLogForAmount(200)
			log.Other = string(raw)
			amount := upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
			assert.Equal(t, tc.known, amount.known)
			if tc.known {
				assert.Equal(t, tc.amount, amount.original.IntPart())
			}
		})
	}
}

func TestHistoricalExpressionOnlyRequiresRelevantUsageSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, expr, semantic string
		known                bool
	}{
		{"plain tokens without semantic", `tier("base",p*2+c*10)`, "", true},
		{"unused Claude cache TTL", `tier("base",c*14)`, "anthropic", true},
		{"used Claude cache TTL", `tier("base",p*2+cc*2+c*10)`, "anthropic", false},
		{"cache subtraction requires semantic", `tier("base",p*2+cr+c*10)`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := map[string]any{"billing_mode": "tiered_expr", "expr_b64": base64.StdEncoding.EncodeToString([]byte(tc.expr)), "group_ratio": 0.5, "cache_creation_tokens": 10}
			if tc.semantic != "" {
				facts["usage_semantic"] = tc.semantic
			}
			raw, err := common.Marshal(facts)
			require.NoError(t, err)
			log := testLogForAmount(175)
			log.Other = string(raw)
			amount := upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
			assert.Equal(t, tc.known, amount.known)
			if tc.known {
				assert.EqualValues(t, 350, amount.original.IntPart())
			}
		})
	}
}

func TestGeminiTestAmountAndCacheCoverageMatchSummaryAndDetails(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 103, BaseURL: urlPtr("https://example.invalid")}).Error)
	require.NoError(t, db.Create(&Log{Type: LogTypeConsume, ChannelId: 103, CreatedAt: 1100, TokenName: "模型测试", Content: "模型测试", ModelName: "test", PromptTokens: 100, CompletionTokens: 50, Quota: 200, Other: `{"model_ratio":1,"completion_ratio":2,"cache_ratio":0.1,"group_ratio":1,"cache_tokens":0,"request_path":"/v1/chat/completions","request_conversion":["OpenAI Compatible","Google Gemini"]}`}).Error)
	summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	require.NotNil(t, group.OriginalAmount)
	assert.EqualValues(t, 200, *group.OriginalAmount)
	assert.Zero(t, group.DataQuality.CacheWriteUnavailableRequests)
	assert.Zero(t, group.DataQuality.UsageWithoutAmountRows)
	assert.EqualValues(t, 1, group.DataQuality.TestPricedRows)
	detail, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, 1, 10, false)
	require.NoError(t, err)
	require.Len(t, detail.Items, 1)
	require.NotNil(t, detail.Items[0].OriginalAmount)
	assert.Equal(t, *group.OriginalAmount, *detail.Items[0].OriginalAmount)
	assert.Zero(t, detail.Items[0].DataQuality.CacheWriteUnavailableRequests)
}

func TestHistoricalEmbeddingTestUsesItsInputOnlyBillingFacts(t *testing.T) {
	log := testLogForAmount(100)
	log.CompletionTokens = 0
	log.Other = `{"model_ratio":1,"completion_ratio":1,"model_price":-1,"group_ratio":1,"cache_tokens":0,"request_path":"/v1/embeddings","request_conversion":["embedding"]}`
	parsed := parseBillingReconciliationLog(log)
	amount := upstreamTestAmountFor(log, parsed)
	require.True(t, amount.known)
	assert.EqualValues(t, 100, amount.original.IntPart())
	assert.True(t, parsed.cacheWrite.known)
}

func TestHistoricalClaudeContextLengthDoesNotRequireUnusedTTLBreakdown(t *testing.T) {
	log := testLogForAmount(30)
	facts := map[string]any{"billing_mode": "tiered_expr", "expr_b64": base64.StdEncoding.EncodeToString([]byte(`tier("base",len)`)), "group_ratio": 0.5, "usage_semantic": "anthropic", "cache_tokens": 10, "cache_creation_tokens": 10}
	raw, err := common.Marshal(facts)
	require.NoError(t, err)
	log.Other = string(raw)
	amount := upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
	require.True(t, amount.known, amount.reason)
	assert.EqualValues(t, 60, amount.original.IntPart())
}

func TestGeminiTestAmountFlowsToDailyAndWeeklyViews(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 103, BaseURL: urlPtr("https://example.invalid")}).Error)
	day, err := ResolveUsageAnalyticsPeriod("day", "2026-09-10", 0)
	require.NoError(t, err)
	require.NoError(t, db.Create(&Log{Type: LogTypeConsume, ChannelId: 103, CreatedAt: day.StartTimestamp + 100, TokenName: "模型测试", Content: "模型测试", ModelName: "test", PromptTokens: 100, CompletionTokens: 50, Quota: 200, Other: `{"model_ratio":1,"completion_ratio":2,"cache_ratio":0.1,"group_ratio":1,"cache_tokens":0,"request_path":"/v1/chat/completions","request_conversion":["OpenAI Compatible","Google Gemini"]}`}).Error)
	for _, cycle := range []string{"day", "week"} {
		period, err := ResolveUsageAnalyticsPeriod(cycle, "2026-09-10", 0)
		require.NoError(t, err)
		view, err := GetUsageUpstreamView(context.Background(), period)
		require.NoError(t, err)
		require.NotNil(t, view.Total.OriginalQuotaEstimate)
		assert.EqualValues(t, 200, *view.Total.OriginalQuotaEstimate)
		assert.Zero(t, view.Total.UsageOnlyRows)
	}
}
