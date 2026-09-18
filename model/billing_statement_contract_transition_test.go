package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementContractTransitionShowsMultipleDiscounts(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "contract starts", true: "contract ends"}[reverse], func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			logs := []Log{
				{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: `{"contract_applicable":false,"model_ratio":1,"group_ratio":1}`},
				{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1101, Quota: 80, Other: `{"model_ratio":1,"group_ratio":1,"contract_discount":0.8}`},
			}
			if reverse {
				logs[0], logs[1] = logs[1], logs[0]
			}
			require.NoError(t, db.Create(&logs).Error)
			statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
			require.NoError(t, err)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			item := statement.Groups[0].Models[0]
			require.NotNil(t, item.OriginalQuota)
			assert.EqualValues(t, 200, *item.OriginalQuota)
			assert.EqualValues(t, 180, item.Usage.NetQuota)
			assert.True(t, item.MultipleContractDiscounts)
			assert.Nil(t, item.ContractDiscountRatio)
		})
	}
}

func TestCustomerStatementWithoutContractDoesNotInventContractDiscount(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: `{"contract_applicable":false,"model_ratio":1,"group_ratio":1}`}).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 1)
	require.Len(t, statement.Groups[0].Models, 1)
	item := statement.Groups[0].Models[0]
	assert.Nil(t, item.ContractDiscountRatio)
	assert.False(t, item.MultipleContractDiscounts)
	require.NotNil(t, item.OriginalQuota)
	assert.EqualValues(t, 100, *item.OriginalQuota)
}
