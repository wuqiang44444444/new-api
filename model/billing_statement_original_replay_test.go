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

func TestBillingReportPublishesReplayedOriginalWithoutChangingSettlement(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	end := start + billingReportDaySeconds - 1
	expr := base64.StdEncoding.EncodeToString([]byte(`tier("base", p * 2 + c * 10 + cr * 0.1 + cc * 2.5 + cc1h * 4)`))
	evidence := fmt.Sprintf(`{"billing_mode":"tiered_expr","billing_unit":"token","expr_b64":%q,"matched_tier":"base","group_ratio":1,"contract_discount":0.75,"usage_semantic":"anthropic","cache_tokens":0,"cache_creation_tokens":0}`, expr)
	charge := Log{UserId: 7, TokenId: 11, ChannelId: 21, ModelName: "sample",
		Type: LogTypeConsume, CreatedAt: start + 1, PromptTokens: 115404, CompletionTokens: 874,
		Quota: 89831, Other: evidence, RequestId: "sample-charge"}
	require.NoError(t, db.Create(&charge).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	pinned, manifest, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	require.Equal(t, 1, manifest.PublishedDays)

	overview, err := GetBillingCustomerStatementList(pinned, start, end, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, overview.Items, 1)
	require.NotNil(t, overview.Items[0].OriginalQuota)
	assert.EqualValues(t, 119774, *overview.Items[0].OriginalQuota)
	assert.EqualValues(t, 89831, overview.Items[0].Usage.NetQuota)

	report, err := GetBillingCustomerStatement(pinned, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	require.NotNil(t, report.OriginalQuota)
	require.NotNil(t, report.DiscountQuota)
	assert.EqualValues(t, 119774, *report.OriginalQuota)
	assert.EqualValues(t, 29943, *report.DiscountQuota)

	var draft BillingStatementVersionLine
	_, err = GetBillingCustomerStatement(ctx, 7, start, end, "api_key", 0, "", "", BillingStatementReadPolicy{
		ObserveFact: func(line BillingStatementVersionLine, _ *Log) error { draft = line; return nil },
	})
	require.NoError(t, err)
	var frozen CustomerExportRow
	require.NoError(t, common.UnmarshalJsonStr(draft.Facts, &frozen))
	assert.Equal(t, "119774", frozen.RecordedOriginal)
	assert.Equal(t, "replayed", frozen.OriginalSource)

	export, _, err := ReadBillingReportCustomerExportBatch(pinned, 7,
		CustomerExportFilters{StartTimestamp: start, EndTimestamp: end + 1}, 0, 100)
	require.NoError(t, err)
	require.Len(t, export, 1)
	assert.Equal(t, "119774", export[0].RecordedOriginal)
	assert.Equal(t, "replayed", export[0].OriginalSource)
	var saved Log
	require.NoError(t, db.First(&saved, charge.Id).Error)
	assert.Equal(t, charge.Other, saved.Other)
	assert.Equal(t, charge.Quota, saved.Quota)

}

func TestCustomerOriginalReplayRejectsMissingOrConflictingSettlementEvidence(t *testing.T) {
	expr := base64.StdEncoding.EncodeToString([]byte(`tier("base", p * 2 + cc * 2.5 + cc1h * 4)`))
	prefix := fmt.Sprintf(`"billing_mode":"tiered_expr","billing_unit":"token","expr_b64":%q,"matched_tier":"base","group_ratio":1,"contract_discount":0.75,"usage_semantic":"anthropic","cache_tokens":0,`, expr)
	for _, tc := range []struct {
		name, suffix, reason string
		quota                int
	}{
		{"missing cache split", `"cache_creation_tokens":10`, BillingOriginalReplayMissingUsage, 75},
		{"charge mismatch", `"cache_creation_tokens":0`, BillingOriginalReplayMismatch, 76},
		{"settlement fell back to hold", `"cache_creation_tokens":0,"admin_info":{"statement_funding_adjustment_failed":true}`, BillingOriginalReplayUnverified, 75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := billingReconciliationLog{UserId: 7, Type: LogTypeConsume, PromptTokens: 100, Quota: tc.quota,
				Other: "{" + prefix + tc.suffix + "}"}
			parsed := parseBillingReconciliationLog(log)
			applyCustomerOriginalReplay(log, &parsed)
			_, ok := customerStatementOriginalQuota(log, parsed)
			assert.False(t, ok)
			assert.Equal(t, tc.reason, parsed.originalReplayReason)
		})
	}
}

func TestBillingReportPreservesPublishedOriginalWhenCandidateLosesEvidence(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	end := start + billingReportDaySeconds - 1
	charge := Log{UserId: 7, TokenId: 11, ChannelId: 21, ModelName: "sample", Type: LogTypeConsume,
		CreatedAt: start + 1, Quota: 80,
		Other: `{"group_ratio":0.8,"contract_applicable":false,"admin_info":{"statement_original_quota":100}}`}
	require.NoError(t, db.Create(&charge).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	_, original, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)

	require.NoError(t, db.Model(&charge).Update("other", `{"group_ratio":0.8,"contract_applicable":false}`).Error)
	require.ErrorIs(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil), errDailyReportCoverageDecreased)
	pinned, current, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	assert.Equal(t, original.Version, current.Version)
	statement, err := GetBillingCustomerStatement(pinned, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	require.NotNil(t, statement.OriginalQuota)
	assert.EqualValues(t, 100, *statement.OriginalQuota)
	assert.EqualValues(t, 80, statement.Summary.NetQuota)
}

func TestBatchStatementOriginalRequiresFrozenConversionAndMatchingCharge(t *testing.T) {
	job := BatchJob{Id: "batch_example", LineCount: 2,
		FrozenSnapshot: BillingText(`{"quota_per_unit":500000,"group_ratio":0.75}`)}
	calc := `{"version":1,"quota":89831,"steps":[{"op":"batch_conversion","inputs":["239548","1000000","500000","0.75"],"result":"89830.5"}]}`
	lines := []BatchJobLine{
		{JobId: job.Id, Status: "completed", ModelQuota: 89831, FinalQuota: 89831, CalculationVersion: 1, Calculation: BillingText(calc)},
		{JobId: job.Id, Status: "failed"},
	}
	original, ok := BatchStatementOriginalQuota(&job, lines, 89831)
	require.True(t, ok)
	assert.EqualValues(t, 119774, original)
	_, ok = BatchStatementOriginalQuota(&job, lines, 89830)
	assert.False(t, ok)
	lines[0].QuotaClamp = "present"
	_, ok = BatchStatementOriginalQuota(&job, lines, 89831)
	assert.False(t, ok)
}
