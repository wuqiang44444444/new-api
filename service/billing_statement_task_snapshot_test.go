package service

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStatementSnapshotUsesFrozenExpressionAndContract(t *testing.T) {
	for _, tc := range []struct{ name, expression, mode string }{
		{"tokens", `tier("base", c * 7)`, "token"},
		{"seconds", `tier("base", u("seconds") * 0.4)`, "unknown"},
		{"invalid", `tier("broken",`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := model.NewLogOther()
			other.SetPublic("expr_b64", base64.StdEncoding.EncodeToString([]byte(tc.expression)))
			other.SetPublic("group_ratio", 0.87)
			other.SetPublic("contract_discount", "0.5")
			appendBillingStatementIdentitySnapshotWithMode(other, "customer", "provider", "per_call")
			admin, ok := other.Snapshot()["admin_info"].(map[string]any)
			require.True(t, ok)
			snapshot, ok := admin["statement_snapshot"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.mode, snapshot["billing_mode"])
			assert.Equal(t, "0.5", snapshot["contract_discount"])
		})
	}
}

func TestTaskStatementPersistsUsageUnitsForCreateAndSettlement(t *testing.T) {
	truncate(t)
	seedUser(t, 40, 10000)
	seedChannel(t, 40)
	snap := &billingexpr.BillingSnapshot{ExprString: `tier("base", u("meter") * 7 / 1000000)`, TaskUsageBilling: true,
		GroupRatio: 1, UsageUnits: map[string]string{"meter": "token"}, UsageFacts: map[string]any{"meter": 100000}}
	task := makeTask(40, 40, 100, 0, BillingSourceWallet, 0)
	task.Properties.OriginModelName = "video"
	task.PrivateData.BillingContext.TieredSnapshot = snap
	encoded, err := common.Marshal(task.PrivateData)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &task.PrivateData))
	info := &relaycommon.RelayInfo{UserId: 40, OriginModelName: "video", UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 40}, TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "GENERATE"},
		PriceData: types.PriceData{Quota: 100, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}, TieredBillingSnapshot: task.PrivateData.BillingContext.TieredSnapshot}
	create := callLogTaskConsumption(t, info, task)
	var createOther map[string]any
	require.NoError(t, common.UnmarshalJsonStr(create.Other, &createOther))
	createSnapshot := createOther["admin_info"].(map[string]any)["statement_snapshot"].(map[string]any)
	assert.Equal(t, "token", createSnapshot["billing_mode"])
	assert.Equal(t, map[string]any{"meter": "token"}, createSnapshot["usage_units"])
	for _, async := range []bool{false, true} {
		if async {
			task.PrivateData.AsyncBilling = &model.TaskAsyncBillingContext{TieredSnapshot: task.PrivateData.BillingContext.TieredSnapshot}
			task.PrivateData.BillingContext.TieredSnapshot = nil
		}
		other := taskBillingOther(task).Snapshot()
		snapshot := other["admin_info"].(map[string]any)["statement_snapshot"].(map[string]any)
		assert.Equal(t, "token", snapshot["billing_mode"])
		assert.Equal(t, map[string]string{"meter": "token"}, snapshot["usage_units"])
	}
}

func TestTaskStatementRecoversExpressionFromAsyncFrozenFacts(t *testing.T) {
	task := &model.Task{Properties: model.Properties{OriginModelName: "customer"}, PrivateData: model.TaskPrivateData{
		BillingContext: &model.TaskBillingContext{GroupRatio: 0.87},
		AsyncBilling:   &model.TaskAsyncBillingContext{TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("base", c * 7)`}},
	}}
	other := taskBillingOther(task).Snapshot()
	assert.Equal(t, "tiered_expr", other["billing_mode"])
	admin, ok := other["admin_info"].(map[string]any)
	require.True(t, ok)
	snapshot, ok := admin["statement_snapshot"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "token", snapshot["billing_mode"])
}
