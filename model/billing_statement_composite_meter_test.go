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

	assert.Equal(t, BillingReconciliationModeUnknown, BillingStatementExpressionMode(`u("input_image_count")`, units))
	assert.Equal(t, BillingReconciliationModeUnknown, BillingStatementExpressionMode(expression, map[string]string{
		"output_seconds": "second", "input_video_seconds": "second",
	}))
}
