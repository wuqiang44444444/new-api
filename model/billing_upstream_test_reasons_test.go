package model

import (
	"context"
	"encoding/base64"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpstreamPendingTestReasonsPartitionSummaryAndDetails(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 21, Name: "test"}).Error)
	facts := []string{
		`{"model_ratio":1,"completion_ratio":1,"cache_tokens":0,"request_path":"/v1/chat/completions","request_conversion":["OpenAI Compatible"]}`,
		`{"billing_mode":"tiered_expr","expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(`tier("base",p+c)`)) + `"}`,
		`{"test_pricing":{"mode":"ratio","status":"settled","original_quota":100}}`,
		`{"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":100}}`,
	}
	for _, other := range facts {
		require.NoError(t, db.Create(&Log{Type: LogTypeConsume, TokenName: "模型测试", Content: "模型测试", ChannelId: 21, CreatedAt: 1100, ModelName: "test", Quota: 100, PromptTokens: 100, Other: other}).Error)
	}
	summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
	require.NoError(t, err)
	want := map[string]int64{"missing_cache_write": 1, "missing_usage_semantic": 1, "missing_group_ratio": 1}
	require.Len(t, summary.Groups, 1)
	require.Len(t, summary.Groups[0].Channels, 1)
	for _, q := range []*BillingReconciliationDataQuality{summary.DataQuality, summary.Groups[0].DataQuality, summary.Groups[0].Channels[0].DataQuality} {
		require.NotNil(t, q)
		assert.Equal(t, want, q.TestAmountPendingReasons)
		assert.EqualValues(t, 3, q.UsageWithoutAmountRows)
		assert.EqualValues(t, 1, q.TestPricedRows)
	}
	seen := map[string]int64{}
	require.NoError(t, ScanUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, false, BillingStatementReadPolicy{}, func(row UpstreamBillingDetailItem) error {
		require.NotNil(t, row.TestPricing)
		if row.OriginalAmount == nil {
			assert.NotEmpty(t, row.TestPricing.Reason)
			seen[row.TestPricing.Reason]++
			assert.Equal(t, map[string]int64{row.TestPricing.Reason: 1}, row.DataQuality.TestAmountPendingReasons)
		} else {
			assert.Empty(t, row.TestPricing.Reason)
		}
		return nil
	}))
	assert.Equal(t, want, seen)
}
