package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxplugin"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// compileTaskPluginSource routes an administrator-provided source to the
// right compile contract. Native keys keep their single generic compile; the
// reserved key always requires the Seedance contract, even if an artifact also
// supplies native hooks. Broken generic sources keep the generic error.
func compileTaskPluginSource(source string, options pluginruntime.Options) (*pluginruntime.LoadedPlugin, error) {
	generic, genericErr := pluginruntime.NewRegistry().Register(source, options)
	if genericErr == nil {
		if generic.Meta.Key == taskseedance.SeedanceExtensionPluginKey {
			plugin, _, err := taskseedance.CompileSeedanceExtensionSource(source)
			return plugin, err
		}
		if generic.Meta.Key == pluginruntime.MinimaxPluginKey {
			plugin, _, err := CompileMinimaxExtensionSource(source)
			return plugin, err
		}
		return generic, nil
	}
	key, keyErr := pluginruntime.MetaKeyFromSource(source, pluginruntime.Options{})
	if keyErr != nil || (key != taskseedance.SeedanceExtensionPluginKey && key != pluginruntime.MinimaxPluginKey) {
		return nil, genericErr
	}
	if key == pluginruntime.MinimaxPluginKey {
		plugin, _, err := CompileMinimaxExtensionSource(source)
		return plugin, err
	}
	plugin, _, err := taskseedance.CompileSeedanceExtensionSource(source)
	return plugin, err
}

// splitSeedanceExtensionRows separates typed extension rows (seedance-link
// and minimax-link) from the native override set so the native registry
// never receives them (index isolation).
func splitSeedanceExtensionRows(rows []model.TaskPlugin) (native []model.TaskPlugin, extensions []model.TaskPlugin) {
	for _, row := range rows {
		if row.Key == taskseedance.SeedanceExtensionPluginKey || row.Key == pluginruntime.MinimaxPluginKey {
			extensions = append(extensions, row)
			continue
		}
		native = append(native, row)
	}
	return native, extensions
}

// syncSeedanceExtensionPlugins seeds the embedded artifact and publishes the
// active extension rows. It runs inside the caller's taskPluginSyncState
// critical section and records its own error bookkeeping so the native sync
// outcome stays untouched.
func syncSeedanceExtensionPlugins(ctx context.Context, rows []model.TaskPlugin) {
	key := taskseedance.SeedanceExtensionPluginKey
	if err := taskseedance.EnsureSeededExtension(ctx); err != nil {
		_ = taskseedance.SyncExtensionSnapshot(ctx, nil)
		taskPluginSyncState.errors[key] = err.Error()
		common.SysError(fmt.Sprintf("sync seedance extension: %v", err))
		return
	}
	// The caller's snapshot predates bundled-version promotion. Read the
	// committed active row so the first sync publishes the new version.
	rows = nil
	active, err := model.GetTaskPluginVersion(key, "")
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		_ = taskseedance.SyncExtensionSnapshot(ctx, nil)
		taskPluginSyncState.errors[key] = "cannot read active Seedance plugin"
		return
	}
	if err == nil && active.Enabled {
		rows = append(rows, *active)
	}
	if err := taskseedance.SyncExtensionSnapshot(ctx, rows); err != nil {
		taskPluginSyncState.errors[key] = err.Error()
		common.SysError(fmt.Sprintf("sync seedance extension: %v", err))
		return
	}
	if _, syncErrors := taskseedance.DescribeExtension(); len(syncErrors) > 0 {
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

// seedanceExtensionDeletionError projects references from the atomic model
// operation; no controller-side precheck can authorize deletion.
func seedanceExtensionDeletionError(c *gin.Context, err error) bool {
	var inUse *model.SeedancePluginInUseError
	if !errors.As(err, &inUse) {
		return false
	}
	c.JSON(200, gin.H{"success": false, "message": inUse.Error(), "data": gin.H{"tasks": inUse.Tasks, "attempts": inUse.Attempts}})
	return true
}

// applySeedanceExtensionListItem projects the real extension runtime state
// into a plugin list item. The native registry is deliberately unaware of
// the extension, so its override/error maps would otherwise report a
// healthy active extension as not_registered and count platform-62
// execution dependencies as zero.
func applySeedanceExtensionListItem(item *taskPluginListItem) error {
	tasks, attempts, err := model.GetSeedancePluginExecutionUsage(item.Meta.Key, "")
	if err != nil {
		return err
	}
	item.InFlightCount = int64(len(tasks) + len(attempts))
	item.RuntimeError = ""

	if !item.Enabled {
		// item.Active marks the promoted version, not the runtime switch; a
		// disabled row is not serving regardless of promotion.
		item.RuntimeStatus = "disabled_fallback"
		return nil
	}
	activeVersion, syncErrors := describeTypedExtensionByKey(item.Meta.Key)
	if len(syncErrors) > 0 {
		item.RuntimeStatus = "compile_failed"
		item.RuntimeError = syncErrors[0]
		return nil
	}
	if activeVersion == "" {
		item.RuntimeStatus = "not_registered"
		return nil
	}
	item.RuntimeStatus = "registered"
	return nil
}

// describeTypedExtensionByKey projects the runtime diagnostics of the
// reserved typed extension that owns the plugin key.
func describeTypedExtensionByKey(key string) (string, []string) {
	if key == pluginruntime.MinimaxPluginKey {
		return minimaxplugin.Default.Describe()
	}
	return taskseedance.DescribeExtension()
}
