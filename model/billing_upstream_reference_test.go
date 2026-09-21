package model

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Core example from the plan: channel A original 1000 at 0.8, channel B
// original 2000 at 0.9, channel C 500 with the default coefficient 1.
// The URL original total is 3500 and the reference total is 3100.
func TestUpstreamReferenceAmountCoreExample(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july := channelDiscountPeriod(time.July, 2026)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 71, Name: "alpha", BaseURL: urlPtr("https://core.example.com")},
		{Id: 72, Name: "beta", BaseURL: urlPtr("https://core.example.com/")},
		{Id: 73, Name: "gamma", BaseURL: urlPtr("https://core.example.com")},
	}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 71, ModelName: "shared", PromptTokens: 10, Quota: 1000, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 72, ModelName: "shared", PromptTokens: 20, Quota: 2000, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1102, Type: LogTypeConsume, ChannelId: 73, ModelName: "shared", PromptTokens: 5, Quota: 500, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	}).Error)
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: july, ChannelId: 71, Discount: decimal.RequireFromString("0.8"), Reason: "contract a"}, 0, 9))
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: july, ChannelId: 72, Discount: decimal.RequireFromString("0.9"), Reason: "contract b"}, 0, 9))

	summary, err := GetProviderBillingURLSummary(1000, 1500, july, "https://core.example.com")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	require.NotNil(t, group.OriginalAmount)
	assert.EqualValues(t, 3500, *group.OriginalAmount)
	require.NotNil(t, group.ReferenceAmount)
	assert.EqualValues(t, 3100, *group.ReferenceAmount)
	assert.True(t, group.ReferenceKnown)
	assert.Zero(t, group.DiscountPendingChannels)
	require.Len(t, group.Channels, 3)
	byChannel := make(map[int]ProviderURLChannelGroupSummary)
	for _, row := range group.Channels {
		byChannel[row.ChannelId] = row
	}
	require.NotNil(t, byChannel[71].Discount)
	assert.True(t, byChannel[71].Discount.Value.Equal(decimal.RequireFromString("0.8")))
	require.NotNil(t, byChannel[72].Discount)
	assert.True(t, byChannel[72].Discount.Value.Equal(decimal.RequireFromString("0.9")))
	require.NotNil(t, byChannel[73].Discount)
	assert.True(t, byChannel[73].Discount.Value.Equal(decimal.NewFromInt(1)))
	assert.Equal(t, "default", byChannel[73].Discount.Source)
	assert.Zero(t, byChannel[73].Discount.Version)

	channelAmounts := make(map[int][2]int64)
	for id, row := range byChannel {
		require.NotNil(t, row.OriginalAmount)
		require.Len(t, row.Models, 1)
		if row.ReferenceAmount != nil {
			channelAmounts[id] = [2]int64{*row.OriginalAmount, *row.ReferenceAmount}
		} else {
			channelAmounts[id] = [2]int64{*row.OriginalAmount, -1}
		}
	}
	assert.Equal(t, [2]int64{1000, 800}, channelAmounts[71])
	assert.Equal(t, [2]int64{2000, 1800}, channelAmounts[72])
	assert.Equal(t, [2]int64{500, 500}, channelAmounts[73])

	var count int64
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Where("channel_id = ?", 73).Count(&count).Error)
	assert.Zero(t, count, "a default projection must not create configuration")

	// Saving an explicit 1 keeps the same total and creates the first real version.
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: july, ChannelId: 73, Discount: decimal.NewFromInt(1), Reason: "confirmed no discount"}, 0, 9))
	summary, err = GetProviderBillingURLSummary(1000, 1500, july, "https://core.example.com")
	require.NoError(t, err)
	group = summary.Groups[0]
	require.NotNil(t, group.ReferenceAmount)
	assert.EqualValues(t, 3100, *group.ReferenceAmount)
	assert.True(t, group.ReferenceKnown)
	assert.Equal(t, 0, group.DiscountPendingChannels)

	// Updating A to 0.7 moves only its share: 700+1800+500 = 3000.
	existing := requireSingleChannelDiscount(t, db, july, 71)
	existing.Discount = decimal.RequireFromString("0.7")
	existing.Reason = "renegotiated"
	require.NoError(t, SaveProviderChannelBillingDiscount(&existing, existing.Version, 9))
	summary, err = GetProviderBillingURLSummary(1000, 1500, july, "https://core.example.com")
	require.NoError(t, err)
	group = summary.Groups[0]
	require.NotNil(t, group.ReferenceAmount)
	assert.EqualValues(t, 3000, *group.ReferenceAmount)
}

