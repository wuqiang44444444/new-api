package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementUsesPublicModelForAggregationAndDetail(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	for _, row := range []Log{
		{UserId: 7, TokenId: 4, ModelName: "price-model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 80, Other: `{"group_ratio":1,"model_ratio":1,"contract_discount":0.8,"admin_info":{"statement_snapshot":{"customer_model":"public-a"}}}`},
		{UserId: 7, TokenId: 4, ModelName: "price-model", Type: LogTypeConsume, CreatedAt: 1101, Quota: 50, Other: `{"group_ratio":1,"model_ratio":1,"contract_discount":0.5,"admin_info":{"statement_snapshot":{"customer_model":"public-b"}}}`},
		{UserId: 8, TokenId: 4, ModelName: "price-model", Type: LogTypeConsume, CreatedAt: 1102, Quota: 999, Other: `{"group_ratio":1,"model_ratio":1,"admin_info":{"statement_snapshot":{"customer_model":"public-a"}}}`},
		{UserId: 7, TokenId: 4, ModelName: "historical-model", Type: LogTypeRefund, CreatedAt: 1103, Quota: 10, Other: `{"group_ratio":1,"model_ratio":1}`},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	s, err := GetBillingCustomerStatement(7, 1000, 1200, "api_key", 4, "", "")
	require.NoError(t, err)
	require.Len(t, s.Groups, 1)
	assert.Len(t, s.Groups[0].Models, 3)
	assert.EqualValues(t, 120, s.Summary.NetQuota)
	filtered, err := GetBillingCustomerStatement(7, 1000, 1200, "api_key", 4, "public-a", "token")
	require.NoError(t, err)
	assert.EqualValues(t, 80, filtered.Summary.NetQuota)
	tokenID := 4
	detail, err := GetBillingStatementLogs(BillingStatementLogFilter{UserId: 7, Start: 1000, End: 1200, TokenId: &tokenID, ModelName: "public-a", BillingMode: "token"}, 1, 10, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, 80, detail.Quota)
	require.Len(t, detail.Items, 1)
	assert.Equal(t, "public-a", detail.Items[0].ModelName)
	assert.NotContains(t, detail.Items[0].Other, "admin_info")
}

func TestCustomerStatementNormalizesTotalInputAcrossUsageSemantics(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	for _, tc := range []struct {
		prompt int
		other  string
	}{
		{1000, `{"usage_semantic":"openai","cache_tokens":900,"group_ratio":1,"model_ratio":1}`},
		{100, `{"usage_semantic":"anthropic","cache_tokens":800,"cache_write_tokens":100,"group_ratio":1,"model_ratio":1}`},
		{100, `{"input_tokens_total":1000,"cache_tokens":900,"group_ratio":1,"model_ratio":1}`},
	} {
		require.NoError(t, db.Create(&Log{UserId: 7, Type: LogTypeConsume, CreatedAt: 1100, PromptTokens: tc.prompt, Quota: 10, Other: tc.other}).Error)
	}
	s, err := GetBillingCustomerStatement(7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 3000, s.Summary.InputTokens)
	assert.EqualValues(t, 2600, s.Summary.CacheReadTokens)
	assert.EqualValues(t, 100, s.Summary.CacheWriteTokens)
	assert.EqualValues(t, 30, s.Summary.NetQuota)
}

func TestCustomerStatementMarksUninterpretableCachedInputWithoutLosingMoney(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, Type: LogTypeConsume, CreatedAt: 1100, PromptTokens: 100, Quota: 80, Other: `{"cache_tokens":900,"group_ratio":1,"model_ratio":1}`}).Error)
	s, err := GetBillingCustomerStatement(7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 80, s.Summary.NetQuota)
	assert.Equal(t, "partial", s.DataQuality.Status)
	encoded, err := common.Marshal(s.DataQuality)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"input_tokens_unavailable_requests":1`)
}
