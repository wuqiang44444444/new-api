package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingCustomerStatementListAggregatesFiltersAndPaginates(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]User{
		{Id: 41, Username: "first", DisplayName: "First Customer", AffCode: "first-code"},
		{Id: 42, Username: "second", DisplayName: "Second Customer", AffCode: "second-code"},
		{Id: 43, Username: "partial", DisplayName: "Partial Customer", AffCode: "partial-code"},
	}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 41, CreatedAt: 1100, Type: LogTypeConsume, Quota: 800, PromptTokens: 10, Other: `{"group_ratio":0.8,"model_ratio":1}`},
		{UserId: 41, CreatedAt: 1200, Type: LogTypeRefund, Quota: 100, Other: `{"admin_info":{"statement_snapshot":{"billing_mode":"token","group_ratio":0.8,"model_ratio":1}}}`},
		{UserId: 42, CreatedAt: 1300, Type: LogTypeConsume, Quota: 500, PromptTokens: 5, Other: `{"group_ratio":1,"model_ratio":1}`},
		{UserId: 43, CreatedAt: 1400, Type: LogTypeConsume, Quota: 300, PromptTokens: 3, Other: `{"model_ratio":1}`},
	}).Error)

	result, err := GetBillingCustomerStatementList(1000, 1500, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	assert.Equal(t, 41, result.Items[0].UserId)
	assert.EqualValues(t, 700, result.Items[0].Usage.NetQuota)
	require.NotNil(t, result.Items[0].OriginalQuota)
	assert.EqualValues(t, 1000, *result.Items[0].OriginalQuota)
	require.NotNil(t, result.Items[0].DiscountQuota)
	assert.EqualValues(t, 200, *result.Items[0].DiscountQuota)
	assert.EqualValues(t, 3, result.Summary.CustomerCount)
	assert.EqualValues(t, 1500, result.Summary.Usage.NetQuota)
	assert.Nil(t, result.Summary.OriginalQuota)
	require.NotNil(t, result.Summary.DataQuality)
	assert.Equal(t, "partial", result.Summary.DataQuality.Status)

	complete, err := GetBillingCustomerStatementList(1000, 1500, "", "complete", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, complete.Items, 2)
	require.NotNil(t, complete.Summary.OriginalQuota)
	assert.EqualValues(t, 1500, *complete.Summary.OriginalQuota)
	assert.EqualValues(t, 200, *complete.Summary.DiscountQuota)

	searched, err := GetBillingCustomerStatementList(1000, 1500, "second", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, searched.Items, 1)
	assert.Equal(t, 42, searched.Items[0].UserId)

	paged, err := GetBillingCustomerStatementList(1000, 1500, "", "", "net_quota", "desc", 2, 1)
	require.NoError(t, err)
	require.Len(t, paged.Items, 1)
	assert.Equal(t, 42, paged.Items[0].UserId)
	assert.EqualValues(t, 3, paged.Total)
}

func TestBillingCustomerStatementListKeepsUnknownOriginalAmountLast(t *testing.T) {
	known := int64(10)
	items := []BillingCustomerStatementListItem{
		{UserId: 1, OriginalQuota: nil},
		{UserId: 2, OriginalQuota: &known},
	}

	sortBillingCustomerStatementList(items, "original_quota", "desc")
	assert.Equal(t, 2, items[0].UserId)
	assert.Equal(t, 1, items[1].UserId)
}
