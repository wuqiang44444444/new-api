package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredMoxingProtocolPreservesFrozenTasksOnly(t *testing.T) {
	protocol := VideoUpstreamProtocol("moxing_media_task_v1")
	require.True(t, protocol.IsValid(), "frozen billing facts must remain recognizable")
	require.Error(t, ValidateVideoUpstreamProtocol(protocol), "new channels and submissions must reject the retired protocol")
	assert.Equal(t, VideoUpstreamProfileThirdPartyRelay, protocol.TransportProfile())
	create, query := protocol.TransportPaths("doubao-seedance-2-0-260128")
	assert.Empty(t, create)
	assert.Equal(t, "/v1/media/tasks/{task_id}", query)
}
