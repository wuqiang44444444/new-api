package plugins

import (
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedanceLinkTestContract() pluginruntime.SeedanceExtensionContract {
	return pluginruntime.SeedanceExtensionContract{
		Key: "seedance-link",
		Protocols: []pluginruntime.SeedanceExtensionProtocol{
			{Name: "feicai_videos_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
		},
	}
}

func TestSeedanceLinkArtifactCompiles(t *testing.T) {
	source := SeedanceSource()
	assert.NotEmpty(t, source)

	plugin, info, err := pluginruntime.CompileSeedanceExtension(source, pluginruntime.Options{Key: "seedance-link"}, seedanceLinkTestContract())
	require.NoError(t, err)
	assert.Equal(t, "seedance-link", plugin.Meta.Key)
	assert.Equal(t, []string{"feicai_videos_v1"}, info.Protocols)

	// The artifact must never be picked up by the generic task-plugin
	// compilation path; if it starts passing, the isolation contract broke.
	_, err = pluginruntime.CompilePlugin(source, pluginruntime.Options{Key: "seedance-link"})
	require.Error(t, err)
}

func TestSeedanceLinkArtifactNotInNativeFactory(t *testing.T) {
	for _, meta := range pluginruntime.DefaultRegistry.Snapshot().Factory {
		assert.NotEqual(t, "seedance-link", meta.Key)
	}
}
