package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxplugin"
	"gorm.io/gorm"
)

// CompileMinimaxExtensionSource compiles an administrator-provided
// minimax-link artifact against the typed extension host contract. It is the
// control-plane entry for upload, activation, listing, and dry-run
// compilation.
func CompileMinimaxExtensionSource(source string) (*pluginruntime.LoadedPlugin, pluginruntime.SeedanceExtensionInfo, error) {
	return pluginruntime.CompileSeedanceExtension(source, pluginruntime.Options{Key: pluginruntime.MinimaxPluginKey}, pluginruntime.MinimaxHostContract())
}

// syncMinimaxExtensionPlugins seeds the embedded artifact (deploy-only,
// never enabling it) and publishes the active extension row. It runs inside
// the caller's taskPluginSyncState critical section and records its own
// error bookkeeping so the native sync outcome stays untouched.
func syncMinimaxExtensionPlugins(ctx context.Context, rows []model.TaskPlugin) {
	key := pluginruntime.MinimaxPluginKey
	if err := minimaxplugin.Default.EnsureSeeded(ctx); err != nil {
		_ = minimaxplugin.Default.SyncSnapshot(ctx, nil)
		taskPluginSyncState.errors[key] = err.Error()
		common.SysError(fmt.Sprintf("sync minimax extension: %v", err))
		return
	}
	// Read the committed active row so the first sync publishes the stored
	// version instead of the caller's pre-insert snapshot.
	published := make([]model.TaskPlugin, 0)
	active, err := model.GetTaskPluginVersion(key, "")
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		_ = minimaxplugin.Default.SyncSnapshot(ctx, nil)
		taskPluginSyncState.errors[key] = "cannot read active MiniMax plugin"
		return
	}
	if err == nil && active.Enabled {
		published = append(published, *active)
	}
	if err := minimaxplugin.Default.SyncSnapshot(ctx, published); err != nil {
		taskPluginSyncState.errors[key] = err.Error()
		common.SysError(fmt.Sprintf("sync minimax extension: %v", err))
		return
	}
	if _, syncErrors := minimaxplugin.Default.Describe(); len(syncErrors) > 0 {
		message := ""
		for _, item := range syncErrors {
			if message != "" {
				message += "; "
			}
			message += item
		}
		taskPluginSyncState.errors[key] = message
		return
	}
	delete(taskPluginSyncState.errors, key)
}
