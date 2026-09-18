package model

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementSplitsDiscountCombinations(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	logs := []Log{
		// Contract combination: 100 / 0.5 / 0.3 = 666.66...
		{UserId: 7, TokenId: 4, ModelName: "model-a", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100,
			Other: `{"group_ratio":0.5,"contract_discount":0.3,"contract_id":5,"contract_version":2,"contract_name":"annual"}`},
		// No contract discount: 100 / 0.5 = 200. Legacy row has no explicit
		// applicability marker; unrecorded contracts use only the group factor.
		{UserId: 7, TokenId: 4, ModelName: "model-a", Type: LogTypeConsume, CreatedAt: 1101, Quota: 100,
			Other: `{"group_ratio":0.5}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.DiscountCombinations, 2)

	var contract *BillingDiscountCombination
	for i := range statement.DiscountCombinations {
		if statement.DiscountCombinations[i].ContractApplicable == "yes" {
			contract = &statement.DiscountCombinations[i]
		}
	}
	require.NotNil(t, contract, "contract combination row")
	assert.Equal(t, "yes", contract.ContractApplicable)
	assert.Equal(t, "annual", contract.ContractName)
	assert.True(t, contract.ContractIdKnown)
	assert.EqualValues(t, 5, contract.ContractId)
	assert.EqualValues(t, 2, contract.ContractVersion)
	require.NotNil(t, contract.GroupRatio)
	assert.InDelta(t, 0.5, *contract.GroupRatio, 1e-9)
	require.NotNil(t, contract.OriginalQuota)
	assert.EqualValues(t, 667, *contract.OriginalQuota)
	assert.EqualValues(t, 100, contract.Usage.NetQuota)
	require.NotNil(t, contract.DiscountQuota)
	assert.EqualValues(t, 567, *contract.DiscountQuota)
	assert.True(t, contract.OriginalKnown)

	var legacy *BillingDiscountCombination
	for i := range statement.DiscountCombinations {
		if statement.DiscountCombinations[i].ContractApplicable == "unrecorded" {
			legacy = &statement.DiscountCombinations[i]
		}
	}
	require.NotNil(t, legacy, "legacy combination row")
	assert.Equal(t, "unrecorded", legacy.ContractApplicable)
	assert.False(t, legacy.ContractIdKnown)
	require.NotNil(t, legacy.OriginalQuota)
	assert.EqualValues(t, 200, *legacy.OriginalQuota)
	assert.True(t, legacy.OriginalKnown)
	require.NotNil(t, statement.OriginalQuota)
	assert.EqualValues(t, 867, *statement.OriginalQuota)
	assert.EqualValues(t, 200, statement.Summary.NetQuota)
}

func TestCustomerStatementCombinationRefundKeepsNegativeSign(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	logs := []Log{
		{UserId: 7, TokenId: 4, ModelName: "video", Type: LogTypeConsume, CreatedAt: 1100, Quota: 87,
			Other: `{"group_ratio":0.87,"contract_applicable":false}`},
		{UserId: 7, TokenId: 4, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1101, Quota: 87,
			Other: `{"group_ratio":0.87,"contract_applicable":false}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.DiscountCombinations, 1)
	combo := statement.DiscountCombinations[0]
	assert.Equal(t, "no", combo.ContractApplicable)
	require.NotNil(t, combo.OriginalQuota)
	// 87 / 0.87 = 100；退款负计，净额为 0，原价与优惠互相抵消。
	assert.EqualValues(t, 0, *combo.OriginalQuota)
	assert.EqualValues(t, 0, combo.Usage.NetQuota)
	require.NotNil(t, combo.DiscountQuota)
	assert.EqualValues(t, 0, *combo.DiscountQuota)
	assert.EqualValues(t, 87, combo.Usage.GrossQuota)
	assert.EqualValues(t, 87, combo.Usage.RefundQuota)
}

func TestCustomerStatementIdentityUnknownCombinationDoesNotClaimContract(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	logs := []Log{
		// 同倍率的身份缺失旧行合并为“历史身份未记录”组合。
		{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 50,
			Other: `{"group_ratio":0.5,"contract_discount":0.3}`},
		{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1102, Quota: 50,
			Other: `{"group_ratio":0.5,"contract_discount":0.3}`},
		// 已知身份的同倍率行单独成组合，不能指定为同一份合同。
		{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1103, Quota: 60,
			Other: `{"group_ratio":0.5,"contract_discount":0.3,"contract_id":9,"contract_version":1,"contract_name":"named"}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.DiscountCombinations, 2)
	unrecorded := statement.DiscountCombinations[0]
	assert.False(t, unrecorded.ContractIdKnown)
	assert.Empty(t, unrecorded.ContractName)
	assert.EqualValues(t, 100, unrecorded.Usage.NetQuota)
	named := statement.DiscountCombinations[1]
	assert.True(t, named.ContractIdKnown)
	assert.Equal(t, "named", named.ContractName)
}

func TestCustomerStatementAuxiliaryChargeCombinationKeepsNetWithoutOriginal(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 110,
		Other: `{"group_ratio":0.5,"contract_applicable":false,"fee_quota":10}`}).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.DiscountCombinations, 1)
	combo := statement.DiscountCombinations[0]
	assert.False(t, combo.OriginalKnown)
	assert.Nil(t, combo.OriginalQuota)
	assert.Nil(t, combo.DiscountQuota)
	assert.EqualValues(t, 110, combo.Usage.NetQuota)
}

func TestCustomerStatementCombinationOverflowMergesIntoOtherRow(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	logs := make([]Log, 0, billingDiscountCombinationCap+1)
	for i := 0; i <= billingDiscountCombinationCap; i++ {
		logs = append(logs, Log{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: int64(1100 + i), Quota: 10,
			Other: `{"group_ratio":` + strconv.FormatFloat(0.1+float64(i)/1000, 'f', -1, 64) + `,"contract_applicable":false}`})
	}
	require.NoError(t, db.Create(&logs).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1500, "api_key", 0, "", "")
	require.NoError(t, err)
	require.LessOrEqual(t, len(statement.DiscountCombinations), billingDiscountCombinationCap+1)
	var overflow *BillingDiscountCombination
	netSum := int64(0)
	knownOriginal := int64(0)
	for i, combo := range statement.DiscountCombinations {
		if combo.Other {
			overflow = &statement.DiscountCombinations[i]
		}
		netSum += combo.Usage.NetQuota
		if combo.OriginalQuota != nil {
			knownOriginal += *combo.OriginalQuota
		}
	}
	require.NotNil(t, overflow)
	assert.False(t, overflow.OriginalKnown)
	assert.Nil(t, overflow.OriginalQuota)
	assert.EqualValues(t, 10, overflow.Usage.NetQuota)
	assert.EqualValues(t, statement.Summary.NetQuota, netSum)
	require.NotNil(t, statement.OriginalQuota)
	// 溢出行原价未知，但其余组合的原价之和仍小于等于账单原价，净额逐行可核对。
	assert.LessOrEqual(t, knownOriginal, *statement.OriginalQuota)
}

func TestCustomerStatementEqualDiscountsHaveStableIdentityOrder(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	logs := []Log{
		{UserId: 7, TokenId: 5, ModelName: "m", Type: LogTypeConsume, CreatedAt: 1000, Quota: 100, Other: `{"group_ratio":1,"contract_discount":0.5,"contract_id":2,"contract_version":1}`},
		{UserId: 7, TokenId: 4, ModelName: "m", Type: LogTypeConsume, CreatedAt: 1001, Quota: 100, Other: `{"group_ratio":1,"contract_discount":0.5,"contract_id":2,"contract_version":2}`},
		{UserId: 7, TokenId: 4, ModelName: "m", Type: LogTypeConsume, CreatedAt: 1002, Quota: 100, Other: `{"group_ratio":1,"contract_discount":0.5,"contract_id":2,"contract_version":1}`},
		{UserId: 7, TokenId: 4, ModelName: "m", Type: LogTypeConsume, CreatedAt: 1003, Quota: 100, Other: `{"group_ratio":1,"contract_discount":0.5,"contract_id":1,"contract_version":1}`},
		{UserId: 7, TokenId: 4, ModelName: "m", Type: LogTypeConsume, CreatedAt: 1004, Quota: 100, Other: `{"group_ratio":1,"contract_discount":0.5}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 900, 1100, "api_key", 0, "", "")
	require.NoError(t, err)
	identities := make([][3]int64, 0, len(statement.DiscountCombinations))
	for _, combo := range statement.DiscountCombinations {
		identities = append(identities, [3]int64{combo.GroupId, combo.ContractId, combo.ContractVersion})
	}
	assert.Equal(t, [][3]int64{{4, 0, 0}, {4, 1, 1}, {4, 2, 1}, {4, 2, 2}, {5, 2, 1}}, identities)
}
