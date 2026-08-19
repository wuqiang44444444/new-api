package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendBillingStatementSnapshotIsAdminOnlyAndFrozen(t *testing.T) {
	other := map[string]interface{}{"model_price": 0.25, "group_ratio": 0.8, "contract_discount": "0.5", "contract_version": 4, "completion_ratio": 2.0}
	relayInfo := &relaycommon.RelayInfo{OriginModelName: "customer-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "provider-model"}}

	appendBillingStatementSnapshot(relayInfo, other)

	_, exposedAtTopLevel := other["statement_snapshot"]
	assert.False(t, exposedAtTopLevel)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	snapshot, ok := adminInfo["statement_snapshot"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "provider-model", snapshot["provider_model"])
	assert.Equal(t, "per_call", snapshot["billing_mode"])
	assert.Equal(t, 1, snapshot["snapshot_version"])
	assert.Equal(t, 0.8, snapshot["group_ratio"])
	assert.Equal(t, "0.5", snapshot["contract_discount"])
	assert.Equal(t, 4, snapshot["contract_version"])
	assert.Equal(t, 2.0, snapshot["completion_ratio"])

}

func TestAppendBillingStatementSnapshotFallsBackToOriginIdentity(t *testing.T) {
	other := map[string]interface{}{"model_price": float64(0)}
	appendBillingStatementIdentitySnapshot(other, "same-model", "")

	adminInfo := other["admin_info"].(map[string]interface{})
	snapshot := adminInfo["statement_snapshot"].(map[string]interface{})
	assert.Equal(t, "same-model", snapshot["provider_model"])
	assert.Equal(t, "token", snapshot["billing_mode"])
}

func TestAppendPerCallBillingStatementSnapshotDoesNotDependOnFixedPrice(t *testing.T) {
	other := map[string]interface{}{"model_price": float64(-1), "model_ratio": 2.5}
	relayInfo := &relaycommon.RelayInfo{OriginModelName: "task-model"}

	appendPerCallBillingStatementSnapshot(relayInfo, other)

	adminInfo := other["admin_info"].(map[string]interface{})
	snapshot := adminInfo["statement_snapshot"].(map[string]interface{})
	assert.Equal(t, "task-model", snapshot["provider_model"])
	assert.Equal(t, "per_call", snapshot["billing_mode"])
}
