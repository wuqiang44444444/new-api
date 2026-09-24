package minimaxplugin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The deploy-only store lifecycle: EnsureSeeded inserts the embedded
// artifact disabled (the first version may be active through the existing
// rule), re-running it never overwrites the administrator's enable choice,
// and ResolveVersion stops resolving deleted versions.
func TestStoreDeployOnlyLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "minimax_plugin_test.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.TaskPlugin{}, &model.Channel{}))

	store := NewStore()
	ctx := context.Background()
	require.NoError(t, store.EnsureSeeded(ctx))
	version := plugins.MinimaxVersion()

	stored, err := model.GetTaskPluginVersion("minimax-link", version)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.True(t, stored.Active, "first version follows the existing active rule")
	assert.False(t, stored.Enabled, "deployment must not enable new admission")

	require.NoError(t, model.SetTaskPluginEnabled("minimax-link", true))

	require.NoError(t, store.EnsureSeeded(ctx), "re-running the sync must be a no-op for existing rows")
	persisted, err := model.GetTaskPluginVersion("minimax-link", version)
	require.NoError(t, err)
	assert.True(t, persisted.Enabled, "re-running the deploy sync preserves the enabled choice")

	// Disabled-but-present versions keep resolving history; deleted versions
	// stop resolving.
	plugin, err := store.ResolveVersion(ctx, version)
	require.NoError(t, err)
	require.NotNil(t, plugin)
	require.NoError(t, db.Where(&model.TaskPlugin{Key: "minimax-link"}).Delete(&model.TaskPlugin{}).Error)
	_, err = store.ResolveVersion(ctx, version)
	assert.ErrorIs(t, err, ErrVersionUnavailable)
}

// ActiveEntryFor admits requests only after an enabled active version that
// declares the protocol exists; everything else fails closed.
func TestStoreActiveEntryFailClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "minimax_active_test.db")), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.TaskPlugin{}, &model.Channel{}))

	store := NewStore()
	_, err = store.ActiveEntryFor("jdcloud_video_task_v1")
	assert.ErrorIs(t, err, ErrUnavailable, "no activated version admits requests")

	require.NoError(t, store.EnsureSeeded(context.Background()))
	_, err = store.ActiveEntryFor("jdcloud_video_task_v1")
	assert.ErrorIs(t, err, ErrUnavailable, "a disabled active version still admits nothing")

	require.NoError(t, model.SetTaskPluginEnabled("minimax-link", true))
	require.NoError(t, store.SyncSnapshot(context.Background(), nil))
	active, err := model.GetTaskPluginVersion("minimax-link", "")
	require.NoError(t, err)
	require.NoError(t, store.SyncSnapshot(context.Background(), []model.TaskPlugin{*active}))
	entry, err := store.ActiveEntryFor("jdcloud_video_task_v1")
	require.NoError(t, err)
	assert.NotNil(t, entry.Plugin)
	assert.Equal(t, plugins.MinimaxVersion(), entry.Plugin.Meta.Version)
}
