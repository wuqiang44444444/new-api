package model

import (
	"encoding/base64"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementCountsTaskCreatesAndReversesPreconsumeRefunds(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 91, Username: "customer", Quota: 12345}).Error)
	for _, row := range []struct {
		typ    int
		quota  int
		other  string
		tokens int
	}{
		{LogTypeConsume, 2801400, `{"is_task":true,"task_id":"first"}`, 0},
		{LogTypeRefund, 2272483, `{"task_id":"first","pre_consumed_quota":2801400,"actual_quota":528917}`, 173700},
		{LogTypeConsume, 2801400, `{"is_task":true,"task_id":"failed"}`, 0},
		{LogTypeRefund, 2801400, `{"task_id":"failed","reason":"failed"}`, 0},
	} {
		var other map[string]any
		require.NoError(t, common.UnmarshalJsonStr(row.other, &other))
		other["group_ratio"] = 0.87
		other["model_price"] = 0
		other["billing_mode"] = "tiered_expr"
		other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(`tier("base", c * 7)`))
		other["admin_info"] = map[string]any{"statement_snapshot": map[string]any{"snapshot_version": 1, "billing_mode": "per_call", "group_ratio": 0.87}}
		encoded, err := common.Marshal(other)
		require.NoError(t, err)
		require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ModelName: "customer-video", Type: row.typ, CreatedAt: 1100, Quota: row.quota, CompletionTokens: row.tokens, Other: string(encoded)}).Error)
	}
	statement, err := GetBillingCustomerStatement(91, 1000, 1200, "api_key", 40, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 1)
	require.Len(t, statement.Groups[0].Models, 1)
	item := statement.Groups[0].Models[0]
	assert.Equal(t, BillingReconciliationModeToken, item.BillingMode)
	assert.EqualValues(t, 2, item.Usage.Requests)
	assert.EqualValues(t, 173700, item.Usage.OutputTokens)
	assert.EqualValues(t, 528917, item.Usage.NetQuota)
	require.NotNil(t, item.OriginalQuota)
	assert.EqualValues(t, 607951, *item.OriginalQuota)
	require.NotNil(t, statement.DiscountQuota)
	assert.EqualValues(t, 79034, *statement.DiscountQuota)
	list, err := GetBillingCustomerStatementList(1000, 1200, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.Equal(t, statement.Summary, list.Items[0].Usage)
	assert.Equal(t, statement.OriginalQuota, list.Items[0].OriginalQuota)
	assert.Equal(t, statement.DiscountQuota, list.Items[0].DiscountQuota)
}

func TestStatementListAggregatesSignedAmountsAcrossCustomersBeforeRounding(t *testing.T) {
	for _, tc := range []struct {
		name         string
		refund       int
		ratio        string
		wantOriginal int64
	}{
		{"refund cancels charge", 80, "0.8", 0},
		{"refund exceeds charge", 160, "0.8", -100},
		{"different discounts", 40, "0.5", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&[]User{{Id: 7, Username: "charge", AffCode: "charge"}, {Id: 8, Username: "refund", AffCode: "refund"}}).Error)
			require.NoError(t, db.Create(&[]Log{
				{UserId: 7, Type: LogTypeConsume, CreatedAt: 1100, Quota: 80, Other: `{"model_price":1,"group_ratio":0.8}`},
				{UserId: 8, Type: LogTypeRefund, CreatedAt: 1100, Quota: tc.refund, Other: `{"model_price":1,"group_ratio":` + tc.ratio + `}`},
			}).Error)
			list, err := GetBillingCustomerStatementList(1000, 1200, "", "", "net_quota", "desc", 1, 1)
			require.NoError(t, err)
			require.Len(t, list.Items, 1) // Summary includes customers outside the page.
			require.NotNil(t, list.Summary.OriginalQuota)
			assert.Equal(t, tc.wantOriginal, *list.Summary.OriginalQuota)
			assert.Equal(t, tc.wantOriginal-list.Summary.Usage.NetQuota, *list.Summary.DiscountQuota)
			filtered, err := GetBillingCustomerStatementList(1000, 1200, "charge", "", "net_quota", "desc", 1, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 100, *filtered.Summary.OriginalQuota)
		})
	}
}

