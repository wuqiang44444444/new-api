package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceFilterKeepsOverlappingRowsAndPaginationInScope(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 25, Name: "Evidence channel", BaseURL: urlPtr("https://evidence.test")}).Error)
	facts := `"model_ratio":1,"group_ratio":1,"completion_ratio":1,"cache_ratio":0.1,"model_price":-1,"user_group_ratio":-1,"contract_applicable":false`
	rows := []Log{
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1100, ModelName: "text", Quota: 10, RequestId: "complete", Other: `{` + facts + `,"request_path":"/v1/chat/completions","cache_tokens":0}`},
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1101, ModelName: "image", Quota: 10, RequestId: "missing-meters", Other: `{` + facts + `,"request_path":"/v1/images/edits","cache_tokens":"bad"}`},
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1102, ModelName: "image", Quota: 10, RequestId: "unpriced", TokenName: "模型测试", Content: "模型测试", Other: `{` + facts + `,"request_path":"/v1/images/edits","cache_tokens":"bad"}`},
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1103, ModelName: "text", Quota: 10, RequestId: "priced", TokenName: "模型测试", Content: "模型测试", Other: `{` + facts + `,"cache_tokens":0,"cache_write_tokens":0,"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":10}}`},
	}
	require.NoError(t, db.Create(&rows).Error)
	summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
	require.NoError(t, err)
	for _, tc := range []struct {
		filter   string
		requests []string
	}{
		{"incomplete", []string{"missing-meters", "unpriced"}},
		{"complete", []string{"complete", "priced"}},
		{"amount_gap", []string{"unpriced"}},
		{"usage_gap", []string{"missing-meters"}},
		{"cache_write_historical", []string{"missing-meters"}},
		{"legacy_test_cache_write_rows", []string{"unpriced"}},
		{"usage_without_amount_rows", []string{"unpriced"}},
		{"test_priced_rows", []string{"priced"}},
		{"other_gap", []string{}},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			filter := UpstreamBillingDetailFilter{Start: 1000, End: 1200, EvidenceFilter: tc.filter}
			detail, err := GetUpstreamBillingDetails(context.Background(), filter, 1, 1, false)
			require.NoError(t, err)
			assert.EqualValues(t, len(tc.requests), detail.Total)
			if len(tc.requests) > 0 {
				require.Len(t, detail.Items, 1)
				assert.Equal(t, tc.requests[0], detail.Items[0].RequestId)
				assert.Equal(t, "Evidence channel", detail.Items[0].ChannelName)
			}
			if len(tc.requests) > 1 {
				second, err := GetUpstreamBillingDetails(context.Background(), filter, 2, 1, false)
				require.NoError(t, err)
				require.Len(t, second.Items, 1)
				assert.Equal(t, tc.requests[1], second.Items[0].RequestId)
			}
			got := []string{}
			require.NoError(t, ScanUpstreamBillingDetails(context.Background(), filter, false, BillingStatementReadPolicy{}, func(row UpstreamBillingDetailItem) error { got = append(got, row.RequestId); return nil }))
			assert.Equal(t, tc.requests, got, "export scanner and UI must select identical rows")
			filter.ChannelIds = []int{99}
			empty, err := GetUpstreamBillingDetails(context.Background(), filter, 1, 50, false)
			require.NoError(t, err)
			assert.Zero(t, empty.Total)
		})
	}
	for reason, count := range summary.DataQuality.TestAmountPendingReasons {
		detail, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200, EvidenceFilter: "test:" + reason}, 1, 50, false)
		require.NoError(t, err)
		assert.Equal(t, count, detail.Total)
	}
	_, err = GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200, EvidenceFilter: "test:typo"}, 1, 50, false)
	require.ErrorContains(t, err, "invalid evidence_filter")
}
