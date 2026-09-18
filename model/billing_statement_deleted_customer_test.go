package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeletedCustomerStatementRetainsHistoryWithoutInventingBalance(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 4, ChannelId: 8, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: `{"model_ratio":1,"group_ratio":1}`},
		{UserId: 7, TokenId: 4, ChannelId: 8, ModelName: "model", Type: LogTypeRefund, CreatedAt: 1101, Quota: 20, Other: `{"model_ratio":1,"group_ratio":1}`},
		{UserId: 8, Type: LogTypeConsume, CreatedAt: 1100, Quota: 900, TokenName: "模型测试", Content: "模型测试"},
	}).Error)
	list, err := GetBillingCustomerStatementList(context.Background(), 1000, 1200, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	for _, dimension := range []string{"api_key", "channel"} {
		statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, dimension, 0, "", "")
		require.NoError(t, err)
		assert.True(t, statement.Deleted)
		assert.Equal(t, list.Items[0].Username, statement.Username)
		assert.Nil(t, statement.CurrentBalance)
		assert.Equal(t, list.Items[0].Usage, statement.Summary)
		assert.EqualValues(t, 80, statement.Summary.NetQuota)
	}
	for _, userID := range []int{8, 999} {
		_, err := GetBillingReconciliationUserById(userID)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
}
