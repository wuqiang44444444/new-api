package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatementClassifiesDurationAndCountWithoutChangingFunds(t *testing.T) {
	ctx := context.Background()
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)

	expression := `tier("video", u("output_seconds") + u("input_video_seconds") + u("input_image_count"))`
	units := map[string]string{"output_seconds": "second", "input_video_seconds": "second", "input_image_count": "count"}
	for _, event := range []struct {
		name      string
		logType   int
		quota     int
		timestamp int64
	}{
		{"create", LogTypeConsume, 600000, 1000},
		{"adjustment", LogTypeRefund, 440000, 1100},
	} {
		other, err := common.Marshal(map[string]any{
			"is_task": true, "task_id": "video-task", "task_billing_event": event.name,
			"expr_b64": base64.StdEncoding.EncodeToString([]byte(expression)), "usage_units": units,
			"usage_facts": map[string]any{"output_seconds": 4, "input_video_seconds": "2.5", "input_image_count": 1},
			"admin_info": map[string]any{"statement_original_quota": event.quota,
				"statement_snapshot": map[string]any{"billing_mode": BillingReconciliationModeUnknown}},
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 4, ChannelId: 76,
			ModelName: "video", Type: event.logType, Quota: event.quota, CreatedAt: event.timestamp, Other: string(other),
		}).Error)
	}

	mode := BillingReconciliationModeDurationAndCount
	statement, err := GetBillingCustomerStatement(ctx, 7, 900, 1200, "api_key", 4, "video", mode)
	require.NoError(t, err)
	require.Len(t, statement.Groups, 1)
	require.Len(t, statement.Groups[0].Models, 1)
	assert.Equal(t, mode, statement.Groups[0].Models[0].BillingMode)
	assert.EqualValues(t, 1, statement.Summary.Requests)
	assert.EqualValues(t, 600000, statement.Summary.GrossQuota)
	assert.EqualValues(t, 440000, statement.Summary.RefundQuota)
	assert.EqualValues(t, 160000, statement.Summary.NetQuota)
	if statement.DataQuality != nil {
		assert.Zero(t, statement.DataQuality.UnknownBillingModeRequests)
	}

	list, err := GetBillingCustomerStatementList(ctx, 900, 1200, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.Equal(t, statement.Summary, list.Items[0].Usage)
	detail, err := GetBillingStatementLogs(ctx, BillingStatementLogFilter{
		UserId: 7, Start: 900, End: 1200, ModelName: "video", BillingMode: mode,
	}, 1, 10, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, 2, detail.Total)
	assert.EqualValues(t, 160000, detail.Quota)

	scanned, err := NextCustomerExportLogBatch(ctx, CustomerExportBatchParams{
		UserId: 7, StartTimestamp: 900, EndTimestamp: 1200, StatementScope: true,
	})
	require.NoError(t, err)
	rows, err := BuildCustomerExportRows(ctx, scanned, "video", mode, nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Nil(t, rows[0].BillableSeconds)
	assert.Empty(t, rows[0].BillableDurations)
	assert.Nil(t, rows[1].BillableSeconds)
	require.Len(t, rows[1].BillableDurations, 2)
	assert.Equal(t, "4", rows[1].BillableDurations["output_seconds"].String())
	assert.Equal(t, "2.5", rows[1].BillableDurations["input_video_seconds"].String())

	assert.Equal(t, BillingReconciliationModeUnknown, BillingStatementExpressionMode(`u("input_image_count")`, units))
	assert.Equal(t, BillingReconciliationModeUnknown, BillingStatementExpressionMode(expression, map[string]string{
		"output_seconds": "second", "input_video_seconds": "second",
	}))
}

func TestExportDurationsUseTheFrozenTierAndExcludeSelectionMeters(t *testing.T) {
	expression := `u("gate_seconds") > 3 ? tier("long", u("long_seconds") * 2 + u("count")) : tier("short", u("short_seconds") + u("count"))`
	other, err := common.Marshal(map[string]any{
		"matched_tier": "long", "expr_b64": base64.StdEncoding.EncodeToString([]byte(expression)),
		"usage_units": map[string]string{"gate_seconds": "second", "long_seconds": "second", "short_seconds": "second", "count": "count"},
		"usage_facts": map[string]any{"gate_seconds": 4, "long_seconds": 8, "short_seconds": 2, "count": 1},
	})
	require.NoError(t, err)
	fact := billingReconciliationLog{Type: LogTypeConsume, Other: string(other)}
	values := exportBillingDurations(fact, parsedBillingReconciliationLog{billingMode: BillingReconciliationModeDurationAndCount},
		loadCustomerExportFacts(fact.Other))
	require.Len(t, values, 1)
	assert.Equal(t, "8", values["long_seconds"].String())
}
