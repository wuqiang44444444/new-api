package model

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementKeepsZeroPriceTaskRefundWithUnclassifiedCharges(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 91, Username: "customer"}).Error)
	for _, row := range []struct {
		typ, quota int
		other      string
		expression bool
	}{
		{LogTypeConsume, 870, `{"task_id":"success","is_task":true}`, true},
		{LogTypeConsume, 870, `{"task_id":"refunded","is_task":true}`, true},
		{LogTypeRefund, 174, `{"task_id":"success","pre_consumed_quota":870,"actual_quota":696}`, true},
		{LogTypeRefund, 870, `{"task_id":"refunded"}`, false},
	} {
		var other map[string]any
		require.NoError(t, common.UnmarshalJsonStr(row.other, &other))
		other["model_price"], other["group_ratio"] = 0, 0.87
		other["admin_info"] = map[string]any{"statement_snapshot": map[string]any{"billing_mode": "per_call", "model_price": 0, "group_ratio": 0.87}}
		if row.expression {
			other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(`tier("seconds", param("_task.duration_seconds") * 100000)`))
		}
		encoded, err := common.Marshal(other)
		require.NoError(t, err)
		require.NoError(t, db.Create(&Log{UserId: 91, TokenId: 40, ChannelId: 76, ModelName: "video", CreatedAt: 1100, Type: row.typ, Quota: row.quota, Other: string(encoded)}).Error)
	}
	s, err := GetBillingCustomerStatement(91, 1000, 1200, "channel", 76, "video", "")
	require.NoError(t, err)
	require.Len(t, s.Groups, 1)
	require.Len(t, s.Groups[0].Models, 1, "refunds must not invent a second per-call group")
	m := s.Groups[0].Models[0]
	assert.Equal(t, BillingReconciliationModeUnknown, m.BillingMode)
	assert.EqualValues(t, 2, m.Usage.Requests)
	assert.EqualValues(t, 696, m.Usage.NetQuota)
	assert.Equal(t, m.Usage, s.Summary)
	require.NotNil(t, m.OriginalQuota)
	assert.EqualValues(t, 800, *m.OriginalQuota)
	list, err := GetBillingCustomerStatementList(1000, 1200, "", "", "net_quota", "desc", 1, 20)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.Equal(t, s.Summary, list.Items[0].Usage)
	channel := 76
	filter := BillingStatementLogFilter{UserId: 91, Start: 1000, End: 1200, ChannelId: &channel, ModelName: "video", BillingMode: "unknown"}
	detail, err := GetBillingStatementLogs(filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, 4, detail.Total)
	assert.Equal(t, m.Usage.NetQuota, detail.Quota)
	filter.BillingMode = "per_call"
	detail, err = GetBillingStatementLogs(filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.Zero(t, detail.Total)
}

func TestStatementDoesNotTreatZeroFixedPriceAsEvidenceForChargedTasks(t *testing.T) {
	for _, tc := range []struct {
		name, other, mode string
		quota, tokens     int
	}{
		{"unclassified charged task", `{"task_id":"t","model_price":0,"admin_info":{"statement_snapshot":{"billing_mode":"per_call"}}}`, "unknown", 80, 0},
		{"measured token task", `{"task_id":"t","model_price":0}`, "token", 80, 100},
		{"positive fixed price", `{"task_id":"t","model_price":1}`, "per_call", 80, 0},
		{"free explicit per call", `{"task_id":"t","model_price":0,"admin_info":{"statement_snapshot":{"billing_mode":"per_call"}}}`, "per_call", 0, 0},
		{"native free token", `{"model_price":0,"model_ratio":1}`, "token", 0, 0},
		{"explicit token fact", `{"task_id":"t","model_price":0,"admin_info":{"statement_snapshot":{"billing_mode":"token"}}}`, "token", 80, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := parseBillingReconciliationLog(billingReconciliationLog{Type: LogTypeRefund, Quota: tc.quota, CompletionTokens: tc.tokens, Other: tc.other})
			assert.Equal(t, tc.mode, parsed.billingMode)
		})
	}
}

func TestCustomerStatementKeepsRefundOnlyModelSignedAndSeparate(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	require.NoError(t, db.Create(&User{Id: 7, Username: "customer"}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "same-model", Type: LogTypeConsume, CreatedAt: 1100, Quota: 800, Other: `{"model_ratio":1,"group_ratio":0.8}`},
		{UserId: 7, TokenId: 4, ChannelId: 76, ModelName: "same-model", Type: LogTypeRefund, CreatedAt: 1100, Quota: 80, Other: `{"model_price":1,"group_ratio":0.8}`},
	}).Error)
	s, err := GetBillingCustomerStatement(7, 1000, 1200, "channel", 76, "", "")
	require.NoError(t, err)
	require.Len(t, s.Groups, 1)
	require.Len(t, s.Groups[0].Models, 2, "real mixed billing must remain separate")
	var net, original int64
	for _, m := range s.Groups[0].Models {
		require.NotNil(t, m.OriginalQuota)
		net += m.Usage.NetQuota
		original += *m.OriginalQuota
		if m.BillingMode == "per_call" {
			assert.EqualValues(t, -80, m.Usage.NetQuota)
			assert.EqualValues(t, -100, *m.OriginalQuota)
		}
	}
	assert.Equal(t, s.Summary.NetQuota, net)
	assert.Equal(t, *s.OriginalQuota, original)
	channel := 76
	detail, err := GetBillingStatementLogs(BillingStatementLogFilter{UserId: 7, Start: 1000, End: 1200, ChannelId: &channel, BillingMode: "per_call"}, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.EqualValues(t, -80, detail.Quota)
}
