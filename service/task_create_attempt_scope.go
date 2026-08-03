package service

import (
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// RequiresVideoTaskCreateAttempt is a correctness boundary, not a rollout
// switch. Every currently published video Link contract must
// establish a durable attempt before an upstream POST. Traffic rollout remains
// controlled by Ability/group configuration.
func RequiresVideoTaskCreateAttempt(info *relaycommon.RelayInfo) bool {
	if info == nil || info.TaskRelayInfo == nil {
		return false
	}
	switch info.ClientProtocol {
	case model.TaskClientProtocolModelArkV3,
		model.TaskClientProtocolKlingV1,
		model.TaskClientProtocolJimeng:
		return true
	default:
		return false
	}
}
