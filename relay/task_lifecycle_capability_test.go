package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestTaskLifecycleUsesFrozenSKUCapability(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		SKUCapabilityVersion: model.VideoSKUCapabilityVersionAZHWV1,
		SKULifecycle: model.VideoSKULifecycleCapability{
			SupportsContent: true,
		},
	}}

	capability := TaskLifecycleCapabilities(task)
	assert.True(t, capability.SupportsContent)
	assert.False(t, capability.SupportsCancelQueued)
	assert.False(t, capability.SupportsDeleteTerminal)
}
