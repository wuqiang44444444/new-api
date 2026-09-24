package minimax

import (
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxplugin"
	"github.com/gin-gonic/gin"
)

// PinMinimaxExtensionForChannel resolves the active extension version and
// pins it in the gin context so the submission snapshot freezes plugin
// identity via the shared execution-snapshot machinery. A missing or
// non-covering active version is an error: creation must fail closed. No
// engine calls happen on this path - hook presence was verified at compile
// time and protocol coverage by ActiveEntryFor.
func PinMinimaxExtensionForChannel(requestContext *gin.Context) error {
	entry, err := minimaxplugin.Default.ActiveEntryFor(protocolName())
	if err != nil {
		return err
	}
	requestContext.Set(minimaxConfigurationContextKey, entry)
	requestContext.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: entry.Plugin})
	return nil
}

// minimaxConfigurationContextKey carries the compiled active version (code +
// declaration pair) pinned for this request.
const minimaxConfigurationContextKey = "minimax_pinned_configuration"

// PinnedMinimaxExtension returns the plugin pinned for this request, or nil
// when nothing valid is pinned. A pin from another typed channel (for
// example a Seedance pin) is ignored, never consumed.
func PinnedMinimaxExtension(requestContext *gin.Context) *pluginruntime.LoadedPlugin {
	value, exists := requestContext.Get(pluginruntime.ContextKeyPinnedPlugin)
	if !exists {
		return nil
	}
	pinned, ok := value.(pluginruntime.PinnedPlugin)
	if !ok || pinned.Plugin == nil {
		return nil
	}
	if pinned.Plugin.Meta.Key != pluginruntime.MinimaxPluginKey {
		return nil
	}
	return pinned.Plugin
}
