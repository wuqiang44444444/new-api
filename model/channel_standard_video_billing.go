package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxlink"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// StandardVideoBillingFields is the shared typed usage contract for pricing
// validation, pre-consume and display. JD credit evidence is not a customer meter.
func StandardVideoBillingFields(channelType int, protocol dto.VideoUpstreamProtocol) map[string]jsplugin.UsageFieldSchema {
	if channelType == constant.ChannelTypeMiniMaxLink {
		return minimaxlink.UsageFields()
	}
	return seedancebilling.UsageFieldsForProtocol(protocol)
}

func standardVideoRequiresTokenBudget(channel Channel, expression string) bool {
	return channel.Type != constant.ChannelTypeMiniMaxLink &&
		seedancebilling.RequiresTokenBudget(channel.GetOtherSettings().VideoUpstreamProtocol, expression)
}
