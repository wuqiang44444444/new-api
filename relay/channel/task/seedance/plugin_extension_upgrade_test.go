package seedance

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedanceEmbeddedUpgradePreservesOldVersionUntilActivation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TaskPlugin{}))
	original := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = original
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	// A synthetic previously installed artifact exercises version persistence;
	// this fixture does not claim to reproduce the released 1.0.1 source.
	oldSource := strings.Replace(plugins.SeedanceSource(), `version: "1.0.2"`, `version: "1.0.1"`, 1)
	require.NotEqual(t, plugins.SeedanceSource(), oldSource)
	old := model.TaskPlugin{Key: SeedanceExtensionPluginKey, Version: "1.0.1", APIVersion: 1,
		Source: oldSource, SourceHash: sourceHashOf(oldSource), Enabled: true}
	require.NoError(t, model.SaveTaskPlugin(&old))
	store := &seedanceExtensionStore{compiled: map[string]*seedanceExtensionEntry{}, seeded: map[string]bool{}}
	require.NoError(t, store.EnsureSeeded(context.Background()))
	require.NoError(t, store.EnsureSeeded(context.Background()))
	versions, err := model.ListTaskPluginVersions(SeedanceExtensionPluginKey)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	installed, err := model.GetTaskPluginVersion(SeedanceExtensionPluginKey, "1.0.1")
	require.NoError(t, err)
	assert.True(t, installed.Active)
	assert.Equal(t, oldSource, installed.Source)
	assert.Equal(t, old.SourceHash, installed.SourceHash)
	upgrade, err := model.GetTaskPluginVersion(SeedanceExtensionPluginKey, "1.0.2")
	require.NoError(t, err)
	assert.False(t, upgrade.Active)
	assert.Equal(t, plugins.SeedanceSource(), upgrade.Source)

	require.NoError(t, model.ActivateTaskPlugin(SeedanceExtensionPluginKey, upgrade.Version))
	active, err := model.ListActiveTaskPlugins()
	require.NoError(t, err)
	require.NoError(t, store.SyncSnapshot(context.Background(), active))
	plugin, err := store.ActiveFor(dto.VideoUpstreamProtocolFeicaiVideosV1)
	require.NoError(t, err)
	assert.Equal(t, upgrade.Version, plugin.Meta.Version)
	historical, err := store.ResolveVersion(context.Background(), old.Version)
	require.NoError(t, err)
	assert.Equal(t, old.Version, historical.Meta.Version)
}
