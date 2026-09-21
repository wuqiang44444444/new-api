package controller

import (
	"github.com/QuantumNous/new-api/service"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTestLogsPreserveCacheUsage(t *testing.T) {
	reported, omitted := true, false
	for _, test := range []struct {
		name     string
		details  dto.InputTokenDetails
		fiveMin  int
		oneHour  int
		semantic string
		want     int
		reported *bool
	}{
		{name: "image cache explicitly zero", reported: &reported},
		{name: "image cache omitted", reported: &omitted},
		{name: "no cache writes", details: dto.InputTokenDetails{CachedTokens: 20}},
		{name: "native OpenAI write", details: dto.InputTokenDetails{CachedTokens: 20, CacheWriteTokens: 30}, want: 30},
		{name: "converted cache total", details: dto.InputTokenDetails{CachedTokens: 20, CachedCreationTokens: 40}, want: 40},
		{name: "duplicate total fields", details: dto.InputTokenDetails{CachedTokens: 20, CachedCreationTokens: 40, CacheWriteTokens: 40}, want: 40},
		{name: "Anthropic TTL split", details: dto.InputTokenDetails{CachedTokens: 20, CachedCreationTokens: 70}, fiveMin: 30, oneHour: 40, semantic: "anthropic", want: 70},
		{name: "split only", details: dto.InputTokenDetails{CachedTokens: 20}, fiveMin: 30, oneHour: 40, semantic: "anthropic", want: 70},
		{name: "total includes unsplit tokens", details: dto.InputTokenDetails{CachedTokens: 20, CachedCreationTokens: 90}, fiveMin: 30, oneHour: 40, semantic: "anthropic", want: 90},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			usage := &dto.Usage{
				CacheReadTokensReported:  test.reported,
				CacheWriteTokensReported: test.reported,
				PromptTokensDetails:      test.details, UsageSemantic: test.semantic,
				ClaudeCacheCreation5mTokens: test.fiveMin, ClaudeCacheCreation1hTokens: test.oneHour,
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			other := buildTestLogOther(ctx, info, types.PriceData{}, usage, nil)
			var fields map[string]any
			require.NoError(t, common.UnmarshalJsonStr(other.JSONString(), &fields))
			assert.EqualValues(t, test.details.CachedTokens, fields["cache_tokens"])
			if test.reported == nil {
				assert.NotContains(t, fields, "cache_read_tokens_reported")
				assert.NotContains(t, fields, "cache_write_tokens_reported")
			} else {
				assert.Equal(t, *test.reported, fields["cache_read_tokens_reported"])
				assert.Equal(t, *test.reported, fields["cache_write_tokens_reported"])
			}
			assert.EqualValues(t, test.want, fields["cache_write_tokens"])
			assert.EqualValues(t, test.fiveMin, fields["cache_creation_tokens_5m"])
			assert.EqualValues(t, test.oneHour, fields["cache_creation_tokens_1h"])
			if test.semantic != "" {
				assert.Equal(t, test.semantic, fields["usage_semantic"])
			}
		})
	}
}

func TestChannelTestCacheWriteCostRemainsExplainableInLog(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr", ExprString: `tier("base", p * 2 + cr * 0.2 + cc * 2.5 + cc1h * 4 + c * 10)`,
			GroupRatio: 1, QuotaPerUnit: common.QuotaPerUnit, ExprVersion: 1,
		},
	}
	usage := &dto.Usage{
		PromptTokens: 2, CompletionTokens: 16, UsageSemantic: "anthropic",
		PromptTokensDetails:         dto.InputTokenDetails{CachedCreationTokens: 1344},
		ClaudeCacheCreation5mTokens: 1344,
	}
	pricing := service.CalculateChannelTestQuota(ctx, info, types.PriceData{}, usage)
	quota, result := pricing.Quota, pricing.Result
	require.NotNil(t, result)
	assert.Equal(t, 1762, quota)
	fields := buildTestLogOther(ctx, info, types.PriceData{}, usage, result).Snapshot()
	assert.EqualValues(t, 1344, fields["cache_write_tokens"])
	assert.EqualValues(t, 1344, fields["cache_creation_tokens_5m"])
	assert.Equal(t, "tiered_expr", fields["billing_mode"])
}

func TestAppendChannelTestPricingRecordsEvidence(t *testing.T) {
	newInfo := func(expr bool) *relaycommon.RelayInfo {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
		if expr {
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprVersion: 4}
		}
		return info
	}
	for _, tc := range []struct {
		name      string
		expr      bool
		result    *billingexpr.TieredResult
		usePrice  bool
		estimated bool
		mode      string
		status    string
		original  int64
		hasOrigin bool
		exprVer   int64
	}{
		{name: "ratio settled keeps recorded fee", mode: "ratio", status: "settled", original: 300, hasOrigin: true},
		{name: "ratio estimated usage has no original", estimated: true, mode: "ratio", status: "estimated"},
		{name: "fixed price ignores estimated usage", usePrice: true, estimated: true, mode: "fixed_price", status: "settled", original: 250000, hasOrigin: true},
		{name: "expression success keeps pre-discount original", expr: true, result: &billingexpr.TieredResult{ActualQuotaBeforeGroup: 12.6}, mode: "tiered_expr", status: "settled", original: 13, hasOrigin: true, exprVer: 4},
		{name: "expression success with estimated usage stays estimated", expr: true, estimated: true, result: &billingexpr.TieredResult{ActualQuotaBeforeGroup: 12.6}, mode: "tiered_expr", status: "estimated", exprVer: 4},
		{name: "expression fallback stays estimated", expr: true, mode: "tiered_expr", status: "estimated", exprVer: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := model.NewLogOther()
			appendChannelTestPricing(other, newInfo(tc.expr), int(tc.original), tc.result, types.PriceData{UsePrice: tc.usePrice, ModelPrice: 0.5}, tc.estimated)
			var fields map[string]any
			require.NoError(t, common.UnmarshalJsonStr(other.JSONString(), &fields))
			record, ok := fields["test_pricing"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.mode, record["mode"])
			assert.Equal(t, tc.status, record["status"])
			if tc.hasOrigin {
				assert.EqualValues(t, tc.original, record["original_quota"])
				if tc.expr {
					assert.EqualValues(t, tc.exprVer, record["expr_version"])
				}
			} else {
				assert.NotContains(t, record, "original_quota", "estimated fees must not pretend a settled original")
			}
		})
	}
}

func TestChannelTestUsageEvidenceDistinguishesActualFromEstimated(t *testing.T) {
	actual := dto.NewOpenAIChatBillingUsage(&dto.Usage{PromptTokens: 100, CompletionTokens: 10})
	estimated := dto.NewOpenAIChatBillingUsage(&dto.Usage{PromptTokens: 100, CompletionTokens: 10})
	estimated.Estimated = true
	for _, tc := range []struct {
		name   string
		usage  any
		local  bool
		actual bool
	}{
		{name: "absent usage"},
		{name: "typed nil", usage: (*dto.Usage)(nil)},
		{name: "local count in plain usage", usage: &dto.Usage{PromptTokens: 100}, local: true},
		{name: "upstream plain usage", usage: dto.Usage{PromptTokens: 100}, actual: true},
		{name: "canonical estimated usage", usage: &dto.Usage{BillingUsage: estimated}},
		{name: "canonical actual usage replaces local estimate", usage: &dto.Usage{BillingUsage: actual}, local: true, actual: true},
		{name: "invalid canonical evidence", usage: &dto.Usage{BillingUsage: &dto.BillingUsage{Semantic: "unknown"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.actual, isTestUsageValue(tc.usage, tc.local))
		})
	}
}
