package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTestLogsPreserveCacheUsage(t *testing.T) {
	for _, test := range []struct {
		name     string
		details  dto.InputTokenDetails
		fiveMin  int
		oneHour  int
		semantic string
		want     int
	}{
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
				PromptTokensDetails: test.details, UsageSemantic: test.semantic,
				ClaudeCacheCreation5mTokens: test.fiveMin, ClaudeCacheCreation1hTokens: test.oneHour,
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
			other := buildTestLogOther(ctx, info, types.PriceData{}, usage, nil)
			var fields map[string]any
			require.NoError(t, common.UnmarshalJsonStr(other.JSONString(), &fields))
			assert.EqualValues(t, test.details.CachedTokens, fields["cache_tokens"])
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
	quota, result := settleTestQuota(info, types.PriceData{}, usage)
	require.NotNil(t, result)
	assert.Equal(t, 1762, quota)
	fields := buildTestLogOther(ctx, info, types.PriceData{}, usage, result).Snapshot()
	assert.EqualValues(t, 1344, fields["cache_write_tokens"])
	assert.EqualValues(t, 1344, fields["cache_creation_tokens_5m"])
	assert.Equal(t, "tiered_expr", fields["billing_mode"])
}
