package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageDiscountsUseFrozenCombinationsAndCustomerScope(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("week", "2026-09-16", usageTestWeekStart+8*86400)
	require.NoError(t, err)
	first := `{"group_ratio":0.9,"user_group_ratio":0.8,"contract_discount":"0.5","contract_id":3,"contract_version":1,"contract_name":"Historical contract","model_ratio":1,"admin_info":{"statement_snapshot":{"customer_model":"public-model","provider_model":"private-provider-model"}}}`
	second := `{"group_ratio":0.8,"contract_discount":"0.9","contract_id":3,"contract_version":2,"contract_name":"Changed contract","model_ratio":1,"admin_info":{"statement_snapshot":{"customer_model":"public-model"}}}`
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 11, ChannelId: 21, Group: "historic-group", ModelName: "price-model", CreatedAt: period.StartTimestamp + 1, Type: LogTypeConsume, Quota: 40, Other: first},
		{UserId: 7, TokenId: 11, ChannelId: 22, Group: "historic-group", ModelName: "price-model", CreatedAt: period.StartTimestamp + 86401, Type: LogTypeConsume, Quota: 40, Other: first},
		{UserId: 7, TokenId: 11, ChannelId: 21, Group: "historic-group", ModelName: "price-model", CreatedAt: period.StartTimestamp + 86402, Type: LogTypeConsume, Quota: 72, Other: second},
		{UserId: 8, TokenId: 11, ChannelId: 21, Group: "other-user", ModelName: "price-model", CreatedAt: period.StartTimestamp + 2, Type: LogTypeConsume, Quota: 999, Other: first},
		{UserId: 7, TokenId: 12, ChannelId: 21, Group: "separate-key", ModelName: "price-model", CreatedAt: period.StartTimestamp + 2, Type: LogTypeConsume, Quota: 40, Other: first},
		{UserId: 7, TokenId: 11, ModelName: "public-model", CreatedAt: period.StartTimestamp + 2, Type: LogTypeRefund, Quota: 10, Other: first},
	}).Error)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)
	require.Len(t, view.Keys, 2)
	row := view.Keys[0].Models[0]
	assert.Equal(t, "public-model", row.ModelName)
	require.Len(t, row.DiscountCombinations, 2)
	combos := row.DiscountCombinations
	assert.EqualValues(t, 80, combos[0].Usage.NetQuota)
	assert.Equal(t, "user_exclusive", combos[0].GroupRatioSource)
	require.NotNil(t, combos[0].GroupRatio)
	assert.Equal(t, 0.8, *combos[0].GroupRatio)
	assert.Equal(t, "historic-group", combos[0].GroupName)
	assert.EqualValues(t, 1, combos[0].ContractVersion)
	require.NotNil(t, combos[0].OriginalQuota)
	assert.EqualValues(t, 200, *combos[0].OriginalQuota)
	assert.EqualValues(t, 120, *combos[0].DiscountQuota)
	assert.EqualValues(t, 2, combos[1].ContractVersion)
	assert.EqualValues(t, row.Total.NetQuota, combos[0].Usage.NetQuota+combos[1].Usage.NetQuota)
	assert.EqualValues(t, 3, row.Total.TotalCalls)
	assert.EqualValues(t, 10, row.Total.UnlinkedRefundQuota)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-provider-model")
	assert.NotContains(t, string(encoded), "other-user")
}

func TestUsageDiscountContractStatesAndAuxiliaryFees(t *testing.T) {
	cases := []struct {
		name, other, state string
		original           bool
	}{
		{"no", `{"group_ratio":0.8,"contract_applicable":false,"model_ratio":1}`, "no", true},
		{"legacy", `{"group_ratio":0.8,"model_ratio":1}`, "unrecorded", true},
		{"incomplete", `{"group_ratio":0.8,"contract_applicable":true,"model_ratio":1}`, "unknown", false},
		{"conflicting", `{"group_ratio":0.8,"contract_applicable":false,"contract_discount":0.5,"model_ratio":1}`, "unknown", false},
		{"fees", `{"group_ratio":0.8,"contract_discount":0.5,"fee_quota":10,"model_ratio":1}`, "yes", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUsageAnalyticsTestDB(t)
			period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
			require.NoError(t, err)
			require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, ModelName: "m", CreatedAt: period.StartTimestamp + 1, Type: LogTypeConsume, Quota: 40, Other: tc.other}).Error)
			view, err := GetUsageCustomerView(context.Background(), period, 7)
			require.NoError(t, err)
			require.Len(t, view.Keys, 1)
			require.Len(t, view.Keys[0].Models[0].DiscountCombinations, 1)
			combo := view.Keys[0].Models[0].DiscountCombinations[0]
			assert.Equal(t, tc.state, combo.ContractApplicable)
			assert.Equal(t, tc.original, combo.OriginalKnown)
			assert.EqualValues(t, 40, combo.Usage.NetQuota)
			if !tc.original {
				assert.Nil(t, combo.OriginalQuota)
				assert.Nil(t, combo.DiscountQuota)
			}
		})
	}
}

