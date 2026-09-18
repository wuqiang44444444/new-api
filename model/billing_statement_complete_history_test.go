package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatementRefundRecoveryAcrossCompleteKeyHistory(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprint("duplicate=", duplicate), func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
			original := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Type: LogTypeConsume, CreatedAt: 800, Quota: 87, Other: `{"group_ratio":0.87,"statement_snapshot":{"billing_mode":"token"}}`}
			require.NoError(t, db.Create(&original).Error)
			// A real regression boundary: unrelated older refunds must not suppress
			// an explicit reference when the key passes the former 1,000-row limit.
			history := make([]Log, 1001)
			for i := range history {
				history[i] = Log{UserId: 7, TokenId: 4, Type: LogTypeRefund, CreatedAt: 100, Other: `{}`}
			}
			require.NoError(t, db.CreateInBatches(history, 100).Error)
			refund := Log{UserId: 7, TokenId: 4, ChannelId: 3, ModelName: "video", Type: LogTypeRefund, CreatedAt: 1000, Quota: 87, Other: fmt.Sprintf(`{"admin_info":{"original_preauth_log_id":%d}}`, original.Id)}
			require.NoError(t, db.Create(&refund).Error)
			if duplicate {
				extra := refund
				extra.Id = 0
				extra.CreatedAt = 2000
				require.NoError(t, db.Create(&extra).Error)
			}
			require.NoError(t, migrateBillingStatementLogIndex(db))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			statement, err := GetBillingCustomerStatement(context.Background(), 7, 900, 1100, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, -87, statement.Summary.NetQuota)
			details, err := GetBillingStatementLogs(context.Background(), BillingStatementLogFilter{UserId: 7, Start: 900, End: 1100}, 1, 20, common.RoleCommonUser)
			require.NoError(t, err)
			require.Len(t, details.Items, 1)
			batch, err := NextCustomerExportLogBatch(context.Background(), CustomerExportBatchParams{UserId: 7, StartTimestamp: 900, EndTimestamp: 1101, Limit: 1, StatementScope: true})
			require.NoError(t, err)
			exported, err := BuildCustomerExportRows(context.Background(), batch, "", "", nil)
			require.NoError(t, err)
			require.Len(t, exported, 1)
			row := CustomerBillingLogRow(details.Items[0])
			for _, fact := range []CustomerExportRow{row, exported[0]} {
				if duplicate {
					assert.Equal(t, "unknown", fact.BillingMode)
					assert.Empty(t, fact.OriginalEstimate)
				} else {
					assert.Equal(t, "token", fact.BillingMode)
					assert.Equal(t, "-100.0000", fact.OriginalEstimate)
				}
				assert.EqualValues(t, 87, fact.Quota)
				assert.Zero(t, fact.RequestCount)
			}
			if duplicate {
				assert.Nil(t, statement.OriginalQuota)
			} else {
				require.NotNil(t, statement.OriginalQuota)
				assert.EqualValues(t, -100, *statement.OriginalQuota)
				assert.Zero(t, statement.DataQuality.UnknownBillingModeRequests)
				assert.Zero(t, statement.DataQuality.MissingHistoricalPriceRows)
			}
		})
	}
}

func TestStatementHistoricalOpenAIInputEvidence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  map[string]any
		total   int64
		unknown bool
	}{
		{name: "frozen text path", total: 1000},
		{name: "explicit anthropic wins", change: map[string]any{"usage_semantic": "anthropic"}, total: 1300},
		{name: "explicit total wins", change: map[string]any{"input_tokens_total": 1700}, total: 1700},
		{name: "invalid explicit total", change: map[string]any{"input_tokens_total": "invalid"}, unknown: true},
		{name: "invalid explicit semantic", change: map[string]any{"usage_semantic": 42}, unknown: true},
		{name: "no path", change: map[string]any{"request_path": nil}, unknown: true},
		{name: "no chain", change: map[string]any{"request_conversion": nil}, unknown: true},
		{name: "no billing path", change: map[string]any{"admin_info": nil}, unknown: true},
		{name: "Claude final override", change: map[string]any{"claude": true}, unknown: true},
		{name: "invalid Claude flag", change: map[string]any{"claude": "false"}, unknown: true},
		{name: "Claude conversion", change: map[string]any{"request_conversion": []string{"OpenAI Compatible", "Claude Messages"}}, unknown: true},
		{name: "OpenAI cache creation", change: map[string]any{"cache_creation_tokens": 10}, total: 1000},
		{name: "OpenAI zero creation", change: map[string]any{"cache_creation_tokens": 0}, total: 1000},
		{name: "cache exceeds input", change: map[string]any{"cache_tokens": 1001}, unknown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := map[string]any{"request_path": "/v1/chat/completions", "request_conversion": []string{"OpenAI Compatible"}, "admin_info": map[string]any{"usage_billing_path": "upstream"}, "cache_tokens": 300, "group_ratio": 0.5}
			for k, v := range tc.change {
				if v == nil {
					delete(other, k)
				} else {
					other[k] = v
				}
			}
			raw, err := common.Marshal(other)
			require.NoError(t, err)
			row := CustomerBillingLogRow(&Log{Type: LogTypeConsume, ModelName: "any-customer-model", PromptTokens: 1000, Quota: 100, Other: string(raw)})
			assert.Equal(t, tc.unknown, row.InputTokensUnavailable)
			assert.Equal(t, tc.total, row.InputTokens)
			assert.EqualValues(t, 100, row.Quota)
			assert.Equal(t, "200.0000", row.OriginalEstimate)
		})
	}
}

func TestStatementRefundReferenceScanHonorsCancellation(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, Type: LogTypeRefund, Other: `{"admin_info":{"original_preauth_log_id":1}}`}).Error)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	counts, err := countBillingStatementRefundReferences(ctx, 7, 4, []int64{1})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, counts, "a cancelled scan cannot certify reference uniqueness")
}
