package plugins

import (
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceLinkArtifactCompiles(t *testing.T) {
	source := SeedanceSource()
	assert.NotEmpty(t, source)

	plugin, info, err := pluginruntime.CompileSeedanceExtension(source, pluginruntime.Options{Key: "seedance-link"}, pluginruntime.SeedanceHostContract())
	require.NoError(t, err)
	assert.Equal(t, "seedance-link", plugin.Meta.Key)
	assert.Equal(t, []string{"funcloud_modelark_v3", "synlink_video_v1", "feicai_videos_v1", "modelark_v3_volcengine", "modelark_v3_byteplus", "ark_media_v1", "tokensave_media_task_v1", "moxing_modelark_media_v1", "modelark_v3_cmcc"}, info.Protocols)

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
