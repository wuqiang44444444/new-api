package plugins

import (
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
)

// The embedded MiniMax artifact must compile under the typed extension
// contract and must never compile as a generic native plugin: the typed
// channel is code-registered, never a factory candidate.
func TestMinimaxEmbeddedArtifactCompilesUnderTypedContract(t *testing.T) {
	source := MinimaxSource()
	if source == "" {
		t.Fatal("embedded minimax-link artifact is empty")
	}
	if version := MinimaxVersion(); version == "" {
		t.Fatal("embedded minimax-link artifact declares no version")
	}
	plugin, info, err := pluginruntime.CompileSeedanceExtension(source, pluginruntime.Options{}, pluginruntime.MinimaxHostContract())
	if err != nil {
		t.Fatalf("embedded minimax-link artifact must compile: %v", err)
	}
	if plugin.Meta.Key != pluginruntime.MinimaxPluginKey {
		t.Fatalf("artifact key = %q, want %q", plugin.Meta.Key, pluginruntime.MinimaxPluginKey)
	}
	if len(info.Protocols) != 1 || info.Protocols[0] != "jdcloud_video_task_v1" {
		t.Fatalf("declared protocols = %v, want [jdcloud_video_task_v1]", info.Protocols)
	}
	if info.Configuration == nil {
		t.Fatal("artifact must declare a channel configuration")
	}
	if _, err := pluginruntime.NewRegistry().Register(source, pluginruntime.Options{}); err == nil {
		t.Fatal("typed artifact must not register into the native plugin registry")
	}
}
