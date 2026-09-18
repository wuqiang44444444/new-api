package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefundEvidenceDoesNotDependOnPageOrBatch(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	// Two keys each below the per-key evidence budget, together above it.
	originals := make([]Log, 1200)
	for i := range originals {
		originals[i] = Log{UserId: 7, TokenId: 4 + i/600, ChannelId: 3, ModelName: "video", Type: LogTypeConsume, CreatedAt: 800, Quota: 87,
			Other: `{"group_ratio":0.87,"contract_applicable":false,"statement_snapshot":{"billing_mode":"per_second"}}`}
	}
	require.NoError(t, db.CreateInBatches(&originals, 100).Error)
	refunds := make([]Log, len(originals))
	for i, original := range originals {
		refunds[i] = Log{UserId: 7, TokenId: original.TokenId, ChannelId: 3, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1000, Quota: 87,
			Other: fmt.Sprintf(`{"admin_info":{"original_preauth_log_id":%d}}`, original.Id)}
	}
	require.NoError(t, db.CreateInBatches(&refunds, 100).Error)
	statement, err := GetBillingCustomerStatement(context.Background(), 7, 900, 1100, "api_key", 0, "", "")
	require.NoError(t, err)
	require.Len(t, statement.Groups, 2)
	for _, group := range statement.Groups {
		require.Len(t, group.Models, 1)
		assert.Equal(t, BillingReconciliationModePerSecond, group.Models[0].BillingMode)
		assert.EqualValues(t, -600*87, group.Usage.NetQuota)
	}
	details, err := GetBillingStatementLogs(context.Background(), BillingStatementLogFilter{UserId: 7, Start: 900, End: 1100, BillingMode: BillingReconciliationModePerSecond}, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, 1200, details.Total)
	assert.Equal(t, statement.Summary.NetQuota, details.Quota)
	for _, size := range []int{500, 1000} {
		var cursor, net int64
		var count int
		for {
			batch, err := NextCustomerExportLogBatch(context.Background(), CustomerExportBatchParams{UserId: 7, StartTimestamp: 900, EndTimestamp: 1101, CursorId: cursor, Limit: size, StatementScope: true})
			require.NoError(t, err)
			if len(batch) == 0 {
				break
			}
			rows, err := BuildCustomerExportRows(context.Background(), batch, "", BillingReconciliationModePerSecond, nil)
			require.NoError(t, err)
			for _, row := range rows {
				net -= row.Quota
				assert.NotEmpty(t, row.OriginalEstimate)
			}
			count += len(rows)
			cursor = int64(batch[len(batch)-1].Id)
		}
		assert.Equal(t, 1200, count)
		assert.Equal(t, details.Quota, net)
	}
}

func TestRefundContradictoryContractEvidenceCannotProduceSavings(t *testing.T) {
	for _, tc := range []struct{ name, original, refund string }{
		{"refund explicitly native", `{"group_ratio":0.5,"contract_discount":"0.5"}`, `"contract_applicable":false`},
		{"original explicitly native", `{"group_ratio":0.5,"contract_applicable":false}`, `"contract_discount":"0.5"`},
		{"refund internally contradictory", `{"group_ratio":0.5,"contract_applicable":false}`, `"contract_applicable":false,"contract_discount":0.5`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "m", Type: LogTypeConsume, CreatedAt: 800, Quota: 25, Other: tc.original}
			require.NoError(t, db.Create(&original).Error)
			refund := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "m", Type: LogTypeRefund, CreatedAt: 1000, Quota: 25,
				Other: fmt.Sprintf(`{%s,"admin_info":{"original_preauth_log_id":%d}}`, tc.refund, original.Id)}
			require.NoError(t, db.Create(&refund).Error)
			statement, err := GetBillingCustomerStatement(context.Background(), 7, 900, 1100, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, -25, statement.Summary.NetQuota)
			assert.Nil(t, statement.OriginalQuota)
			assert.Nil(t, statement.DiscountQuota)
			batch, err := NextCustomerExportLogBatch(context.Background(), CustomerExportBatchParams{UserId: 7, StartTimestamp: 900, EndTimestamp: 1101})
			require.NoError(t, err)
			rows, err := BuildCustomerExportRows(context.Background(), batch, "", "", nil)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Empty(t, rows[0].OriginalEstimate)
		})
	}
}

func TestExportPreservesPlaygroundAndAllStatementFilters(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	key, channel := 0, 3
	base := Log{UserId: 7, TokenId: key, ChannelId: channel, Username: "customer", TokenName: "playground", Group: "vip", RequestId: "req", UpstreamRequestId: "up", ModelName: "m", Type: LogTypeConsume, CreatedAt: 1100, Quota: 87,
		Other: `{"group_ratio":0.87,"contract_applicable":false,"model_ratio":1}`}
	logs := []Log{base}
	for i := 0; i < 9; i++ {
		other := base
		switch i {
		case 0:
			other.TokenId = 4
		case 1:
			other.ChannelId = 5
		case 2:
			other.TokenName = "other"
		case 3:
			other.Group = "other"
		case 4:
			other.RequestId = "other"
		case 5:
			other.UpstreamRequestId = "other"
		case 6:
			other.Username = "other"
		case 7:
			other.CreatedAt = 1101
		case 8:
			other.Type = LogTypeError
		}
		logs = append(logs, other)
	}
	require.NoError(t, db.Create(&logs).Error)
	filter := BillingStatementLogFilter{UserId: 7, Start: 900, End: 1100, TokenId: &key, ChannelId: &channel, TokenName: base.TokenName, Group: base.Group, Username: base.Username, RequestId: base.RequestId, UpstreamRequestId: base.UpstreamRequestId}
	details, err := GetBillingStatementLogs(context.Background(), filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	batch, err := NextCustomerExportLogBatch(context.Background(), CustomerExportBatchParams{UserId: 7, StartTimestamp: 900, EndTimestamp: 1101, TokenId: &key, ChannelId: &channel, TokenName: base.TokenName, Group: base.Group, Username: base.Username, RequestId: base.RequestId, UpstreamRequestId: base.UpstreamRequestId, StatementScope: true})
	require.NoError(t, err)
	require.Len(t, batch, 1)
	assert.EqualValues(t, details.Total, len(batch))
	assert.EqualValues(t, details.Quota, batch[0].Quota)
	assert.Equal(t, int64(1100), batch[0].CreatedAt)
}

func TestUsageExportUsesNativeModelWildcardFilter(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "model-a", Quota: 1},
		{UserId: 7, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "model-b", Quota: 2},
		{UserId: 7, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "other", Quota: 3},
		{UserId: 8, Type: LogTypeConsume, CreatedAt: 1000, ModelName: "model-a", Quota: 4},
	}).Error)
	batch, err := NextCustomerExportLogBatch(context.Background(), CustomerExportBatchParams{UserId: 7, StartTimestamp: 900, EndTimestamp: 1100, ModelName: "model%"})
	require.NoError(t, err)
	rows, err := BuildCustomerExportRows(context.Background(), batch, "", "", nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.EqualValues(t, 3, rows[0].Quota+rows[1].Quota)
}
