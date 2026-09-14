package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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
		{UserId: 7, CreatedAt: 1150, Type: LogTypeConsume, ChannelId: 22, ModelName: "shared-model", PromptTokens: 150, Quota: 900, Other: `{"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1100, Type: LogTypeConsume, ChannelId: 21, ModelName: "shared-model", PromptTokens: 100, Quota: 1200, Other: `{"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1200, Type: LogTypeRefund, ChannelId: 22, ModelName: "shared-model", PromptTokens: 50, Quota: 300, Other: `{"task_id":"t-1","task_billing_event":"adjustment","actual_quota":300,"pre_consumed_quota":300,"statement_snapshot":{"billing_mode":"token"}}`},
		{UserId: 7, CreatedAt: 1250, Type: LogTypeRefund, ChannelId: 21, ModelName: "shared-model", Quota: 400, Other: `{"group_ratio":1,"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1300, Type: LogTypeConsume, ChannelId: 23, ModelName: "v2-model", PromptTokens: 100, Quota: 501, Other: `{"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1310, Type: LogTypeConsume, ChannelId: 24, ModelName: "orphan-model"},
		{UserId: 7, CreatedAt: 1320, Type: LogTypeConsume, ChannelId: 25, ModelName: "ghost-model", PromptTokens: 30, Other: `{"model_ratio":1}`},
		{UserId: 7, CreatedAt: 1320, Type: LogTypeConsume, ChannelId: 26, ModelName: "default-fallback-model", PromptTokens: 40, Other: `{"model_ratio":1}`},
	}
	require.NoError(t, db.Create(&logs).Error)

	summary, err := GetProviderBillingURLSummary(1000, 1500, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 5)
	encoded, err := common.Marshal(summary)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"discount":`)

	merged := summary.Groups[0]
	assert.Equal(t, "https://api.example.com", merged.UrlKey)
	assert.Equal(t, "https://api.example.com", merged.BaseURL)
	assert.False(t, merged.Unidentified)
	assert.False(t, merged.Deleted)
	assert.Equal(t, []int{21, 22}, merged.ChannelIds)
	assert.EqualValues(t, 2, merged.ChannelCount)
	assert.EqualValues(t, 1, merged.ModelCount)
	assert.EqualValues(t, 2, merged.Usage.Requests)
	assert.EqualValues(t, 0, merged.Usage.BillableCalls)
	assert.EqualValues(t, 300, merged.Usage.InputTokens)

	require.Len(t, merged.Models, 1)
	mergedModel := merged.Models[0]
	assert.Equal(t, "shared-model", mergedModel.ProviderModel)
	assert.Equal(t, BillingReconciliationModeToken, mergedModel.BillingMode)
	assert.False(t, mergedModel.ProviderModelFallback)
	assert.EqualValues(t, 2, mergedModel.Usage.Requests)
	assert.EqualValues(t, 300, mergedModel.Usage.InputTokens)
	require.Len(t, mergedModel.Channels, 2)
	assert.Equal(t, "alpha", mergedModel.Channels[0].ChannelName)
	assert.EqualValues(t, 100, mergedModel.Channels[0].Usage.InputTokens)
	assert.Equal(t, "beta", mergedModel.Channels[1].ChannelName)
	assert.EqualValues(t, 200, mergedModel.Channels[1].Usage.InputTokens)
	assert.Equal(t, 21, mergedModel.Channels[0].DetailFilter.ChannelId)
	assert.Equal(t, "shared-model", mergedModel.Channels[0].DetailFilter.ModelName)

	// Cross-check with the channel view: the URL group total equals the sum of
	// its channels' upstream summaries.
	channelSummaryAlpha, err := GetProviderBillingSummary(1000, 1500, 1000, 21, "", "", 9)
	require.NoError(t, err)
	channelSummaryBeta, err := GetProviderBillingSummary(1000, 1500, 1000, 22, "", "", 9)
	require.NoError(t, err)
	assert.EqualValues(t, channelSummaryAlpha.Channels[0].Usage.InputTokens+channelSummaryBeta.Channels[0].Usage.InputTokens, merged.Usage.InputTokens)
	assert.EqualValues(t, channelSummaryAlpha.Channels[0].Usage.Requests+channelSummaryBeta.Channels[0].Usage.Requests, merged.Usage.Requests)
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

	summary, err := GetProviderBillingURLSummary(1000, 1500, "")
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

	// The read-only URL view must not materialize monthly discount defaults.
	var discountRows int64
	require.NoError(t, db.Model(&ProviderBillingDiscount{}).Count(&discountRows).Error)
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

	filtered, err := GetProviderBillingURLSummary(1000, 1500, "https://api.example.com")
	require.NoError(t, err)
	require.Len(t, filtered.Groups, 1)
	assert.Equal(t, "https://api.example.com", filtered.Groups[0].UrlKey)
	assert.Equal(t, []int{21}, filtered.Groups[0].ChannelIds)
	assert.Equal(t, filtered.Groups[0].DataQuality, filtered.DataQuality)

	unidentifiedFiltered, err := GetProviderBillingURLSummary(1000, 1500, "channel:24")
	require.NoError(t, err)
	require.Len(t, unidentifiedFiltered.Groups, 1)
	assert.Equal(t, "channel:24", unidentifiedFiltered.Groups[0].UrlKey)
	assert.True(t, unidentifiedFiltered.Groups[0].Unidentified)

	empty, err := GetProviderBillingURLSummary(1000, 1500, "https://api.example.com/other")
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
	urls, err := GetProviderBillingURLSummary(1000, 1500, "")
	require.NoError(t, err)
	require.Len(t, urls.Groups, 1)
	group := urls.Groups[0]
	assert.EqualValues(t, 2, group.ModelCount, "token and per-call share one identity; unknown fallback stays separate")
	require.Len(t, group.Models, 3)
	var total ProviderBillingUsage
	var quality *BillingReconciliationDataQuality
	for _, channelID := range []int{21, 22} {
		summary, err := GetProviderBillingSummary(1000, 1500, 1000, channelID, "", "", 9)
		require.NoError(t, err)
		require.Len(t, summary.Channels, 1)
		channel := summary.Channels[0]
		total.Requests += channel.Usage.Requests
		total.BillableCalls += channel.Usage.BillableCalls
		total.InputTokens += channel.Usage.InputTokens
		total.CacheReadTokens += channel.Usage.CacheReadTokens
		total.CacheWriteTokens += channel.Usage.CacheWriteTokens
		total.OutputTokens += channel.Usage.OutputTokens
		accumulateBillingReconciliationQuality(&quality, channel.DataQuality)
		for _, urlModel := range group.Models {
			var expected *ProviderBillingPlatformSummary
			for i := range channel.Models {
				if channel.Models[i].BillingMode == urlModel.BillingMode {
					expected = &channel.Models[i]
				}
			}
			require.NotNil(t, expected)
			var found bool
			for _, leaf := range urlModel.Channels {
				if leaf.ChannelId == channelID {
					found = true
					assert.Equal(t, expected.Usage, leaf.Usage)
					assert.Equal(t, expected.DataQuality, leaf.DataQuality)
				}
			}
			assert.True(t, found)
		}
	}
	finalizeBillingReconciliationQuality(&quality)
	assert.Equal(t, total, group.Usage)
	assert.Equal(t, quality, group.DataQuality)
	assert.EqualValues(t, 6, group.Usage.Requests)
	assert.EqualValues(t, 2, group.Usage.BillableCalls)
	assert.Positive(t, group.Usage.CacheReadTokens)
	assert.Positive(t, group.Usage.CacheWriteTokens)
}