func TestUsageDiscountTaskRefundFollowsFinishDayAndRequiresCompleteMoney(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "incomplete"}[complete], func(t *testing.T) {
			db := setupUsageAnalyticsTestDB(t)
			period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
			require.NoError(t, err)
			task := Task{ID: 100, TaskID: "discount-task", UserId: 7, ChannelId: 21, Platform: "video", Status: TaskStatusSuccess, FinishTime: period.StartTimestamp + 10, BillingState: TaskBillingStateSettled, Quota: 20, Properties: Properties{OriginModelName: "video"}, PrivateData: TaskPrivateData{TokenId: 11}}
			require.NoError(t, db.Create(&task).Error)
			other := `{"group_ratio":0.8,"contract_discount":0.5,"contract_id":3,"contract_version":1,"contract_name":"Historical contract","model_ratio":1}`
			require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, Group: "historical", RequestId: "task-billing:100:create", CreatedAt: period.StartTimestamp - 100, Type: LogTypeConsume, Quota: 40, Other: other}).Error)
			if complete {
				require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, Group: "historical", RequestId: "task-billing:100:adjustment", CreatedAt: period.EndTimestamp + 100, Type: LogTypeRefund, Quota: 20, Other: other}).Error)
			}
			view, err := GetUsageCustomerView(context.Background(), period, 7)
			require.NoError(t, err)
			require.Len(t, view.Keys, 1)
			row := view.Keys[0].Models[0]
			assert.EqualValues(t, 1, row.Total.TotalCalls)
			assert.EqualValues(t, 20, row.Total.NetQuota)
			if complete {
				require.Len(t, row.DiscountCombinations, 1)
				combo := row.DiscountCombinations[0]
				assert.Equal(t, "historical", combo.GroupName)
				assert.Equal(t, "video", combo.ModelName)
				assert.EqualValues(t, 20, combo.Usage.RefundQuota)
				require.NotNil(t, combo.OriginalQuota)
				assert.EqualValues(t, 50, *combo.OriginalQuota)
				assert.EqualValues(t, 30, *combo.DiscountQuota)
			} else {
				assert.Empty(t, row.DiscountCombinations)
				assert.EqualValues(t, 1, row.Total.RowsMissingMoney)
			}
			next, err := ResolveUsageAnalyticsPeriod("day", "2026-09-17", 0)
			require.NoError(t, err)
			later, err := GetUsageCustomerView(context.Background(), next, 7)
			require.NoError(t, err)
			assert.Empty(t, later.Keys)
		})
	}
}

func TestUsageDiscountBatchUsesTerminalScopeWithoutCountingLogsAsCalls(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", 0)
	require.NoError(t, err)
	require.NoError(t, db.Create(&BatchJob{Id: "batch-discount", UserId: 7, TokenId: 11, ChannelId: 21, PublicModel: "public-batch", UpstreamStatus: "completed", CompletedAt: period.StartTimestamp + 20, CountTotal: 3, CountCompleted: 3, SettleState: BatchSettleSettled, DeliveryState: BatchDeliveryReady}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, ChannelId: 21, Group: "batch-group", RequestId: "batch-discount", CreatedAt: period.EndTimestamp + 100, Type: LogTypeConsume, Quota: 40, Other: `{"billing_mode":"azure_batch","batch_line_count":3,"group_ratio":0.8,"contract_discount":0.5,"model_ratio":1}`}).Error)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)
	require.Len(t, view.Keys, 1)
	row := view.Keys[0].Models[0]
	assert.EqualValues(t, 3, row.Total.TotalCalls)
	require.Len(t, row.DiscountCombinations, 1)
	combo := row.DiscountCombinations[0]
	assert.Equal(t, "batch-group", combo.GroupName)
	assert.Equal(t, "public-batch", combo.ModelName)
	assert.EqualValues(t, 40, combo.Usage.NetQuota)
	require.NotNil(t, combo.OriginalQuota)
	assert.EqualValues(t, 100, *combo.OriginalQuota)
}

func TestUsageDiscountCombinationLimitPreservesMoney(t *testing.T) {
	db := setupUsageAnalyticsTestDB(t)
	period, err := ResolveUsageAnalyticsPeriod("week", "2026-09-16", 0)
	require.NoError(t, err)
	// Exactly one more historical group than the published combination limit;
	// splitting across two days exercises the period merge boundary.
	logs := make([]Log, 0, billingDiscountCombinationCap+1)
	for i := 0; i <= billingDiscountCombinationCap; i++ {
		logs = append(logs, Log{UserId: 7, TokenId: 11, Group: fmt.Sprintf("historic-%d", i), ModelName: "m", CreatedAt: period.StartTimestamp + int64(i%2)*86400 + 1, Type: LogTypeConsume, Quota: 1, Other: `{"group_ratio":1,"contract_applicable":false,"model_ratio":1}`})
	}
	require.NoError(t, db.CreateInBatches(logs, 100).Error)
	view, err := GetUsageCustomerView(context.Background(), period, 7)
	require.NoError(t, err)
	require.Len(t, view.Keys, 1)
	row := view.Keys[0].Models[0]
	require.Len(t, row.DiscountCombinations, 1)
	combo := row.DiscountCombinations[0]
	assert.True(t, combo.Other)
	assert.EqualValues(t, billingDiscountCombinationCap+1, combo.Usage.NetQuota)
	assert.Equal(t, row.Total.NetQuota, combo.Usage.NetQuota)
	assert.Nil(t, combo.GroupRatio)
	assert.Nil(t, combo.ContractRatio)
	assert.Nil(t, combo.OriginalQuota)
}
