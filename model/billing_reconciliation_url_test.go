package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBillingURLGroupKeyEquivalenceRules(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "trailing slash equals bare host", rawURL: "https://api.example.com/", want: "https://api.example.com"},
		{name: "scheme and host case fold", rawURL: "HTTPS://API.EXAMPLE.COM:443", want: "https://api.example.com"},
		{name: "bare host equals itself", rawURL: "https://api.example.com", want: "https://api.example.com"},
		{name: "path trailing slash is stripped", rawURL: "https://api.example.com/v1/", want: "https://api.example.com/v1"},
		{name: "different paths stay separate", rawURL: "https://api.example.com/v2", want: "https://api.example.com/v2"},
		{name: "encoded slash is not a path separator", rawURL: "https://api.example.com/team%2Fprod/", want: "https://api.example.com/team%2Fprod"},
		{name: "encoded trailing slash remains part of identity", rawURL: "https://api.example.com/team%2F", want: "https://api.example.com/team%2F"},
		{name: "IPv6 default port retains brackets", rawURL: "https://[::1]:443/", want: "https://[::1]"},
		{name: "non-default port stays in the identity", rawURL: "https://api.example.com:8443", want: "https://api.example.com:8443"},
		{name: "default http port is stripped", rawURL: "http://api.example.com:80", want: "http://api.example.com"},
		{name: "http and https stay separate", rawURL: "http://api.example.com", want: "http://api.example.com"},
		{name: "credentials block grouping", rawURL: "https://user:pass@api.example.com", want: ""},
		{name: "query blocks grouping", rawURL: "https://api.example.com/v1?key=1", want: ""},
		{name: "fragment blocks grouping", rawURL: "https://api.example.com#section", want: ""},
		{name: "empty URL cannot form an identity", rawURL: "", want: ""},
		{name: "unparseable text is not a URL identity", rawURL: "not a url", want: ""},
		{name: "non-http scheme, empty and unparseable URLs cannot form a group identity", rawURL: "ftp://api.example.com", want: ""},
		{rawURL: "", want: ""},
		{rawURL: "   ", want: ""},
		{rawURL: "not a url", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeBillingURLGroupKey(test.rawURL)
			assert.Equal(t, test.want, got)
		})
	}
}

func urlPtr(value string) *string {
	return &value
}

