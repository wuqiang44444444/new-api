package model

import (
	"context"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatementRecoveredFactsMatchDetailsExportAndCombinations(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Group: "historic-group", Type: LogTypeConsume, CreatedAt: 900, Quota: 87,
		Other: `{"admin_info":{"statement_snapshot":{"billing_mode":"per_second","customer_model":"video","provider_model":"private-upstream","user_group_ratio":0.87,"contract_applicable":false}}}`}
	require.NoError(t, db.Create(&original).Error)
	refund := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1100, Quota: 87,
		Other: `{"admin_info":{"original_preauth_log_id":` + strconv.Itoa(original.Id) + `}}`}
	require.NoError(t, db.Create(&refund).Error)
	ctx := context.Background()
	batch, err := NextCustomerExportLogBatch(ctx, CustomerExportBatchParams{UserId: 7, StartTimestamp: 1000, EndTimestamp: 1200, Limit: 100})
	require.NoError(t, err)
	exported, err := BuildCustomerExportRows(ctx, batch, "", "", nil)
	require.NoError(t, err)
	require.Len(t, exported, 1)
	expected := exported[0]
	assert.Equal(t, "per_second", expected.BillingMode)
	assert.Equal(t, "historic-group", expected.GroupName)
	assert.Equal(t, "user_exclusive", expected.GroupRatioSource)
	assert.Equal(t, "no", expected.ContractApplicable)
	assert.Equal(t, "-100.0000", expected.OriginalEstimate)
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		result, err := GetBillingStatementLogs(ctx, BillingStatementLogFilter{UserId: 7, Start: 1000, End: 1200, BillingMode: "per_second"}, 1, 20, role)
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, expected, CustomerBillingLogRow(result.Items[0]))
		assert.EqualValues(t, -87, result.Quota)
		if role == common.RoleCommonUser {
			assert.NotContains(t, result.Items[0].Other, "admin_info")
			assert.NotContains(t, result.Items[0].Other, "original_preauth_log_id")
			assert.NotContains(t, result.Items[0].Other, "private-upstream")
		}
	}
	statement, err := GetBillingCustomerStatement(ctx, 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.DiscountCombinations, 1)
	combo := statement.DiscountCombinations[0]
	assert.Equal(t, expected.GroupName, combo.GroupName)
	assert.Equal(t, expected.GroupRatioSource, combo.GroupRatioSource)
	assert.Equal(t, expected.GroupRatio, combo.GroupRatio)
	require.NotNil(t, combo.OriginalQuota)
	assert.EqualValues(t, -100, *combo.OriginalQuota)
	var stored Log
	require.NoError(t, db.First(&stored, refund.Id).Error)
	assert.Equal(t, refund.Other, stored.Other)
	assert.Empty(t, stored.Group)
}

func TestStatementDiscountCombinationsKeepFrozenGroupIdentityAndSource(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	logs := []Log{
		{Group: "first", Other: `{"group_ratio":0.5,"contract_applicable":false}`},
		{Group: "second", Other: `{"group_ratio":0.5,"contract_applicable":false}`},
		{Group: "first", Other: `{"group_ratio":1,"user_group_ratio":0.5,"contract_applicable":false}`},
		{Group: "", Other: `{"group_ratio":0.5,"contract_applicable":false}`},
	}
	for i := range logs {
		logs[i].UserId, logs[i].TokenId, logs[i].ChannelId = 7, 4, 3
		logs[i].ModelName, logs[i].Type, logs[i].CreatedAt, logs[i].Quota = "model", LogTypeConsume, 1100, 50
	}
	require.NoError(t, db.Create(&logs).Error)
	for _, dimension := range []string{"api_key", "channel"} {
		statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, dimension, 0, "", "")
		require.NoError(t, err)
		require.Len(t, statement.DiscountCombinations, 4)
		var net, original int64
		identities := make(map[string]bool)
		for _, combo := range statement.DiscountCombinations {
			identities[combo.GroupName+":"+combo.GroupRatioSource] = true
			net += combo.Usage.NetQuota
			require.NotNil(t, combo.OriginalQuota)
			original += *combo.OriginalQuota
		}
		assert.Equal(t, map[string]bool{"first:group": true, "second:group": true, "first:user_exclusive": true, ":group": true}, identities)
		assert.EqualValues(t, 200, net)
		require.NotNil(t, statement.OriginalQuota)
		assert.Equal(t, *statement.OriginalQuota, original)
	}
}

func TestStatementDetailKeepsInputSemanticAfterRedaction(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	logs := []Log{
		{ModelName: "known", Other: `{"cache_tokens":300,"admin_info":{"usage_billing_path":"billing-usage-gemini"}}`},
		{ModelName: "unknown", Other: `{"cache_tokens":300,"admin_info":{"usage_billing_path":"upstream"}}`},
	}
	for i := range logs {
		logs[i].UserId = 7
		logs[i].Type = LogTypeConsume
		logs[i].CreatedAt = 1100
		logs[i].PromptTokens = 1000
	}
	require.NoError(t, db.Create(&logs).Error)
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser} {
		response, err := GetBillingStatementLogs(context.Background(), BillingStatementLogFilter{UserId: 7, Start: 1000, End: 1200}, 1, 20, role)
		require.NoError(t, err)
		require.Len(t, response.Items, 2)
		for _, log := range response.Items {
			row := CustomerBillingLogRow(log)
			assert.Equal(t, log.ModelName == "unknown", row.InputTokensUnavailable)
			if log.ModelName == "known" {
				assert.EqualValues(t, 1000, row.InputTokens)
			}
			if role == common.RoleCommonUser {
				assert.NotContains(t, log.Other, "usage_billing_path")
			}
		}
	}
}

func TestStatementRefundConflictingGroupFactsDoNotInventHistoricalPrice(t *testing.T) {
	for _, tc := range []struct{ name, group, other string }{
		{"group identity", "different", `{"group_ratio":0.5,"contract_applicable":false}`},
		{"ratio source", "original", `{"user_group_ratio":0.5,"contract_applicable":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			original := Log{UserId: 7, TokenId: 4, ModelName: "model", Group: "original", Type: LogTypeConsume, CreatedAt: 900, Quota: 50, Other: `{"group_ratio":0.5,"contract_applicable":false}`}
			require.NoError(t, db.Create(&original).Error)
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(tc.other, &other))
			other["admin_info"] = map[string]any{"original_preauth_log_id": original.Id}
			raw, err := common.Marshal(other)
			require.NoError(t, err)
			refund := Log{UserId: 7, TokenId: 4, ModelName: "model", Group: tc.group, Type: LogTypeRefund, CreatedAt: 1100, Quota: 50, Other: string(raw)}
			require.NoError(t, db.Create(&refund).Error)
			result, err := GetBillingStatementLogs(context.Background(), BillingStatementLogFilter{UserId: 7, Start: 1000, End: 1200}, 1, 20, common.RoleCommonUser)
			require.NoError(t, err)
			require.Len(t, result.Items, 1)
			row := CustomerBillingLogRow(result.Items[0])
			assert.Equal(t, tc.group, row.GroupName)
			assert.Empty(t, row.OriginalEstimate)
			assert.EqualValues(t, -50, result.Quota)
		})
	}
}
