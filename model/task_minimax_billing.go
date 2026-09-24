package model

import (
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// HasMiniMaxBillingFacts identifies durable MiniMax funding from frozen facts.
// It does not grant Seedance token semantics to provider credit evidence.
func (t *Task) HasMiniMaxBillingFacts() bool {
	return t != nil && t.PrivateData.AsyncBilling != nil &&
		t.Platform == constant.TaskPlatform(MiniMaxLinkTaskPlatform()) &&
		t.PrivateData.VideoUpstreamProtocol == dto.VideoUpstreamProtocol(constant.VideoUpstreamProtocolJdCloudTaskV1)
}

// HasTypedVideoBillingFacts selects the existing atomic video funding lifecycle.
// Native usage tasks remain outside this local state machine.
func (t *Task) HasTypedVideoBillingFacts() bool {
	return t != nil && (t.HasSeedanceBillingFacts() || t.HasMiniMaxBillingFacts())
}

func isMiniMaxBillingChannel(info *relaycommon.RelayInfo) bool {
	return info.ChannelMeta != nil && info.ChannelMeta.ChannelType == constant.ChannelTypeMiniMaxLink
}
