package model

import "github.com/QuantumNous/new-api/constant"

// channelSkipsGenericAbilities reports whether a typed local channel kind
// manages its own dedicated routing and must never enter the generic Ability
// dispatch pool. Batch channels follow Link-style local governance: a public
// model routed through a Batch channel is never reachable by generic sync
// dispatch, so identical model names cannot accidentally pick a Batch channel
// for sync traffic.
func channelSkipsGenericAbilities(channelType int) bool {
	if channelType == constant.ChannelTypeSeedanceLink || channelType == constant.ChannelTypeAzureBatch {
		return true
	}
	// MiniMax Link channels share the typed no-generic-Ability contract.
	return channelType == constant.ChannelTypeMiniMaxLink
}
