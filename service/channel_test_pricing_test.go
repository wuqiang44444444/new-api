package service

import (
	"context"
	"github.com/QuantumNous/new-api/model"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTestNativePricingAndFrozenOriginal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage dto.Usage
		price hosttypes.PriceData
		expr  string
		want  int
	}{
		{name: "cache discount", usage: dto.Usage{PromptTokens: 89, CompletionTokens: 16, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 88}}, price: hosttypes.PriceData{ModelRatio: 1.45, CompletionRatio: 4.996551724138, CacheRatio: 0.1}, want: 130},
		{name: "cache write premium", usage: dto.Usage{PromptTokens: 100, CompletionTokens: 10, PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 80}}, price: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, CacheCreationRatio: 1.25}, want: 140},
		{name: "expression rounds half away from zero", usage: dto.Usage{PromptTokens: 1}, expr: `tier("base", p * 25.2)`, want: 13},
		{name: "fixed price without token usage", price: hosttypes.PriceData{UsePrice: true, ModelPrice: 0.0000252}, want: 13},
		{name: "explicit free price", usage: dto.Usage{PromptTokens: 100}, price: hosttypes.PriceData{ModelRatio: 0, CompletionRatio: 1}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, group := range []float64{0, 0.5, 3} {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
				price := tc.price
				price.GroupRatioInfo.GroupRatio = group
				if tc.expr != "" {
					info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: tc.expr, GroupRatio: group, QuotaPerUnit: common.QuotaPerUnit, ExprVersion: 1}
				}
				pricing := CalculateChannelTestQuota(ctx, info, price, &tc.usage)
				quota, result, usage := pricing.Quota, pricing.Result, pricing.Usage
				assert.Equal(t, tc.want, quota)
				require.NotNil(t, usage)
				assert.Equal(t, "openai", usage.UsageSemantic)
				assert.Equal(t, "", tc.usage.UsageSemantic, "the original response is not mutated")
				if tc.expr != "" {
					require.NotNil(t, result)
					assert.Equal(t, group, info.TieredBillingSnapshot.GroupRatio, "the original snapshot is not mutated")
				}
			}
		})
	}
}

func TestChannelTestUsesCanonicalUsageAndAuditsSaturation(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	usage := &dto.Usage{PromptTokens: 999, BillingUsage: dto.NewOpenAIChatBillingUsage(&dto.Usage{PromptTokens: 100, CompletionTokens: 10})}
	pricing := CalculateChannelTestQuota(ctx, info, hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2}, usage)
	quota, priced := pricing.Quota, pricing.Usage
	assert.Equal(t, 120, quota)
	assert.Equal(t, 100, priced.PromptTokens)
	assert.Equal(t, 999, usage.PromptTokens)
	quota = CalculateChannelTestQuota(ctx, info, hosttypes.PriceData{ModelRatio: 1e30}, usage).Quota
	assert.Equal(t, common.MaxQuota, quota)
	require.NotNil(t, info.QuotaClamp)
}

func TestHistoricalTestRecalculationMatchesNativeCachePricing(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}))
	require.NoError(t, model.DB.Create(&model.Channel{Id: 9801, Name: "historical pricing"}).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	native := CalculateChannelTestQuota(ctx, info, hosttypes.PriceData{ModelRatio: 2, CompletionRatio: 3, CacheRatio: 0.1}, &dto.Usage{PromptTokens: 100, CompletionTokens: 50, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 40}})
	require.NoError(t, model.DB.Create(&model.Log{Type: model.LogTypeConsume, CreatedAt: 1100, ChannelId: 9801, TokenName: "模型测试", Content: "模型测试", PromptTokens: 100, CompletionTokens: 50, Quota: 500, Other: `{"model_ratio":2,"completion_ratio":3,"cache_ratio":0.1,"cache_tokens":40,"cache_write_tokens":0,"group_ratio":1,"request_path":"/v1/chat/completions","request_conversion":["OpenAI Compatible"]}`}).Error)
	details, err := model.GetUpstreamBillingDetails(context.Background(), model.UpstreamBillingDetailFilter{Start: 1000, End: 1200, ChannelIds: []int{9801}}, 1, 10, false)
	require.NoError(t, err)
	require.Len(t, details.Items, 1)
	require.NotNil(t, details.Items[0].OriginalAmount)
	assert.EqualValues(t, native.Quota, *details.Items[0].OriginalAmount)
	assert.EqualValues(t, 428, *details.Items[0].OriginalAmount)
}
