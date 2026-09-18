package model

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalDurationSizeMultiplierIsNotAnotherMeter(t *testing.T) {
	for _, tc := range []struct{ name, expression, mode string }{
		{"frozen seconds and size", `v1:tier("base", param("_task.duration_seconds") * 145689.000000 * param("_task.size_multiplier"))`, "per_second"},
		{"size alone does not prove calls", `tier("base", param("_task.size_multiplier") * 145689)`, "unknown"},
		{"client size is not historical probe", `tier("base", param("_task.duration_seconds") * param("size_multiplier"))`, "unknown"},
		{"unknown probe remains unknown", `tier("base", param("_task.duration_seconds") * param("_task.other_multiplier"))`, "unknown"},
		{"undeclared usage is not a probe", `tier("base", param("_task.duration_seconds") * u("size_multiplier"))`, "unknown"},
		{"mixed seconds and tokens remain unknown", `tier("base", param("_task.duration_seconds") * param("_task.size_multiplier") + c)`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.mode, BillingStatementExpressionMode(tc.expression, nil))
		})
	}
}

func TestHistoricalSizedDurationAgreesAcrossStatementReaders(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	ctx := context.Background()
	expression := `v1:tier("base", param("_task.duration_seconds") * 145689.000000 * param("_task.size_multiplier"))`
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	require.NoError(t, db.Create(&Task{TaskID: "refunded-size-task", UserId: 7, AppID: 4, ChannelId: 76,
		Properties: Properties{OriginModelName: "custom-video"},
		PrivateData: TaskPrivateData{TokenId: 4, AsyncBilling: &TaskAsyncBillingContext{
			TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: expression},
		}},
	}).Error)
	other, err := common.Marshal(map[string]any{"is_task": true, "model_price": 0, "group_ratio": 0.87,
		"billing_mode": "tiered_expr", "expr_b64": base64.StdEncoding.EncodeToString([]byte(expression))})
	require.NoError(t, err)
	logs := []Log{
		{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "custom-video", CreatedAt: 1100, Type: LogTypeConsume, Quota: 253499, Other: string(other)},
		{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "custom-video", CreatedAt: 1101, Type: LogTypeConsume, Quota: 253499, Other: string(other)},
		{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "custom-video", CreatedAt: 1102, Type: LogTypeRefund, Quota: 253499, Other: `{"task_id":"refunded-size-task","model_price":0,"group_ratio":0.87}`},
	}
	require.NoError(t, db.Create(&logs).Error)
	var baseline []Log
	require.NoError(t, db.Order("id").Find(&baseline).Error)
	interactive, err := GetBillingCustomerStatement(ctx, 7, 1000, 1200, "api_key", 4, "", "")
	require.NoError(t, err)
	require.Len(t, interactive.Groups, 1)
	require.Len(t, interactive.Groups[0].Models, 1)
	assert.Equal(t, "per_second", interactive.Groups[0].Models[0].BillingMode)
	assert.Zero(t, interactive.DataQuality.UnknownBillingModeRequests)
	assert.EqualValues(t, 253499, interactive.Summary.NetQuota)
	assert.EqualValues(t, 2, interactive.Summary.Requests)
	assert.Zero(t, interactive.Summary.BillableCalls)
	assert.Zero(t, interactive.Summary.OutputTokens)
	batched, err := GetBillingCustomerStatement(ctx, 7, 1000, 1200, "api_key", 4, "", "", BillingStatementReadPolicy{BeforeBatch: func(context.Context) error { return nil }})
	require.NoError(t, err)
	assert.Equal(t, interactive, batched)
	list, err := GetBillingCustomerStatementList(ctx, 1000, 1200, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.Equal(t, interactive.Summary, list.Items[0].Usage)
	assert.Zero(t, list.Items[0].DataQuality.UnknownBillingModeRequests)
	key := 4
	detail, err := GetBillingStatementLogs(ctx, BillingStatementLogFilter{UserId: 7, Start: 1000, End: 1200, TokenId: &key, BillingMode: "per_second"}, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, 3, detail.Total)
	assert.EqualValues(t, 253499, detail.Quota)
	var after []Log
	require.NoError(t, db.Order("id").Find(&after).Error)
	assert.Equal(t, baseline, after, "classification must not rewrite billing records")
}
