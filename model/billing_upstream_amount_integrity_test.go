package model

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamMixedTestsKeepKnownSubtotalButBlockCompleteTotals(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 93, BaseURL: urlPtr("https://mixed.example")}).Error)
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: 1000, ChannelId: 93, Discount: decimal.RequireFromString("0.5"), Reason: "test"}, 0, 1))
	require.NoError(t, db.Create(&[]Log{
		{CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 93, ModelName: "priced", TokenName: "模型测试", Content: "模型测试", Quota: 101, Other: `{"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":101}}`},
		{CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 93, ModelName: "pending", TokenName: "模型测试", Content: "模型测试", Quota: 245, PromptTokens: 89, CompletionTokens: 16, Other: `{"model_ratio":1.45,"completion_ratio":4.996551724138,"cache_tokens":88,"cache_ratio":0.1}`},
	}).Error)
	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	assert.Nil(t, group.OriginalAmount)
	assert.Nil(t, group.ReferenceAmount)
	assert.False(t, group.ReferenceKnown)
	require.NotNil(t, group.KnownOriginalAmount)
	assert.EqualValues(t, 101, *group.KnownOriginalAmount)
	require.NotNil(t, group.KnownReferenceAmount)
	assert.EqualValues(t, 51, *group.KnownReferenceAmount)
	require.Len(t, group.Channels, 1)
	channel := group.Channels[0]
	assert.Nil(t, channel.OriginalAmount)
	assert.Nil(t, channel.ReferenceAmount)
	assert.Equal(t, group.KnownOriginalAmount, channel.KnownOriginalAmount)
	assert.Equal(t, group.KnownReferenceAmount, channel.KnownReferenceAmount)
	assert.EqualValues(t, 1, channel.DataQuality.TestPricedRows)
	assert.EqualValues(t, 1, channel.DataQuality.UsageWithoutAmountRows)
	var known int64
	var pending int
	err = ScanUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1500}, false, BillingStatementReadPolicy{}, func(row UpstreamBillingDetailItem) error {
		require.NotNil(t, row.TestPricing)
		if row.OriginalAmount == nil {
			pending++
			assert.Equal(t, "pending", row.TestPricing.Status)
		} else {
			known += *row.OriginalAmount
			assert.Equal(t, "priced", row.TestPricing.Status)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, *group.KnownOriginalAmount, known, "detail/export scanner and summary use the same originals")
	assert.Equal(t, 1, pending)
}

func TestUpstreamVersionedZeroTestIsKnownAtEveryLevel(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 21, BaseURL: urlPtr("https://zero.example")}).Error)
	require.NoError(t, db.Create(&Log{CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 21, ModelName: "free", TokenName: "模型测试", Content: "模型测试", Other: `{"test_pricing":{"version":1,"mode":"fixed_price","status":"settled","original_quota":0}}`}).Error)
	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	require.NotNil(t, group.OriginalAmount)
	assert.Zero(t, *group.OriginalAmount)
	assert.True(t, group.ReferenceKnown)
	require.Len(t, group.Channels, 1)
	require.NotNil(t, group.Channels[0].OriginalAmount)
	assert.Zero(t, *group.Channels[0].OriginalAmount)
	require.Len(t, group.Channels[0].Models, 1)
	require.NotNil(t, group.Channels[0].Models[0].OriginalAmount)
	assert.Zero(t, *group.Channels[0].Models[0].OriginalAmount)
}

func TestUpstreamMalformedExplicitPricingNeverFallsBackToPrice(t *testing.T) {
	for _, evidence := range []string{`null`, `"broken"`, `{"version":1,"mode":"ratio","status":"settled"}`, `{"version":1,"mode":"unknown","status":"settled","original_quota":500000}`} {
		log := testLogForAmount(500000)
		log.Other = `{"model_price":1,"test_pricing":` + evidence + `}`
		parsed := parseBillingReconciliationLog(log)
		amount := upstreamTestAmountFor(log, parsed)
		assert.True(t, amount.pending)
		assert.Equal(t, "pending", upstreamTestPricingProjection(parsed, amount).Status)
	}
}
