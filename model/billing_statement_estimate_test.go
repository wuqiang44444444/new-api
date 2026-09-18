package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingStatementEstimateReasonsPreserveMoneyAcrossProjections(t *testing.T) {
	original := int64(100)
	for _, tc := range []struct {
		name, other string
		reasons     []string
		original    *int64
	}{
		{"no discount", `{"group_ratio":1,"contract_applicable":false}`, nil, &original},
		{"no historical contract", `{"group_ratio":1}`, nil, &original},
		{"incomplete contract", `{"group_ratio":1,"contract_id":7}`, []string{BillingEstimateMissingContract}, nil},
		{"invalid contract factor", `{"group_ratio":1,"contract_discount":"bad"}`, []string{BillingEstimateMissingContract}, nil},
		{"missing group", `{"contract_applicable":false}`, []string{BillingEstimateMissingGroup}, nil},
		{"both missing", `{}`, []string{BillingEstimateMissingGroup}, nil},
		{"conflict", `{"group_ratio":1,"contract_applicable":false,"contract_discount":0.8}`, []string{BillingEstimateInvalidFacts}, nil},
		{"unreadable", `{`, []string{BillingEstimateInvalidFacts}, nil},
		{"auxiliary", `{"group_ratio":1,"contract_applicable":false,"fee_quota":10}`, []string{BillingEstimateAuxiliaryCharge}, nil},
		{"overflow", `{"group_ratio":0.000000000000000001,"contract_applicable":false}`, []string{BillingEstimateAmountOutOfRange}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: tc.other}).Error)
			statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
			require.NoError(t, err)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			require.Len(t, statement.DiscountCombinations, 1)
			assert.EqualValues(t, 100, statement.Summary.NetQuota)
			assert.Equal(t, tc.original, statement.OriginalQuota)
			assert.Equal(t, tc.reasons, statement.EstimateReasons)
			assert.Equal(t, tc.reasons, statement.Groups[0].EstimateReasons)
			assert.Equal(t, tc.reasons, statement.Groups[0].Models[0].EstimateReasons)
			assert.Equal(t, tc.reasons, statement.DiscountCombinations[0].EstimateReasons)
			rows, err := BuildCustomerExportRows(context.Background(), []customerExportScanRow{{UserId: 7, TokenId: 4, ModelName: "model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: tc.other}}, "", "", nil)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, tc.reasons, rows[0].EstimateReasons)
			assert.EqualValues(t, 100, rows[0].Quota)
			if tc.original != nil {
				require.NotNil(t, statement.DiscountQuota)
				assert.Zero(t, *statement.DiscountQuota)
				assert.Equal(t, "100.0000", rows[0].OriginalEstimate)
			} else {
				assert.Empty(t, rows[0].OriginalEstimate)
			}
		})
	}
}

func TestBillingStatementEstimateReasonsUnionWithoutPartialTotals(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	for _, row := range []Log{
		{UserId: 7, TokenId: 4, ModelName: "contract", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: `{"group_ratio":1,"contract_id":7}`},
		{UserId: 7, TokenId: 5, ModelName: "auxiliary", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: `{"group_ratio":1,"contract_applicable":false,"fee_quota":1}`},
		{UserId: 7, TokenId: 5, ModelName: "known", Type: LogTypeConsume, CreatedAt: 1100, Quota: 100, Other: `{"group_ratio":1,"contract_applicable":false}`},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{BillingEstimateAuxiliaryCharge, BillingEstimateMissingContract}, statement.EstimateReasons)
	assert.Nil(t, statement.OriginalQuota)
	assert.Nil(t, statement.DiscountQuota)
	assert.EqualValues(t, 300, statement.Summary.NetQuota)
	frozen, err := FreezeBillingStatementProjection(statement)
	require.NoError(t, err)
	version := &BillingStatementVersion{SummaryProjection: frozen}
	restored, err := ReadBillingStatementProjection(version, "api_key", nil, "", "")
	require.NoError(t, err)
	assert.Equal(t, statement.EstimateReasons, restored.EstimateReasons)
	filtered, err := ReadBillingStatementProjection(version, "api_key", nil, "known", "")
	require.NoError(t, err)
	assert.Empty(t, filtered.EstimateReasons)
	require.NotNil(t, filtered.OriginalQuota)
	assert.EqualValues(t, 100, *filtered.OriginalQuota)

}

