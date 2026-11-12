package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageLogDetailShowsRecordedAndPublishedBillingAmounts(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	expr := base64.StdEncoding.EncodeToString([]byte(`tier("base", p * 2 + c * 10)`))
	logs := []Log{
		{UserId: 7, TokenId: 11, Type: LogTypeConsume, CreatedAt: start + 1, RequestId: "replay",
			ModelName: "model", Quota: 89831, PromptTokens: 115404, CompletionTokens: 874,
			Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","billing_unit":"token","expr_b64":%q,"matched_tier":"base","group_ratio":1,"contract_discount":0.75,"usage_semantic":"anthropic","cache_tokens":0}`, expr)},
		{UserId: 7, TokenId: 11, Type: LogTypeConsume, CreatedAt: start + 2, RequestId: "recorded-zero",
			ModelName: "model", Other: `{"admin_info":{"statement_original_quota":0,"statement_snapshot":{"provider_model":"private"}}}`},
		{UserId: 7, TokenId: 11, Type: LogTypeConsume, CreatedAt: start + 3, RequestId: "missing",
			ModelName: "model", Quota: 100, Other: `{"group_ratio":0.8,"contract_applicable":false}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	filter := LogFilter{StartTimestamp: start, EndTimestamp: end}
	page, err := GetLogCursorPage(ctx, 7, filter, "start", 1, 20, 0, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, page.DetailRefs, len(logs))
	refs := make(map[string]string)
	for i, item := range page.Items {
		refs[item.RequestId] = page.DetailRefs[i]
	}
	readDetail := func(requestID string) (*Log, CustomerExportRow) {
		t.Helper()
		detail, err := GetLogCursorDetail(ctx, 7, refs[requestID])
		require.NoError(t, err)
		var projection struct {
			Facts *CustomerExportRow `json:"billing_facts"`
		}
		require.NoError(t, common.UnmarshalJsonStr(detail.Other, &projection))
		require.NotNil(t, projection.Facts)
		return detail, *projection.Facts
	}

	detail, recorded := readDetail("recorded-zero")
	assert.Equal(t, "0", recorded.RecordedOriginal)
	assert.NotContains(t, detail.Other, "admin_info")
	assert.NotContains(t, detail.Other, "private")
	_, unpublished := readDetail("replay")
	assert.Empty(t, unpublished.RecordedOriginal)

	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	detail, replayed := readDetail("replay")
	assert.Equal(t, "119774", replayed.RecordedOriginal)
	assert.Equal(t, "replayed", replayed.OriginalSource)
	assert.Equal(t, 89831, detail.Quota)
	_, missing := readDetail("missing")
	assert.Empty(t, missing.RecordedOriginal)
	assert.NotEmpty(t, missing.EstimateReasons)
	_, err = GetLogCursorDetail(ctx, 8, refs["replay"])
	require.Error(t, err)
}