// A row whose frozen price evidence is incomplete keeps the parent amounts
// incomplete instead of being zero-filled.
func TestUpstreamReferenceAmountStaysIncompleteOnMissingEvidence(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july := channelDiscountPeriod(time.July, 2026)
	require.NoError(t, db.Create(&Channel{Id: 74, Name: "partial", BaseURL: urlPtr("https://partial.example.com")}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 74, ModelName: "model", Quota: 800, Other: `{"group_ratio":0.8,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 74, ModelName: "model", Quota: 500, Other: `{"model_ratio":1}`},
	}).Error)
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: july, ChannelId: 74, Discount: decimal.RequireFromString("0.5"), Reason: "half"}, 0, 9))

	summary, err := GetProviderBillingURLSummary(1000, 1500, july, "https://partial.example.com")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	assert.Nil(t, group.OriginalAmount)
	assert.Nil(t, group.ReferenceAmount)
	assert.False(t, group.ReferenceKnown)
	assert.Equal(t, 0, group.DiscountPendingChannels)
	require.NotNil(t, group.DataQuality)
	assert.EqualValues(t, 1, group.DataQuality.MissingHistoricalPriceRows)
	assert.Contains(t, group.EstimateReasons, BillingEstimateMissingGroup)
}

// A channel whose rows are all native channel tests keeps usage but no
// settlement: pending test amounts still block complete parent totals.
func TestUpstreamReferenceAmountPendingTestOnlyChannelBlocksTotals(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july := channelDiscountPeriod(time.July, 2026)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 76, Name: "real", BaseURL: urlPtr("https://mixed.example.com")},
		{Id: 77, Name: "tests-only", BaseURL: urlPtr("https://mixed.example.com/")},
	}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 76, ModelName: "shared", Quota: 1000, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 8, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 77, ModelName: "shared", Quota: 9000, TokenId: 0, TokenName: "模型测试", Content: "模型测试", Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	}).Error)
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: july, ChannelId: 76, Discount: decimal.RequireFromString("0.8"), Reason: "contract"}, 0, 9))

	summary, err := GetProviderBillingURLSummary(1000, 1500, july, "https://mixed.example.com")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	assert.Nil(t, group.OriginalAmount)
	assert.Nil(t, group.ReferenceAmount)
	assert.False(t, group.ReferenceKnown)
	require.NotNil(t, group.KnownOriginalAmount)
	assert.EqualValues(t, 1000, *group.KnownOriginalAmount)
	require.NotNil(t, group.KnownReferenceAmount)
	assert.EqualValues(t, 800, *group.KnownReferenceAmount)
	assert.Equal(t, 0, group.DiscountPendingChannels)
	require.Len(t, group.Channels, 2)
	testsOnly := group.Channels[1]
	assert.Equal(t, 77, testsOnly.ChannelId)
	assert.True(t, testsOnly.UsageOnly)
	assert.Nil(t, testsOnly.OriginalAmount)
	assert.Nil(t, testsOnly.ReferenceAmount)
	real := group.Channels[0]
	require.NotNil(t, real.OriginalAmount)
	assert.EqualValues(t, 1000, *real.OriginalAmount)
	require.NotNil(t, real.ReferenceAmount)
	assert.EqualValues(t, 800, *real.ReferenceAmount)
}

func TestUpstreamAmountsSumRoundedEvidenceRows(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	period := channelDiscountPeriod(time.July, 2026)
	require.NoError(t, db.Create(&[]Channel{{Id: 71, BaseURL: urlPtr("https://round.example")}, {Id: 72, BaseURL: urlPtr("https://round.example")}}).Error)
	for _, id := range []int{71, 72} {
		require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: period, ChannelId: id, Discount: decimal.RequireFromString("0.6"), Reason: "rounding"}, 0, 9))
	}
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 71, ModelName: "a", Quota: 1, Other: `{"contract_applicable":false,"group_ratio":0.8,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 72, ModelName: "a", Quota: 1, Other: `{"contract_applicable":false,"group_ratio":0.8,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1102, Type: LogTypeConsume, ChannelId: 71, ModelName: "b", Quota: 1, Other: `{"contract_applicable":false,"group_ratio":0.8,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1103, Type: LogTypeRefund, ChannelId: 71, ModelName: "b", Quota: 1, Other: `{"contract_applicable":false,"group_ratio":0.8,"model_ratio":1}`},
	}).Error)
	summary, err := GetProviderBillingURLSummary(1000, 1500, period, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	require.NotNil(t, group.OriginalAmount)
	require.NotNil(t, group.ReferenceAmount)
	assert.EqualValues(t, 2, *group.OriginalAmount)
	assert.EqualValues(t, 2, *group.ReferenceAmount)
	var original, reference int64
	for _, channel := range group.Channels {
		var leafOriginal, leafReference int64
		for _, leaf := range channel.Models {
			require.NotNil(t, leaf.OriginalAmount)
			require.NotNil(t, leaf.ReferenceAmount)
			leafOriginal += *leaf.OriginalAmount
			leafReference += *leaf.ReferenceAmount
		}
		require.NotNil(t, channel.OriginalAmount)
		require.NotNil(t, channel.ReferenceAmount)
		assert.Equal(t, leafOriginal, *channel.OriginalAmount)
		assert.Equal(t, leafReference, *channel.ReferenceAmount)
		original += leafOriginal
		reference += leafReference
	}
	assert.Equal(t, original, *group.OriginalAmount)
	assert.Equal(t, reference, *group.ReferenceAmount)
}
