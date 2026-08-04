package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestPublishedVideoProtocolsAlwaysRequireTaskCreateAttempt(t *testing.T) {
	for _, protocol := range []string{
		model.TaskClientProtocolModelArkV3,
		model.TaskClientProtocolKlingV1,
		model.TaskClientProtocolJimeng,
	} {
		info := &relaycommon.RelayInfo{
			TaskRelayInfo: &relaycommon.TaskRelayInfo{ClientProtocol: protocol},
		}
		assert.True(t, RequiresVideoTaskCreateAttempt(info), protocol)
	}

	assert.False(t, RequiresVideoTaskCreateAttempt(nil))
	assert.False(t, RequiresVideoTaskCreateAttempt(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}))
	assert.False(t, RequiresVideoTaskCreateAttempt(&relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{ClientProtocol: "unknown"},
	}))
}
