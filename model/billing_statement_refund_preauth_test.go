package model

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preauthTestPeriod() (int64, int64) {
	return 1000, 1200
}

func findStatementModelRow(t *testing.T, statement BillingCustomerStatement, tokenId int64, mode string) BillingReconciliationModelSummary {
	t.Helper()
	for _, group := range statement.Groups {
		if group.Id != tokenId {
			continue
		}
		for _, item := range group.Models {
			if item.BillingMode == mode {
				return item
			}
		}
	}
	t.Fatalf("statement model row not found: token=%d mode=%s", tokenId, mode)
	return BillingReconciliationModelSummary{}
}

func TestManualRefundRecoversFactsFromOriginalPreauth(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	periodStart, periodEnd := preauthTestPeriod()
	original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeConsume,
		CreatedAt: periodStart + 10, Quota: 87,
		Other: `{"contract_applicable":false,"statement_snapshot":{"snapshot_version":1,"billing_mode":"per_second","provider_model":"v","customer_model":"seedance-2-0-m","group_ratio":0.87},"group_ratio":0.87}`}
	require.NoError(t, db.Create(&original).Error)
	require.NotZero(t, original.Id)
	refund := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeRefund,
		CreatedAt: periodStart + 20, Quota: 87,
		Other: `{"admin_info":{"original_preauth_log_id":` + strconv.FormatInt(int64(original.Id), 10) + `}}`}
	require.NoError(t, db.Create(&refund).Error)

	statement, err := GetBillingCustomerStatement(context.Background(), 7, periodStart, periodEnd, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 1)
	require.Len(t, statement.Groups[0].Models, 1)
	item := findStatementModelRow(t, statement, 4, BillingReconciliationModePerSecond)
	assert.Equal(t, BillingReconciliationModePerSecond, item.BillingMode)
	// 折前链条整体恢复：87 / 0.87 = 100（消费）+ 100（退款负计）= 0，
	// 原价与优惠不再是“无法完整计算”的空值。
	require.NotNil(t, item.OriginalQuota)
	assert.EqualValues(t, 0, *item.OriginalQuota)
	assert.EqualValues(t, 0, item.Usage.NetQuota)
	require.NotNil(t, statement.DataQuality)
	assert.Zero(t, statement.DataQuality.UnknownBillingModeRequests)
	assert.Zero(t, statement.DataQuality.MissingHistoricalPriceRows)
}

func TestManualRefundIdentityMismatchStaysUnknown(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	periodStart, periodEnd := preauthTestPeriod()
	original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeConsume,
		CreatedAt: periodStart + 10, Quota: 87,
		Other: `{"contract_applicable":false,"statement_snapshot":{"snapshot_version":1,"billing_mode":"per_second","provider_model":"v","customer_model":"seedance-2-0-m","group_ratio":0.87},"group_ratio":0.87}`}
	require.NoError(t, db.Create(&original).Error)
	// TokenId 与原预扣不一致：身份校验失败关闭，保持未知。
	refund := Log{UserId: 7, TokenId: 9, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeRefund,
		CreatedAt: periodStart + 20, Quota: 87,
		Other: `{"admin_info":{"original_preauth_log_id":` + strconv.FormatInt(int64(original.Id), 10) + `}}`}
	require.NoError(t, db.Create(&refund).Error)

	statement, err := GetBillingCustomerStatement(context.Background(), 7, periodStart, periodEnd, "api_key", 0, "", "")
	require.NoError(t, err)
	item := findStatementModelRow(t, statement, 9, BillingReconciliationModeUnknown)
	assert.Nil(t, item.OriginalQuota)
	assert.EqualValues(t, statement.DataQuality.UnknownBillingModeRequests, 1)
}

func TestManualRefundSharedReferenceAndPartialRefundStayUnknown(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	periodStart, periodEnd := preauthTestPeriod()
	original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeConsume,
		CreatedAt: periodStart + 10, Quota: 87,
		Other: `{"contract_applicable":false,"statement_snapshot":{"snapshot_version":1,"billing_mode":"per_second","provider_model":"v","customer_model":"seedance-2-0-m","group_ratio":0.87},"group_ratio":0.87}`}
	require.NoError(t, db.Create(&original).Error)
	reference := `{"admin_info":{"original_preauth_log_id":` + strconv.FormatInt(int64(original.Id), 10) + `}}`
	logs := []Log{
		// 两条退款引用同一原预扣：归属冲突，双双保持未知。
		{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeRefund,
			CreatedAt: periodStart + 20, Quota: 87, Other: reference},
		{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeRefund,
			CreatedAt: periodStart + 21, Quota: 87, Other: reference},
	}
	require.NoError(t, db.Create(&logs).Error)

	statement, err := GetBillingCustomerStatement(context.Background(), 7, periodStart, periodEnd, "api_key", 0, "", "")
	require.NoError(t, err)
	item := findStatementModelRow(t, statement, 4, BillingReconciliationModeUnknown)
	assert.Nil(t, item.OriginalQuota)
	assert.EqualValues(t, 2, statement.DataQuality.UnknownBillingModeRequests)
}

func TestManualRefundMissingReferenceKeepsUnknown(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	periodStart, periodEnd := preauthTestPeriod()
	refund := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "seedance-2-0-m", Type: LogTypeRefund,
		CreatedAt: periodStart + 20, Quota: 87,
		Other: `{"admin_info":{"manual_refund":true}}`}
	require.NoError(t, db.Create(&refund).Error)

	statement, err := GetBillingCustomerStatement(context.Background(), 7, periodStart, periodEnd, "api_key", 0, "", "")
	require.NoError(t, err)
	item := findStatementModelRow(t, statement, 4, BillingReconciliationModeUnknown)
	assert.Nil(t, item.OriginalQuota)
}