func TestBillingStatementZeroAmountAndRefundEstimates(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	for _, tc := range []struct {
		name, other string
		kind, quota int
		original    int64
		export      string
	}{
		{"free", `{}`, LogTypeConsume, 0, 0, "0"},
		{"refund", `{"group_ratio":0.5,"contract_applicable":false}`, LogTypeRefund, 100, -200, "-200.0000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := Log{UserId: 7, TokenId: 4, ModelName: tc.name, Type: tc.kind, CreatedAt: 1100, Quota: tc.quota, Other: tc.other}
			require.NoError(t, db.Create(&row).Error)
			statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, tc.name, "")
			require.NoError(t, err)
			require.NotNil(t, statement.OriginalQuota)
			assert.Equal(t, tc.original, *statement.OriginalQuota)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			require.NotNil(t, statement.Groups[0].Models[0].OriginalQuota)
			assert.Empty(t, statement.EstimateReasons)
			exported, err := BuildCustomerExportRows(context.Background(), []customerExportScanRow{{UserId: 7, TokenId: 4, ModelName: tc.name, Type: tc.kind, CreatedAt: 1100, Quota: tc.quota, Other: tc.other}}, "", "", nil)
			require.NoError(t, err)
			require.Len(t, exported, 1)
			assert.Equal(t, tc.export, exported[0].OriginalEstimate)
			assert.Empty(t, exported[0].EstimateReasons)
		})
	}
}

func TestBillingStatementSavingsAgreeAtEveryLevelWithoutContractRecords(t *testing.T) {
	for _, tc := range []struct {
		name, other            string
		kind                   int
		original, net, savings int64
	}{
		{"full price", `{"group_ratio":1}`, LogTypeConsume, 100, 100, 0},
		{"group discount", `{"group_ratio":0.5}`, LogTypeConsume, 200, 100, 100},
		{"contract and group", `{"group_ratio":0.5,"contract_discount":0.8}`, LogTypeConsume, 250, 100, 150},
		{"group refund", `{"group_ratio":0.5}`, LogTypeRefund, -200, -100, -100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ModelName: "model", Type: tc.kind, CreatedAt: 1100, Quota: 100, Other: tc.other}).Error)
			statement, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 0, "", "")
			require.NoError(t, err)
			require.Len(t, statement.Groups, 1)
			require.Len(t, statement.Groups[0].Models, 1)
			require.Len(t, statement.DiscountCombinations, 1)
			for _, amounts := range [][3]*int64{
				{statement.OriginalQuota, statement.DiscountQuota, &statement.Summary.NetQuota},
				{statement.Groups[0].OriginalQuota, statement.Groups[0].DiscountQuota, &statement.Groups[0].Usage.NetQuota},
				{statement.Groups[0].Models[0].OriginalQuota, statement.Groups[0].Models[0].DiscountQuota, &statement.Groups[0].Models[0].Usage.NetQuota},
				{statement.DiscountCombinations[0].OriginalQuota, statement.DiscountCombinations[0].DiscountQuota, &statement.DiscountCombinations[0].Usage.NetQuota},
			} {
				require.NotNil(t, amounts[0])
				require.NotNil(t, amounts[1])
				assert.Equal(t, tc.original, *amounts[0])
				assert.Equal(t, tc.savings, *amounts[1])
				assert.Equal(t, tc.net, *amounts[2])
			}
			assert.Empty(t, statement.EstimateReasons)
			// Earlier confirmed projections may omit model savings. Preserve that
			// frozen fact; improvements require a new correction version.
			statement.Groups[0].Models[0].DiscountQuota = nil
			frozen, err := FreezeBillingStatementProjection(statement)
			require.NoError(t, err)
			version := &BillingStatementVersion{SummaryProjection: frozen}
			for _, filter := range []string{"", "model"} {
				restored, err := ReadBillingStatementProjection(version, "api_key", nil, filter, "")
				require.NoError(t, err)
				assert.Nil(t, restored.Groups[0].Models[0].DiscountQuota)
			}
			assert.Equal(t, frozen, version.SummaryProjection)
		})
	}
}

func TestBillingStatementSavingsDoNotWrapOnSubtraction(t *testing.T) {
	original := int64(9223372036854775807)
	assert.Nil(t, BillingStatementEstimatedSavings(&original, -1))
	original = -9223372036854775807
	assert.Nil(t, BillingStatementEstimatedSavings(&original, 2))
	assert.Nil(t, BillingStatementEstimatedSavings(nil, 100))
}

// name/count/price alone do not freeze historical quota conversion or rounding.
func TestHistoricalToolSurchargeDoesNotInventConversion(t *testing.T) {
	row := CustomerBillingLogRow(&Log{Type: LogTypeConsume, Quota: 123, Other: `{"group_ratio":0.5,"contract_applicable":false,"tool_surcharges":[{"name":"web_search","count":2,"price":10}]}`})
	assert.EqualValues(t, 123, row.Quota)
	assert.Empty(t, row.OriginalEstimate)
	assert.Contains(t, row.EstimateReasons, BillingEstimateAuxiliaryCharge)
}
