package relay

import (
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/channel/task/minimax"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The MiniMax typed pin carries the extension plugin for freezing and
// declaration reads. The request-path adaptor dispatch must keep the numeric
// typed platform identity and return the typed Go adaptor — the generic
// js-plugin adaptor would rewrite the platform to the plugin key and break
// frozen connections, the query schedule and content dispatch.
func TestGetTaskAdaptorForRequestKeepsMiniMaxTypedPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context := &gin.Context{}
	context.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{
		Plugin: &pluginruntime.LoadedPlugin{Meta: pluginruntime.Meta{Key: pluginruntime.MinimaxPluginKey, Version: "1.0.0"}},
	})
	platform, adaptor := getTaskAdaptorForRequest(context, minimax.Platform)
	assert.Equal(t, minimax.Platform, platform)
	_, typed := adaptor.(*minimax.TaskAdaptor)
	require.True(t, typed, "the typed Go adaptor must serve the MiniMax platform")
}
