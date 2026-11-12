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

func TestHistoricalFixedTierRequiresUniqueSettledPrice(t *testing.T) {
	const fixed = `param("kind") == "text" ? tier("text", 90000) : tier("reference", 45000 + param("references") * 3000)`
	for _, tc := range []struct {
		name, expression, tier, extra, wantMode string
		quota                                   int
	}{
		{"fixed matched branch", fixed, "text", "", BillingReconciliationModePerCall, 45000},
		{"amount mismatch", fixed, "text", "", BillingReconciliationModeUnknown, 45001},
		{"request dependent price", fixed, "reference", "", BillingReconciliationModeUnknown, 22500},
		{"unknown matched branch", fixed, "missing", "", BillingReconciliationModeUnknown, 45000},
		{"ambiguous tier", `param("kind") == "text" ? tier("text", 90000) : tier("text", 90000)`, "text", "", BillingReconciliationModeUnknown, 45000},
		{"missing contract state", fixed, "text", "", BillingReconciliationModeUnknown, 45000},
		{"conflicting recorded original", fixed, "text", `,"admin_info":{"statement_original_quota":45001}`, BillingReconciliationModeUnknown, 45000},
		{"conflicting frozen mode", fixed, "text", `,"statement_snapshot":{"billing_mode":"per_second"}`, BillingReconciliationModeUnknown, 45000},
		{"funding fallback", fixed, "text", `,"admin_info":{"statement_funding_adjustment_failed":true}`, BillingReconciliationModeUnknown, 45000},
		{"saturation marker", fixed, "text", `,"admin_info":{"quota_saturation":{"direction":"high"}}`, BillingReconciliationModeUnknown, 45000},
		{"auxiliary charge", fixed, "text", `,"fee_quota":0`, BillingReconciliationModeUnknown, 45000},
		{"explicit image quantity", fixed, "text", `,"image_count":2`, BillingReconciliationModeUnknown, 45000},
		{"token pricing", `param("kind") == "text" ? tier("text", p * 90000) : tier("reference", p * 45000)`, "text", "", BillingReconciliationModeToken, 45000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr := base64.StdEncoding.EncodeToString([]byte(tc.expression))
			contract := `,"contract_applicable":false`
			if tc.name == "missing contract state" {
				contract = ""
			}
			log := billingReconciliationLog{UserId: 7, Type: LogTypeConsume, Quota: tc.quota, PromptTokens: 1,
				Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","billing_unit":"token","expr_b64":%q,"matched_tier":%q,"group_ratio":1%s%s}`, expr, tc.tier, contract, tc.extra)}
			assert.Equal(t, tc.wantMode, parseBillingReconciliationLog(log).billingMode)
		})
	}
}

func TestHistoricalFixedTierBillingStatementAndPublishedReport(t *testing.T) {
	db, start, monthEnd := setupBillingReportTest(t)
	ctx := context.Background()
	end := start + billingReportDaySeconds - 1
	expr := base64.StdEncoding.EncodeToString([]byte(`param("kind") == "text" ? tier("text", 90000) : tier("reference", 45000 + param("references") * 3000)`))
	other := fmt.Sprintf(`{"request_path":"/v1/images/generations","billing_mode":"tiered_expr","billing_unit":"token","expr_b64":%q,"matched_tier":"text","group_ratio":1,"contract_applicable":false,"usage_semantic":"openai","admin_info":{"statement_snapshot":{"billing_mode":"token","model_price":0,"model_ratio":0}}}`, expr)
	charge := Log{UserId: 7, TokenId: 11, ChannelId: 21, ModelName: "legacy-image", Type: LogTypeConsume,
		CreatedAt: start + 1, PromptTokens: 1, Quota: 45000, Other: other}
	require.NoError(t, db.Create(&charge).Error)
	require.NoError(t, migrateBillingStatementVersionDB())
	review, err := GetBillingSourceReview(ctx, 7, start, monthEnd)
	require.NoError(t, err)
	assert.Empty(t, review.Issues)
	assert.EqualValues(t, 45000, review.NetQuota)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	pinned, manifest, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	require.Equal(t, 1, manifest.PublishedDays)

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{{"live", ctx}, {"published", pinned}} {
		t.Run(tc.name, func(t *testing.T) {
			statement, err := GetBillingCustomerStatement(tc.ctx, 7, start, end, "api_key", 0, "", "")
			require.NoError(t, err)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			assert.Equal(t, BillingReconciliationModePerCall, statement.Groups[0].Models[0].BillingMode)
			assert.EqualValues(t, 1, statement.Summary.Requests)
			assert.EqualValues(t, 1, statement.Summary.BillableCalls)
			assert.EqualValues(t, 45000, statement.Summary.NetQuota)
			if statement.DataQuality != nil {
				assert.Zero(t, statement.DataQuality.UnknownBillingModeRequests)
			}
			filter := BillingStatementLogFilter{UserId: 7, Start: start, End: end, BillingMode: BillingReconciliationModePerCall}
			details, err := GetBillingStatementLogs(tc.ctx, filter, 1, 20, common.RoleAdminUser)
			require.NoError(t, err)
			assert.EqualValues(t, 1, details.Total)
			assert.EqualValues(t, 45000, details.Quota)
			filter.BillingMode = BillingReconciliationModeUnknown
			details, err = GetBillingStatementLogs(tc.ctx, filter, 1, 20, common.RoleAdminUser)
			require.NoError(t, err)
			assert.Zero(t, details.Total)
		})
	}

	export, _, err := ReadBillingReportCustomerExportBatch(pinned, 7,
		CustomerExportFilters{StartTimestamp: start, EndTimestamp: end + 1, BillingMode: BillingReconciliationModePerCall}, 0, 20)
	require.NoError(t, err)
	require.Len(t, export, 1)
	assert.Equal(t, BillingReconciliationModePerCall, export[0].BillingMode)
	assert.EqualValues(t, 45000, export[0].Quota)
	var saved Log
	require.NoError(t, db.First(&saved, charge.Id).Error)
	assert.Equal(t, charge.Other, saved.Other)
	assert.Equal(t, charge.Quota, saved.Quota)
}