func TestGetProviderBillingURLSummaryMergesChannelsByCurrentBaseURL(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 21, Name: "alpha", BaseURL: urlPtr("https://API.Example.COM:443/")},
		{Id: 22, Name: "beta", BaseURL: urlPtr("https://api.example.com")},
		{Id: 23, Name: "gamma", BaseURL: urlPtr("https://api.example.com/v2")},
		{Id: 24, Name: "delta", BaseURL: urlPtr("")},
		{Id: 26, Name: "epsilon", Type: 1, BaseURL: nil},
	}).Error)

	logs := []Log{
		{UserId: 7, CreatedAt: 1150, Type: LogTypeConsume, ChannelId: 22, ModelName: "shared-model", PromptTokens: 150, Quota: 900, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 21, ModelName: "shared-model", PromptTokens: 100, Quota: 1200, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1200, Type: LogTypeRefund, ChannelId: 22, ModelName: "shared-model", PromptTokens: 50, Quota: 300, Other: `{"task_id":"t-1","task_billing_event":"adjustment","actual_quota":300,"pre_consumed_quota":300,"contract_applicable":false,"group_ratio":1,"model_ratio":1,"statement_snapshot":{"billing_mode":"token"}}`},
		{UserId: 7, CreatedAt: 1250, Type: LogTypeRefund, ChannelId: 21, ModelName: "shared-model", Quota: 400, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, ChannelId: 23, ModelName: "v2-model", PromptTokens: 100, Quota: 501, Other: `{"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1310, Type: LogTypeConsume, ChannelId: 24, ModelName: "orphan-model"},
		{UserId: 7, CreatedAt: 1320, Type: LogTypeConsume, ChannelId: 25, ModelName: "ghost-model", PromptTokens: 30, Other: `{"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1320, Type: LogTypeConsume, ChannelId: 26, ModelName: "default-fallback-model", PromptTokens: 40, Other: `{"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 5)
	encoded, err := common.Marshal(summary)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"discount":null`)
	assert.Contains(t, string(encoded), `"value":"1","version":0,"source":"default"`)

	merged := summary.Groups[0]
	assert.Equal(t, "https://api.example.com", merged.UrlKey)
	assert.Equal(t, "https://api.example.com", merged.BaseURL)
	assert.Empty(t, merged.CustomName, "no alias is saved for this grouping key")
	assert.False(t, merged.Unidentified)
	assert.False(t, merged.Deleted)
	assert.Equal(t, []int{21, 22}, merged.ChannelIds)
	assert.EqualValues(t, 2, merged.ChannelCount)
	assert.EqualValues(t, 1, merged.ModelCount)
	assert.EqualValues(t, 2, merged.Usage.Requests)
	assert.EqualValues(t, 0, merged.Usage.BillableCalls)
	assert.EqualValues(t, 300, merged.Usage.InputTokens)

	// One channel parent row per channel; same-named models stay inside their
	// own channel row with their per-channel coefficient.
	require.Len(t, merged.Channels, 2)
	alpha := merged.Channels[0]
	assert.Equal(t, "alpha", alpha.ChannelName)
	assert.Equal(t, 21, alpha.ChannelId)
	require.Len(t, alpha.Models, 1)
	assert.Equal(t, "shared-model", alpha.Models[0].ProviderModel)
	assert.False(t, alpha.Models[0].ProviderModelFallback)
	assert.Equal(t, BillingReconciliationModeToken, alpha.Models[0].BillingMode)
	assert.EqualValues(t, 100, alpha.Models[0].Usage.InputTokens)
	assert.Equal(t, "shared-model", alpha.Models[0].DetailFilter.ModelName)
	assert.Equal(t, 21, alpha.Models[0].DetailFilter.ChannelId)
	beta := merged.Channels[1]
	assert.Equal(t, "beta", beta.ChannelName)
	require.Len(t, beta.Models, 1)
	assert.EqualValues(t, 200, beta.Models[0].Usage.InputTokens)
	assert.EqualValues(t, 200, beta.Usage.InputTokens)

	// Channel usage totals reconcile with their leaves and the group total.
	assert.EqualValues(t, merged.Channels[0].Usage.InputTokens+merged.Channels[1].Usage.InputTokens, merged.Usage.InputTokens)
	assert.EqualValues(t, merged.Channels[0].Usage.Requests+merged.Channels[1].Usage.Requests, merged.Usage.Requests)
	// Amounts: refunds provable as task adjustments stay negative in the
	// official-price projection, including ordinary customer refunds.
	require.NotNil(t, merged.OriginalAmount)
	assert.EqualValues(t, 1400, *merged.OriginalAmount)
	require.NotNil(t, merged.ReferenceAmount)
	assert.EqualValues(t, 1400, *merged.ReferenceAmount)
	assert.Zero(t, merged.DiscountPendingChannels)
	assert.True(t, merged.ReferenceKnown)
	require.NotNil(t, alpha.Discount)
	require.NotNil(t, beta.Discount)
}

func TestGetProviderBillingURLSummaryKeepsUnidentifiableAndDeletedChannelsSeparate(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 23, Name: "gamma", BaseURL: urlPtr("https://api.example.com/v2")},
		{Id: 24, Name: "delta", BaseURL: urlPtr("")},
	}).Error)

	logs := []Log{
		{UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, ChannelId: 23, ModelName: "v2-model", PromptTokens: 100, Quota: 501, Other: `{"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1310, Type: LogTypeConsume, ChannelId: 24, ModelName: "orphan-model"},
		{UserId: 7, CreatedAt: 1320, Type: LogTypeConsume, ChannelId: 25, ModelName: "ghost-model", PromptTokens: 30, Other: `{"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 3)

	v2 := summary.Groups[0]
	assert.Equal(t, "https://api.example.com/v2", v2.UrlKey)
	assert.False(t, v2.Unidentified)
	assert.Equal(t, []int{23}, v2.ChannelIds)

	deleted := summary.Groups[1]
	assert.Equal(t, "channel:25", deleted.UrlKey)
	assert.True(t, deleted.Unidentified)
	assert.True(t, deleted.Deleted)
	assert.Equal(t, "Channel #25", deleted.DisplayName)
	assert.Empty(t, deleted.BaseURL)

	delta := summary.Groups[2]
	assert.Equal(t, "channel:24", delta.UrlKey)
	assert.True(t, delta.Unidentified)
	assert.False(t, delta.Deleted)
	assert.Equal(t, "delta", delta.DisplayName)
	assert.Empty(t, delta.BaseURL)
	require.NotNil(t, delta.DataQuality)
	assert.Equal(t, "partial", delta.DataQuality.Status)
	assert.EqualValues(t, 1, delta.DataQuality.UnavailableRequests)
	assert.EqualValues(t, 1, delta.DataQuality.UnknownBillingModeRequests)

	// The read-only URL view must not materialize monthly discounts.
	var discountRows int64
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Count(&discountRows).Error)
	assert.Zero(t, discountRows)
}

func TestGetProviderBillingURLSummaryURLFilterReturnsOnlyMatchingGroup(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 21, Name: "alpha", BaseURL: urlPtr("https://api.example.com")},
		{Id: 22, Name: "beta", BaseURL: urlPtr("https://api.example.com/v2")},
		{Id: 24, Name: "delta", BaseURL: urlPtr("")},
	}).Error)

	logs := []Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 21, ModelName: "m", PromptTokens: 10, Other: `{"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, ChannelId: 22, ModelName: "m", PromptTokens: 10},
		{UserId: 7, CreatedAt: 1310, Type: LogTypeConsume, ChannelId: 24, ModelName: "m", PromptTokens: 10, Other: `{"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	filtered, err := GetProviderBillingURLSummary(1000, 1500, 1000, "https://api.example.com")
	require.NoError(t, err)
	require.Len(t, filtered.Groups, 1)
	assert.Equal(t, "https://api.example.com", filtered.Groups[0].UrlKey)
	assert.Equal(t, []int{21}, filtered.Groups[0].ChannelIds)
	assert.Equal(t, filtered.Groups[0].DataQuality, filtered.DataQuality)

	unidentifiedFiltered, err := GetProviderBillingURLSummary(1000, 1500, 1000, "channel:24")
	require.NoError(t, err)
	require.Len(t, unidentifiedFiltered.Groups, 1)
	assert.Equal(t, "channel:24", unidentifiedFiltered.Groups[0].UrlKey)
	assert.True(t, unidentifiedFiltered.Groups[0].Unidentified)

	empty, err := GetProviderBillingURLSummary(1000, 1500, 1000, "https://api.example.com/other")
	require.NoError(t, err)
	assert.Empty(t, empty.Groups)
	assert.Equal(t, &BillingReconciliationDataQuality{Status: "complete"}, empty.DataQuality)
	require.NotNil(t, empty.Groups)
	encoded, err := common.Marshal(empty)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"url_groups":[]`)
}

func TestProviderBillingURLSummaryMatchesAllChannelUsageAndQualityDimensions(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 21, Name: "alpha", BaseURL: urlPtr("https://api.example.com")},
		{Id: 22, Name: "beta", BaseURL: urlPtr("https://api.example.com/")},
	}).Error)
	for _, channel := range []int{21, 22} {
		require.NoError(t, db.Create(&[]Log{
			{CreatedAt: 1100, Type: LogTypeConsume, ChannelId: channel, ModelName: "same-model", PromptTokens: 100, CompletionTokens: 20, Other: `{"group_ratio":1,"model_ratio":1,"cache_tokens":5,"cache_creation_tokens":3}`},
			{CreatedAt: 1101, Type: LogTypeConsume, ChannelId: channel, ModelName: "same-model", Other: `{"group_ratio":1,"model_price":0.002}`},
			{CreatedAt: 1102, Type: LogTypeConsume, ChannelId: channel, ModelName: "same-model"},
		}).Error)
	}
	urls, err := GetProviderBillingURLSummary(1000, 1500, 1000, "")
	require.NoError(t, err)
	require.Len(t, urls.Groups, 1)
	group := urls.Groups[0]
	assert.EqualValues(t, 2, group.ModelCount, "token and per-call share one identity; unknown fallback stays separate")
	require.Len(t, group.Channels, 2)
	var total ProviderBillingUsage
	var quality *BillingReconciliationDataQuality
	for _, channel := range group.Channels {
		for _, leaf := range channel.Models {
			total.Requests += leaf.Usage.Requests
			total.BillableCalls += leaf.Usage.BillableCalls
			total.InputTokens += leaf.Usage.InputTokens
			total.CacheReadTokens += leaf.Usage.CacheReadTokens
			total.CacheWriteTokens += leaf.Usage.CacheWriteTokens
			total.OutputTokens += leaf.Usage.OutputTokens
			accumulateBillingReconciliationQuality(&quality, leaf.DataQuality)
		}
	}
	finalizeBillingReconciliationQuality(&quality)
	assert.Equal(t, total, group.Usage)
	assert.Equal(t, quality, group.DataQuality)
	// Channel totals reconcile with their model leaves and the group total.
	var channelTotal ProviderBillingUsage
	for _, channel := range group.Channels {
		var rowTotal ProviderBillingUsage
		for _, leaf := range channel.Models {
			accumulateProviderBillingUsage(&rowTotal, leaf.Usage)
		}
		assert.Equal(t, rowTotal, channel.Usage)
		accumulateProviderBillingUsage(&channelTotal, channel.Usage)
	}
	assert.Equal(t, channelTotal, group.Usage)
	assert.EqualValues(t, 6, group.Usage.Requests)
	assert.EqualValues(t, 2, group.Usage.BillableCalls)
	assert.Positive(t, group.Usage.CacheReadTokens)
	assert.Positive(t, group.Usage.CacheWriteTokens)
}

