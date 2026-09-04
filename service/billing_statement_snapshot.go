package service

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// appendBillingStatementSnapshot persists only identities and classification
// that were true at settlement time. It never evaluates or changes pricing.
// The snapshot lives under admin_info so ordinary user log responses remove it
// together with the other administrator-only fields.
func appendBillingStatementSnapshot(relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if relayInfo == nil {
		return
	}
	appendBillingStatementIdentitySnapshot(other, relayInfo.OriginModelName, relayInfo.GetUpstreamModelName())
}

func appendPerCallBillingStatementSnapshot(relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if relayInfo == nil {
		return
	}
	appendBillingStatementIdentitySnapshotWithMode(other, relayInfo.OriginModelName, relayInfo.GetUpstreamModelName(), "per_call")
}

func appendBillingStatementIdentitySnapshot(other *model.LogOther, originModel string, upstreamModel string) {
	appendBillingStatementIdentitySnapshotWithMode(other, originModel, upstreamModel, "")
}

func appendBillingStatementIdentitySnapshotWithMode(other *model.LogOther, originModel string, upstreamModel string, explicitMode string) {
	if other == nil {
		return
	}
	providerModel := strings.TrimSpace(upstreamModel)
	if providerModel == "" {
		providerModel = strings.TrimSpace(originModel)
	}
	if providerModel == "" {
		return
	}
	billingMode := "token"
	if explicitMode == "per_call" {
		billingMode = explicitMode
	} else if modelPrice, ok := other.Snapshot()["model_price"].(float64); ok && modelPrice > 0 {
		billingMode = "per_call"
	}
	values := other.Snapshot()
	snapshot := map[string]interface{}{
		"snapshot_version": 1,
		"billing_mode":     billingMode,
		"provider_model":   providerModel,
	}
	for _, key := range []string{
		"model_price", "model_ratio", "completion_ratio", "cache_ratio",
		"group_ratio", "user_group_ratio", "expr_b64",
		"contract_discount", "contract_version",
	} {
		if value, exists := values[key]; exists {
			snapshot[key] = value
		}
	}
	other.SetAdmin("statement_snapshot", snapshot)
}
