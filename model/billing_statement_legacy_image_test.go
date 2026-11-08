package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyImageTokenSettlementUsesTokenGroupAndDetailFilter(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 41, Username: "customer"}).Error)
	require.NoError(t, db.Create(&Log{
		UserId: 41, TokenId: 40, ModelName: "customer-image", Type: LogTypeConsume, CreatedAt: 1100,
		Quota: 2988, PromptTokens: 19, CompletionTokens: 196,
		Other: `{"task_id":"image-task","is_task":true,"task_billing_event":"create","image_count":1,"model_price":-1,"model_ratio":2.5,"group_ratio":1,"admin_info":{"statement_snapshot":{"snapshot_version":1,"billing_mode":"per_call","model_price":-1}}}`,
	}).Error)

	ctx := context.Background()
	statement, err := GetBillingCustomerStatement(ctx, 41, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 1)
	require.Len(t, statement.Groups[0].Models, 1)
	assert.Equal(t, BillingReconciliationModeToken, statement.Groups[0].Models[0].BillingMode)
	require.Len(t, statement.DiscountCombinations, 1)
	assert.Equal(t, BillingReconciliationModeToken, statement.DiscountCombinations[0].BillingMode)
	assert.EqualValues(t, 2988, statement.Summary.NetQuota)

	filter := BillingStatementLogFilter{UserId: 41, Start: 1000, End: 1200, BillingMode: BillingReconciliationModeToken}
	details, err := GetBillingStatementLogs(ctx, filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, details.Items, 1)
	assert.Equal(t, 2988, details.Items[0].Quota)
	filter.BillingMode = BillingReconciliationModePerCall
	details, err = GetBillingStatementLogs(ctx, filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.Empty(t, details.Items)
}