func TestStatementListRoundsOnlyOnceAndRejectsAggregateOverflow(t *testing.T) {
	one := int64(1)
	for _, tc := range []struct {
		name, ratio string
		quota       int
		want        *int64
	}{
		{"fractional totals", "3", 1, &one},
		{"aggregate overflow", "0.0000000003", math.MaxInt32, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&[]Log{
				{UserId: 7, Type: LogTypeConsume, CreatedAt: 1100, Quota: tc.quota, Other: `{"model_price":1,"group_ratio":` + tc.ratio + `}`},
				{UserId: 8, Type: LogTypeConsume, CreatedAt: 1100, Quota: tc.quota, Other: `{"model_price":1,"group_ratio":` + tc.ratio + `}`},
			}).Error)
			list, err := GetBillingCustomerStatementList(1000, 1200, "", "", "net_quota", "desc", 1, 20)
			require.NoError(t, err)
			assert.Equal(t, tc.want, list.Summary.OriginalQuota)
			if tc.want == nil {
				assert.Nil(t, list.Summary.DiscountQuota)
				assert.Equal(t, "partial", list.Summary.DataQuality.Status)
			}
		})
	}
}

func TestCustomerStatementRefundNeedsFrozenPriceAndKeepsPeriodBoundary(t *testing.T) {
	refundedOriginal := int64(-100)
	for _, tc := range []struct {
		name, other  string
		wantOriginal *int64
	}{
		{"known refund", `{"model_price":1,"group_ratio":0.8}`, &refundedOriginal},
		{"missing refund price", `{"model_price":1}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			require.NoError(t, db.Create(&[]Log{
				{UserId: 7, Type: LogTypeConsume, CreatedAt: 900, Quota: 80, Other: `{"model_price":1,"group_ratio":0.8}`},
				{UserId: 7, Type: LogTypeRefund, CreatedAt: 1100, Quota: 80, Other: tc.other},
			}).Error)
			s, err := GetBillingCustomerStatement(7, 1000, 1200, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, -80, s.Summary.NetQuota)
			assert.EqualValues(t, 80, s.Summary.RefundQuota)
			assert.Equal(t, tc.wantOriginal, s.OriginalQuota)
		})
	}
}

func TestTaskRefundCountExcludesSettlementAdjustments(t *testing.T) {
	var usage BillingReconciliationUsage
	for _, other := range []string{
		`{"task_id":"success","model_price":1,"pre_consumed_quota":100,"actual_quota":80}`,
		`{"task_id":"failed","model_price":1,"reason":"failed"}`,
	} {
		log := billingReconciliationLog{Type: LogTypeRefund, Quota: 20, Other: other}
		accumulateBillingReconciliationLog(&usage, log, parseBillingReconciliationLog(log))
	}
	assert.EqualValues(t, 1, usage.RefundedCalls)
	assert.EqualValues(t, 40, usage.RefundQuota)
}

func TestStatementRefundAcrossModelsIsNettedBeforeSummaryRounding(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 1, ModelName: "new", Type: LogTypeConsume, CreatedAt: 1100, Quota: 80, Other: `{"model_price":1,"group_ratio":0.8}`},
		{UserId: 7, TokenId: 2, ModelName: "previous", Type: LogTypeRefund, CreatedAt: 1100, Quota: 80, Other: `{"model_price":1,"group_ratio":0.8}`},
	}).Error)
	s, err := GetBillingCustomerStatement(7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	require.NotNil(t, s.OriginalQuota)
	assert.Zero(t, *s.OriginalQuota)
	assert.Zero(t, *s.DiscountQuota)
	list, err := GetBillingCustomerStatementList(1000, 1200, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	assert.Equal(t, s.OriginalQuota, list.Items[0].OriginalQuota)
	assert.Equal(t, s.DiscountQuota, list.Items[0].DiscountQuota)
}

func TestStatementInvalidHistoricalRatioDoesNotInventOriginalAmount(t *testing.T) {
	for _, ratio := range []string{"NaN", "+Inf", "1e-310"} {
		t.Run(ratio, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			other, err := common.Marshal(map[string]any{"group_ratio": ratio, "model_price": 1})
			require.NoError(t, err)
			require.NoError(t, db.Create(&Log{UserId: 7, Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: string(other)}).Error)
			s, err := GetBillingCustomerStatement(7, 1000, 1200, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, 100, s.Summary.NetQuota)
			assert.Nil(t, s.OriginalQuota)
			assert.Equal(t, "partial", s.DataQuality.Status)
		})
	}
}
