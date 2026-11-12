package controller

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaymode "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
)

func observeAutomaticChannelAlert(ctx context.Context, channel *model.Channel, result testResult, scope string, automatic bool) {
	if !automatic || result.checkUnsupported || seedanceLinkChannel(channel) {
		// Seedance's typed asset check observes its own operation.
		return
	}
	modelName := result.testedModel
	if modelName == "" {
		modelName = channelAutoCheckModelName(channel)
	}
	slot := -1
	if channel.ChannelInfo.IsMultiKey && result.context != nil && common.GetContextKeyBool(result.context, constant.ContextKeyChannelIsMultiKey) {
		slot = common.GetContextKeyInt(result.context, constant.ContextKeyChannelMultiKeyIndex)
	}
	status := result.upstreamStatus
	switch scope {
	case "generation_probe":
		if result.context != nil && result.context.Request != nil {
			service.ObserveChannelRelayProbe(ctx, channel, modelName, relaymode.Path2RelayMode(result.context.Request.URL.Path),
				status, result.upstreamAttempted, result.upstreamResponded, result.localErr == nil && result.newAPIError == nil, slot)
		}
	case "readonly_probe":
		service.ObserveChannelProbe(ctx, channel, "", "connection_probe", status, result.upstreamAttempted,
			result.upstreamResponded, result.localErr == nil && result.newAPIError == nil, -1)
	}
}
