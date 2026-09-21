package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeTextOmittedZeroCacheWriteIsNotMissingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		test        bool
		missing     int64
	}{
		{"ordinary text omitted zero", ``, false, 0},
		{"positive write", `,"cache_write_tokens":12`, false, 0},
		{"reported absent overrides writer", `,"cache_write_tokens_reported":false`, false, 1},
		{"malformed meter is not omitted zero", `,"cache_write_tokens":"bad"`, false, 1},
		{"claude writer must retain zero", `,"claude":true`, false, 1},
		{"legacy test cannot borrow text settlement evidence", ``, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&Channel{Id: 25, BaseURL: urlPtr("https://example.test")}).Error)
			row := Log{UserId: 7, Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1100, ModelName: "any-name", PromptTokens: 100, Quota: 10,
				Other: `{"request_path":"/v1/chat/completions","model_ratio":1,"group_ratio":1,"completion_ratio":1,"cache_tokens":0,"cache_ratio":0.1,"model_price":-1,"user_group_ratio":-1,"contract_applicable":false` + tc.extra + `}`}
			if tc.test {
				row.TokenName, row.Content = "模型测试", "模型测试"
			}
			require.NoError(t, db.Create(&row).Error)
			summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
			require.NoError(t, err)
			details, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, 1, 10, false)
			require.NoError(t, err)
			require.Len(t, details.Items, 1)
			assert.Equal(t, tc.missing, summary.DataQuality.CacheWriteUnavailableRequests)
			assert.Equal(t, tc.missing, details.Items[0].DataQuality.CacheWriteUnavailableRequests)
			if !tc.test {
				require.NotNil(t, summary.Groups[0].OriginalAmount)
				assert.EqualValues(t, 10, *summary.Groups[0].OriginalAmount, "quality classification must not change money")
			}
		})
	}
}

func TestUpstreamEvidenceCoverageCountsOverlappingGapsOnce(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Channel{Id: 25, BaseURL: urlPtr("https://example.test")}).Error)
	common := `"model_ratio":1,"group_ratio":1,"completion_ratio":1,"cache_ratio":0.1,"model_price":-1,"user_group_ratio":-1,"contract_applicable":false`
	rows := []Log{
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1100, ModelName: "text", Quota: 10, Other: `{` + common + `,"request_path":"/v1/chat/completions","cache_tokens":0}`},
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1101, ModelName: "image", Quota: 10, Other: `{` + common + `,"request_path":"/v1/images/edits","cache_tokens":"bad"}`},
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1102, ModelName: "image", Quota: 10, TokenName: "模型测试", Content: "模型测试", Other: `{` + common + `,"request_path":"/v1/images/edits","cache_tokens":"bad"}`},
		{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1103, ModelName: "text", Quota: 10, TokenName: "模型测试", Content: "模型测试", Other: `{` + common + `,"cache_tokens":0,"cache_write_tokens":0,"test_pricing":{"version":1,"mode":"ratio","status":"settled","original_quota":10}}`},
	}
	require.NoError(t, db.Create(&rows).Error)
	summary, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
	require.NoError(t, err)
	for _, q := range []*BillingReconciliationDataQuality{summary.DataQuality, summary.Groups[0].DataQuality, summary.Groups[0].Channels[0].DataQuality} {
		require.NotNil(t, q.EvidenceCoverage)
		assert.EqualValues(t, 4, q.EvidenceCoverage.Rows)
		assert.EqualValues(t, 2, q.EvidenceCoverage.GapRows, "read, write and pricing gaps overlap; priced tests alone are not gaps")
		assert.EqualValues(t, 1, q.EvidenceCoverage.AmountGapRows)
		assert.EqualValues(t, 1, q.EvidenceCoverage.UsageGapRows)
		assert.Zero(t, q.EvidenceCoverage.OtherGapRows)
		assert.EqualValues(t, 1, q.UsageWithoutAmountRows)
		assert.EqualValues(t, 1, q.LegacyTestCacheReadRows)
		assert.EqualValues(t, 1, q.LegacyTestCacheWriteRows)
		assert.EqualValues(t, 2, q.CacheReadUnavailableRequests)
		assert.EqualValues(t, 2, q.CacheWriteUnavailableRequests)
	}
	var total, gaps int64
	require.NoError(t, ScanUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, false, BillingStatementReadPolicy{}, func(row UpstreamBillingDetailItem) error {
		total++
		if row.DataQuality.Status != "complete" {
			gaps++
		}
		return nil
	}))
	assert.Equal(t, total, summary.DataQuality.EvidenceCoverage.Rows)
	assert.Equal(t, gaps, summary.DataQuality.EvidenceCoverage.GapRows)
}

func TestUpstreamVideoOnlyBillingDoesNotRequirePromptCacheMeters(t *testing.T) {
	for _, tc := range []struct {
		name, facts string
		missing     int64
	}{
		{"output tokens only", `"expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(`tier("video", c * 7)`)) + `"`, 0},
		{"input cache billed separately", `"expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(`tier("video", p + c * 7 + cr * 0.1)`)) + `"`, 1},
		{"native total token adjustment", `"actual_quota":10,"pre_consumed_quota":12`, 0},
		{"missing contract", `"note":"no billing contract"`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&Channel{Id: 25, BaseURL: urlPtr("https://video.test")}).Error)
			require.NoError(t, db.Create(&Log{Type: LogTypeConsume, ChannelId: 25, CreatedAt: 1100, ModelName: "video", CompletionTokens: 100, Quota: 10, Other: `{"task_id":"video-task","is_task":true,"model_price":0,"group_ratio":1,"contract_applicable":false,` + tc.facts + `}`}).Error)
			result, err := GetProviderBillingURLSummary(1000, 1200, 1000, "")
			require.NoError(t, err)
			rows, err := GetUpstreamBillingDetails(context.Background(), UpstreamBillingDetailFilter{Start: 1000, End: 1200}, 1, 10, false)
			require.NoError(t, err)
			require.Len(t, rows.Items, 1)
			for _, q := range []*BillingReconciliationDataQuality{result.DataQuality, rows.Items[0].DataQuality} {
				assert.Equal(t, tc.missing, q.CacheReadUnavailableRequests)
				assert.Equal(t, tc.missing, q.CacheWriteUnavailableRequests)
			}
			require.NotNil(t, rows.Items[0].OriginalAmount)
			assert.EqualValues(t, 10, *rows.Items[0].OriginalAmount)
		})
	}
}

func TestUpstreamHistoricalSecondsMissingValueIsNotMissingUnit(t *testing.T) {
	expression := base64.StdEncoding.EncodeToString([]byte(`tier("video", param("_task.duration_seconds") * 1000)`))
	log := billingReconciliationLog{Type: LogTypeConsume, Other: `{"is_task":true,"expr_b64":"` + expression + `"}`}
	parsed := parseBillingReconciliationLog(log)
	seconds, missing, unitKnown := upstreamBillingSeconds(log, parsed)
	assert.Nil(t, seconds)
	assert.True(t, missing)
	assert.True(t, unitKnown)
}
