package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamQualityMatchesDetailsAndAllSummaryLevels(t *testing.T) {
	for _, tc := range []struct {
		name, facts                  string
		cacheMissing, secondsMissing int64
		cache                        int64
		secondsValueMissing          int64
	}{
		{"image cache omitted", `"request_path":"/v1/images/edits","cache_tokens":0,"cache_read_tokens_reported":false`, 0, 0, 0, 0},
		{"image explicit zero", `"request_path":"/v1/images/edits","cache_tokens":0,"cache_read_tokens_reported":true`, 0, 0, 0, 0},
		{"image cache hit", `"request_path":"/v1/images/edits","cache_tokens":80,"cache_read_tokens_reported":true`, 0, 0, 80, 0},
		{"historical image billed zero", `"request_path":"/v1/images/generations","cache_tokens":0`, 0, 0, 0, 0},
		{"historical positive image cache", `"request_path":"/v1/images/generations","cache_tokens":80`, 0, 0, 80, 0},
		{"missing measured seconds", `"statement_snapshot":{"billing_mode":"per_second"},"usage_units":{"duration":"second"}`, 0, 1, 0, 1},
		{"missing frozen unit", `"statement_snapshot":{"billing_mode":"per_second"},"usage_facts":{"duration":3}`, 0, 1, 0, 0},
		{"explicit zero seconds", `"statement_snapshot":{"billing_mode":"per_second"},"usage_units":{"duration":"second"},"usage_facts":{"duration":0}`, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&Channel{Id: 81, BaseURL: urlPtr("https://quality.example")}).Error)
			other := fmt.Sprintf(`{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"usage_semantic":"openai","cache_write_tokens":0,"cache_write_tokens_reported":true,%s}`, tc.facts)
			require.NoError(t, db.Create(&Log{UserId: 7, ChannelId: 81, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, PromptTokens: 100, Quota: 10, Other: other}).Error)
			details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200, ChannelIds: []int{81}}, 1, 10, false)
			require.NoError(t, err)
			require.Len(t, details.Items, 1)
			summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
			require.NoError(t, err)
			require.Len(t, summary.Groups, 1)
			group := summary.Groups[0]
			require.Len(t, group.Channels, 1)
			channel := group.Channels[0]
			require.Len(t, channel.Models, 1)
			want := "complete"
			if tc.cacheMissing+tc.secondsMissing > 0 {
				want = "partial"
			}
			for _, quality := range []*BillingReconciliationDataQuality{details.Items[0].DataQuality, channel.Models[0].DataQuality, channel.DataQuality, group.DataQuality, summary.DataQuality} {
				require.NotNil(t, quality)
				assert.Equal(t, want, quality.Status)
				assert.Equal(t, tc.cacheMissing, quality.CacheReadUnavailableRequests)
				assert.Equal(t, tc.secondsMissing, quality.SecondsUnavailableRows)
				assert.Equal(t, tc.secondsValueMissing, quality.SecondsValueMissingRows)
			}
			assert.Equal(t, tc.cache, details.Items[0].CacheReadTokens)
			assert.Equal(t, tc.cache, group.Usage.CacheReadTokens)
			require.NotNil(t, group.OriginalAmount)
			assert.EqualValues(t, 10, *group.OriginalAmount, "evidence quality never changes settled amounts")
		})
	}
}

