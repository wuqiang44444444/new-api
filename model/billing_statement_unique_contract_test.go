package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerOriginalReplayOnlyPublishesUniqueSettlementSolution(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		charge, tokens   int
		want             string
	}{
		{"unique", `tier("base", p * 0.25 + cr * 0.025 + c * 1.5 + img_o * 30)`, 11737, 1056, "15860"},
		{"no matching value", `tier("base", p * 0.25 + cr * 0.025 + c * 1.5 + img_o * 30)`, 11738, 1056, ""},
		{"multiple values match", `tier("base", c * 2 + img_o * 2)`, 2, 3, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanned := customerExportScanRow{
				UserId: 7, Type: LogTypeConsume, Quota: tc.charge, PromptTokens: 162, CompletionTokens: tc.tokens,
				Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"matched_tier":"base","billing_unit":"token","group_ratio":1,"contract_discount":0.74,"usage_semantic":"gemini","cache_tokens":0,"request_path":"/v1beta/models/fixture:generateContent"}`,
					base64.StdEncoding.EncodeToString([]byte(tc.expression))),
			}
			fact := billingReconciliationLog{UserId: scanned.UserId, Type: scanned.Type, Quota: scanned.Quota,
				PromptTokens: scanned.PromptTokens, CompletionTokens: scanned.CompletionTokens, Other: scanned.Other}
			parsed := parseBillingReconciliationLog(fact)
			applyCustomerOriginalReplay(fact, &parsed)
			row := buildCustomerExportRow(scanned, fact, parsed)
			assert.Equal(t, tc.want, row.RecordedOriginal)
			assert.EqualValues(t, tc.charge, row.Quota)
			if tc.want == "" {
				assert.Nil(t, row.DerivedOutputImageTokens)
				return
			}
			require.NotNil(t, row.DerivedOutputImageTokens)
			assert.Equal(t, 1056, *row.DerivedOutputImageTokens)
			assert.Equal(t, customerOriginalUniqueSettlement, row.OriginalSource)
		})
	}
}

func TestLegacyTaskStatementProjectsFrozenContractDifferenceWithoutChangingFunding(t *testing.T) {
	db, day, _ := setupBillingReportTest(t)
	ctx := context.Background()
	initial := billingexpr.NewCalculation()
	initial.Add("quota_conversion", "quota", 200000, 0.4, 500000, 1)
	initial.Add("group_ratio", "quota", 200000, 200000, 1)
	initial.Add("contract_ratio", "quota", 146000, 200000, 0.73)
	initial.Add("round", "quota", 146000, 146000)
	initial.Finish(146000)
	final := billingexpr.NewCalculation()
	final.Add("quota_conversion", "quota", 200000, 0.4, 500000, 1)
	final.Add("group_ratio", "quota", 200000, 200000, 1)
	final.Add("round", "quota", 200000, 200000)
	final.Finish(200000)
	task := Task{
		TaskID: "legacy-billing-task", Status: TaskStatusSuccess, UserId: 7, AppID: 11, ChannelId: 21,
		SubmitTime: day + 100, FinishTime: day + 200, Quota: 200000,
		PrivateData: TaskPrivateData{TokenId: 11, BillingContext: &TaskBillingContext{
			OriginModelName: "sample", InitialCalculation: initial, SettlementCalculation: final,
			TieredSnapshot: &billingexpr.BillingSnapshot{QuotaPerUnit: 500000, GroupRatio: 1, TaskUsageBilling: true, EstimatedQuotaBeforeGroup: 200000},
			ContractFact:   &types.ContractBillingFact{ContractId: 113, ContractVersion: 13, RatioUnits: 73_000_000},
		}},
	}
	require.NoError(t, db.Create(&task).Error)
	const fields = `"task_id":"legacy-billing-task","is_task":true,"group_ratio":1,"contract_id":113,"contract_version":13,"contract_discount":0.73`
	logs := []Log{
		{Type: LogTypeConsume, UserId: 7, TokenId: 11, ChannelId: 21, ModelName: "sample", CreatedAt: day + 100,
			RequestId: "initial", Quota: 146000, Other: `{` + fields + `,"task_billing_event":"create"}`},
		{Type: LogTypeConsume, UserId: 7, TokenId: 11, ChannelId: 21, ModelName: "sample", CreatedAt: day + 200,
			RequestId: "final", Quota: 54000, Other: `{` + fields + `,"task_billing_event":"adjustment","pre_consumed_quota":146000,"actual_quota":200000}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, BuildBillingReportDay(ctx, day, "billing-test", "runner", nil, nil))
	ctx, report, err := ResolveBillingReport(ctx, day, day+86399, "")
	require.NoError(t, err)
	rows, _, err := ReadBillingReportCustomerExportBatch(ctx, 7, CustomerExportFilters{StartTimestamp: day, EndTimestamp: day + 86400}, 0, 20)
	require.NoError(t, err)
	require.Equal(t, 1, report.PublishedDays)
	require.Len(t, rows, 2)
	assert.Equal(t, "200000", rows[0].RecordedOriginal)
	assert.Equal(t, "0", rows[1].RecordedOriginal)
	assert.EqualValues(t, 54000, rows[1].Quota)
	require.NotNil(t, rows[1].ContractSettlement)
	assert.Equal(t, CustomerContractSettlement{ExpectedQuota: 146000, ActualQuota: 200000, DeltaQuota: 54000}, *rows[1].ContractSettlement)
	var unchanged Log
	require.NoError(t, db.First(&unchanged, logs[1].Id).Error)
	assert.Equal(t, logs[1].Quota, unchanged.Quota)
	assert.Equal(t, logs[1].Other, unchanged.Other)
}