// The same provider model name used by two channels of one URL group stays in
// its own channel row; amounts still reconcile from leaves to group.
func TestProviderBillingURLSummaryKeepsSameModelInsideEachChannel(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Channel{
		{Id: 31, Name: "one", BaseURL: urlPtr("https://split.example.com")},
		{Id: 32, Name: "two", BaseURL: urlPtr("https://split.example.com/")},
	}).Error)
	require.NoError(t, SaveProviderChannelBillingDiscount(&ProviderChannelBillingDiscount{PeriodStart: 1000, ChannelId: 31, Discount: decimal.RequireFromString("0.5"), Reason: "half"}, 0, 9))
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 31, ModelName: "shared", PromptTokens: 10, Quota: 1000, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1101, Type: LogTypeConsume, ChannelId: 32, ModelName: "shared", PromptTokens: 20, Quota: 400, Other: `{"contract_applicable":false,"group_ratio":1,"model_ratio":1}`},
	}).Error)

	summary, err := GetProviderBillingURLSummary(1000, 1500, 1000, "https://split.example.com")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	assert.EqualValues(t, 1, group.ModelCount, "one model identity across both channels")
	require.Len(t, group.Channels, 2)
	for _, channel := range group.Channels {
		require.Len(t, channel.Models, 1)
		assert.Equal(t, "shared", channel.Models[0].ProviderModel)
	}
	one, two := group.Channels[0], group.Channels[1]
	assert.Equal(t, "one", one.ChannelName)
	require.NotNil(t, one.OriginalAmount)
	require.NotNil(t, one.ReferenceAmount)
	assert.EqualValues(t, 1000, *one.OriginalAmount)
	assert.EqualValues(t, 500, *one.ReferenceAmount, "the saved coefficient applies only inside its channel")
	require.NotNil(t, two.OriginalAmount)
	require.NotNil(t, two.ReferenceAmount)
	assert.EqualValues(t, 400, *two.OriginalAmount)
	assert.EqualValues(t, 400, *two.ReferenceAmount, "an unconfigured channel projects the default coefficient 1")
	require.NotNil(t, group.OriginalAmount)
	assert.EqualValues(t, 1400, *group.OriginalAmount)
	require.NotNil(t, group.ReferenceAmount)
	assert.EqualValues(t, 900, *group.ReferenceAmount)
	assert.True(t, group.ReferenceKnown)
	assert.Equal(t, 0, group.DiscountPendingChannels, "unconfigured channels are defaults, not migration-pending")
}