func TestUpstreamTestCoverageDoesNotMakeKnownEvidencePartial(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "quality-user", Quota: 100}).Error)
	require.NoError(t, db.Create(&Channel{Id: 81, BaseURL: urlPtr("https://quality.example")}).Error)
	other := `{"contract_applicable":false,"group_ratio":1,"model_ratio":1,"completion_ratio":1,"usage_semantic":"openai","cache_tokens":0,"cache_write_tokens":0,"upstream_model_name":"model"}`
	logs := []Log{
		{UserId: 7, ChannelId: 81, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, PromptTokens: 100, Quota: 10, Other: other},
		// 测试行冻结原生计价原价；已知金额与缓存质量分别判断。
		{UserId: 7, ChannelId: 81, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1101, PromptTokens: 100, Quota: 100, Other: other[:len(other)-1] + `,"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":100}}`, TokenName: "模型测试", Content: "模型测试"},
	}
	require.NoError(t, db.Create(&logs).Error)
	summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	group := summary.Groups[0]
	require.Len(t, group.Channels, 1)
	channel := group.Channels[0]
	require.Len(t, channel.Models, 1)
	for _, q := range []*BillingReconciliationDataQuality{summary.DataQuality, group.DataQuality, channel.DataQuality, channel.Models[0].DataQuality} {
		assert.Equal(t, "complete", q.Status)
		assert.EqualValues(t, 0, q.UsageWithoutAmountRows)
		assert.EqualValues(t, 1, q.TestPricedRows)
	}
	assert.EqualValues(t, 2, group.Usage.Requests)
	require.NotNil(t, group.OriginalAmount)
	assert.EqualValues(t, 110, *group.OriginalAmount)
	require.NotNil(t, group.ReferenceAmount)
	assert.EqualValues(t, 110, *group.ReferenceAmount)
	details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, 1, 10, false)
	require.NoError(t, err)
	require.Len(t, details.Items, 2)
	for _, row := range details.Items {
		assert.Equal(t, "complete", row.DataQuality.Status)
	}
	customer, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, customer.Summary.Requests)
	assert.EqualValues(t, 10, customer.Summary.NetQuota)
	require.NotNil(t, customer.CurrentBalance)
	assert.EqualValues(t, 100, *customer.CurrentBalance)

	require.NoError(t, db.Delete(&logs[0]).Error)
	testsOnly, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
	require.NoError(t, err)
	require.Len(t, testsOnly.Groups, 1)
	leaf := testsOnly.Groups[0].Channels[0].Models[0]
	// 纯测试渠道在价格证据充分时同样有金额，“仅计用量”不再适用。
	assert.False(t, leaf.UsageOnly)
	require.NotNil(t, leaf.OriginalAmount)
	assert.EqualValues(t, 100, *leaf.OriginalAmount)
	require.NotNil(t, leaf.ReferenceAmount)
	assert.EqualValues(t, 100, *leaf.ReferenceAmount)
	assert.Equal(t, "complete", leaf.DataQuality.Status)
}

func TestUpstreamCacheEvidenceMatchesForConsumptionAndChannelTests(t *testing.T) {
	for _, tc := range []struct {
		name, facts         string
		missing, unreported int64
		writeMissing        int64
	}{
		{"legacy image zero", `"cache_tokens":0`, 0, 0, 1},
		{"legacy billed write zero", `"cache_tokens":0,"cache_write_tokens":0`, 0, 0, 0},
		{"response omitted meters", `"cache_tokens":0,"cache_write_tokens":0,"cache_read_tokens_reported":false,"cache_write_tokens_reported":false`, 0, 0, 0},
		{"explicit zero meters", `"cache_tokens":0,"cache_write_tokens":0,"cache_read_tokens_reported":true,"cache_write_tokens_reported":true`, 0, 0, 0},
		{"historical positive meters", `"cache_tokens":20,"cache_write_tokens":10`, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&Channel{Id: 81, BaseURL: urlPtr("https://quality.example")}).Error)
			other := fmt.Sprintf(`{"request_path":"/v1/images/edits","contract_applicable":false,"group_ratio":1,"model_ratio":1,"usage_semantic":"openai",%s}`, tc.facts)
			logs := []Log{
				{UserId: 7, ChannelId: 81, ModelName: "image", Type: LogTypeConsume, CreatedAt: 1100, PromptTokens: 100, Quota: 10, Other: other},
				{UserId: 7, ChannelId: 81, ModelName: "image", Type: LogTypeConsume, CreatedAt: 1101, PromptTokens: 100, Quota: 10, Other: other, TokenName: "模型测试", Content: "模型测试"},
				{UserId: 7, ChannelId: 81, ModelName: "image", Type: LogTypeRefund, CreatedAt: 1102, Quota: 2, Other: other},
			}
			require.NoError(t, db.Create(&logs).Error)
			summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
			require.NoError(t, err)
			require.Len(t, summary.Groups, 1)
			g := summary.Groups[0]
			ch := g.Channels[0]
			leaf := ch.Models[0]
			for _, q := range []*BillingReconciliationDataQuality{summary.DataQuality, g.DataQuality, ch.DataQuality, leaf.DataQuality} {
				assert.Equal(t, 2*tc.missing, q.CacheReadUnavailableRequests)
				assert.Equal(t, 2*tc.writeMissing, q.CacheWriteUnavailableRequests)
				assert.Equal(t, 2*tc.unreported, q.CacheReadUnreportedRequests)
				assert.Equal(t, 2*tc.unreported, q.CacheWriteUnreportedRequests)
			}
			details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, 1, 10, false)
			require.NoError(t, err)
			require.Len(t, details.Items, 3)
			for _, r := range details.Items {
				missing, unreported, writeMissing := tc.missing, tc.unreported, tc.writeMissing
				if r.Event == "refund" {
					missing, unreported, writeMissing = 0, 0, 0
				}
				assert.Equal(t, missing, r.DataQuality.CacheReadUnavailableRequests)
				assert.Equal(t, writeMissing, r.DataQuality.CacheWriteUnavailableRequests)
				assert.Equal(t, unreported, r.DataQuality.CacheReadUnreportedRequests)
				assert.Equal(t, unreported, r.DataQuality.CacheWriteUnreportedRequests)
			}
		})
	}
}
