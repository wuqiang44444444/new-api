package relay

import (
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relay/channel"
)

func freezeTaskBillingUsageUnits(snapshot *billingexpr.BillingSnapshot, adaptor channel.TaskAdaptor) {
	if provider, ok := adaptor.(interface{ BillingUsageUnits() map[string]string }); ok {
		snapshot.UsageUnits = provider.BillingUsageUnits()
	}
}
