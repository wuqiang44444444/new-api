package service

import (
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// frozenVideoBillingFetchContext returns only task-create-time facts. Provider
// adapters may use them while normalizing a terminal response, but polling must
// never reconstruct them from current channel or pricing settings.
func frozenVideoBillingFetchContext(task *model.Task) *relaycommon.VideoTaskBillingContext {
	if task == nil {
		return &relaycommon.VideoTaskBillingContext{}
	}
	context := &relaycommon.VideoTaskBillingContext{ProviderModel: task.Properties.UpstreamModelName}
	if task.PrivateData.AsyncBilling == nil {
		return context
	}
	context.EstimatedTokens = task.PrivateData.AsyncBilling.EstimatedTokens
	if task.PrivateData.AsyncBilling.BillingProbe == nil {
		return context
	}
	context.BillingProbeBody = append([]byte(nil), task.PrivateData.AsyncBilling.BillingProbe.Body...)
	return context
}
