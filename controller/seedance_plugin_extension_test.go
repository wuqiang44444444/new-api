package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitSeedanceExtensionRowsIsolatesNativeOverrideSet(t *testing.T) {
	rows := []model.TaskPlugin{
		{Key: "doubao", Version: "1.0.1"},
		{Key: "seedance-link", Version: "1.0.1"},
		{Key: "sora", Version: "2.0.0"},
	}
	native, extensions := splitSeedanceExtensionRows(rows)
	require.Len(t, native, 2)
	require.Len(t, extensions, 1)
	assert.Equal(t, "seedance-link", extensions[0].Key)
	assert.Equal(t, "doubao", native[0].Key)
	assert.Equal(t, "sora", native[1].Key)
}

func TestCompileTaskPluginSourceRoutesReservedKey(t *testing.T) {
	// The embedded seedance-link artifact cannot pass the generic contract;
	// the routed compile must accept it and never touch DefaultRegistry.
	before := jsplugin.DefaultRegistry.Snapshot()
	generationBefore := jsplugin.DefaultRegistry.Generation()

	loaded, err := compileTaskPluginSource(plugins.SeedanceSource(), jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, "seedance-link", loaded.Meta.Key)
	assert.Equal(t, before, jsplugin.DefaultRegistry.Snapshot())
	assert.Same(t, generationBefore, jsplugin.DefaultRegistry.Generation())

	// A generic artifact cannot bypass the reserved Seedance contract.
	_, err = compileTaskPluginSource(taskPluginControllerTestSource("seedance-link", "2.0.0"), jsplugin.Options{})
	require.Error(t, err)

	// Generic plugins keep the generic contract.
	loaded, err = compileTaskPluginSource(taskPluginControllerTestSource("generic-key", "1.0.1"), jsplugin.Options{})
	require.NoError(t, err)
	assert.Equal(t, "generic-key", loaded.Meta.Key)

	// A broken seedance artifact reports the seedance contract error.
	broken := `
export const meta = {
  apiVersion: 1, key: "seedance-link", name: "broken", version: "9.9.9",
  author: { name: "t" }, seedanceProtocols: ["feicai_videos_v1"],
};
export const seedance = { "feicai_videos_v1": {} };
`
	_, err = compileTaskPluginSource(broken, jsplugin.Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required export")
}

func TestSeedanceExtensionDeletionBlocksExecutionDependencies(t *testing.T) {
	setupTaskPluginControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
	row := model.TaskPlugin{Key: "seedance-link", Version: "1.0.1"}
	require.NoError(t, model.DB.Create(&row).Error)
	pinned := &model.Task{TaskID: "task-pinned", Platform: "67", Status: model.TaskStatusInProgress}
	pinned.PrivateData.Execution = &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: row.Key, Version: row.Version}}
	require.NoError(t, model.DB.Create(pinned).Error)
	for _, force := range []string{"", "true"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/task_plugin/seedance-link/1.0.1?force="+force, nil)
		c.Params = gin.Params{{Key: "key", Value: row.Key}, {Key: "version", Value: row.Version}}
		DeleteTaskPluginVersion(c)
		assert.Contains(t, recorder.Body.String(), "task plugin is still in use")
		_, err := model.GetTaskPluginVersion(row.Key, row.Version)
		require.NoError(t, err)
	}
}

func TestSyncSeedanceExtensionPluginsSeedsEmbeddedArtifact(t *testing.T) {
	setupTaskPluginControllerTest(t)

	syncSeedanceExtensionPlugins(nil, nil)

	version, err := model.GetTaskPluginVersion("seedance-link", "")
	require.NoError(t, err)
	assert.Equal(t, plugins.SeedanceVersion(), version.Version)
	assert.True(t, version.Active)
	assert.Equal(t, fmt.Sprintf("%x", common.Sha256Raw([]byte(plugins.SeedanceSource()))), version.SourceHash)

	// Seeding is idempotent per process: no duplicate row, same hash.
	syncSeedanceExtensionPlugins(nil, nil)
	versions, err := model.ListTaskPluginVersions("seedance-link")
	require.NoError(t, err)
	require.Len(t, versions, 1)
}

// TestApplySeedanceExtensionListItemProjectsRealStatus pins the list-view
// contract: the native registry is unaware of the extension, so the item
// status must come from the extension store — a healthy active extension
// reports registered, sync failures report compile_failed, and execution
// references come from the platform-62 usage query.
func TestApplySeedanceExtensionListItemProjectsRealStatus(t *testing.T) {
	setupTaskPluginControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
	require.NoError(t, taskseedance.SyncExtensionSnapshot(nil, nil))

	// Healthy active extension with the seeded row synced.
	rows := []model.TaskPlugin{{
		Key: "seedance-link", Version: plugins.SeedanceVersion(), Source: model.LongText(plugins.SeedanceSource()),
		SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(plugins.SeedanceSource()))),
		Enabled:    true, Active: true,
	}}
	require.NoError(t, taskseedance.SyncExtensionSnapshot(nil, rows))

	item := taskPluginListItem{Active: true, Enabled: true, Meta: jsplugin.Meta{Key: "seedance-link", Version: rows[0].Version}}
	require.NoError(t, applySeedanceExtensionListItem(&item))
	assert.Equal(t, "registered", item.RuntimeStatus)
	assert.Empty(t, item.RuntimeError)
	item.Meta.Version = "unpublished-version"
	require.NoError(t, applySeedanceExtensionListItem(&item))
	assert.Equal(t, "not_registered", item.RuntimeStatus)
	item.RuntimeError = "cannot read active Seedance plugin"
	require.NoError(t, applySeedanceExtensionListItem(&item))
	assert.Equal(t, "compile_failed", item.RuntimeStatus)
	assert.Equal(t, "cannot read active Seedance plugin", item.RuntimeError)

	// A disabled active row reports disabled_fallback, never not_registered
	// noise from the native registry.
	disabled := taskPluginListItem{Active: true, Enabled: false, Meta: jsplugin.Meta{Key: "seedance-link"}}
	require.NoError(t, applySeedanceExtensionListItem(&disabled))
	assert.Equal(t, "disabled_fallback", disabled.RuntimeStatus)
}

func TestSeedanceListCountsAttemptsAndDisabledHistory(t *testing.T) {
	setupTaskPluginControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
	require.NoError(t, taskseedance.SyncExtensionSnapshot(nil, nil))
	pinned := &model.Task{TaskID: "existing", Platform: "67", Status: model.TaskStatusInProgress}
	pinned.PrivateData.Execution = &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: "seedance-link", Version: "older"}}
	require.NoError(t, model.DB.Create(pinned).Error)
	frozen, err := common.Marshal(map[string]string{"plugin_key": "seedance-link", "plugin_version": "newer"})
	require.NoError(t, err)
	attempt := model.TaskCreateAttempt{AttemptID: "unknown-create", Status: model.TaskCreateAttemptUnknown, UpstreamProtocol: "feicai_videos_v1", FrozenConnectionSnapshot: frozen}
	require.NoError(t, model.DB.Create(&attempt).Error)
	for _, enabled := range []bool{true, false} {
		item := taskPluginListItem{Enabled: enabled, Meta: jsplugin.Meta{Key: "seedance-link"}}
		require.NoError(t, applySeedanceExtensionListItem(&item))
		assert.EqualValues(t, 2, item.InFlightCount)
	}
	// An unavailable query must surface, not display an authoritative zero.
	require.NoError(t, model.DB.Migrator().DropTable(&model.TaskCreateAttempt{}))
	item := taskPluginListItem{Enabled: false, Meta: jsplugin.Meta{Key: "seedance-link"}}
	require.Error(t, applySeedanceExtensionListItem(&item))
}

func TestLinkPluginManagementClassifiesBothExtensions(t *testing.T) {
	setupTaskPluginControllerTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TaskCreateAttempt{}))
	rows := []model.TaskPlugin{
		{Key: jsplugin.SeedancePluginKey, Version: plugins.SeedanceVersion(), APIVersion: 3, Source: model.LongText(plugins.SeedanceSource()), Active: true},
		{Key: jsplugin.MinimaxPluginKey, Version: plugins.MinimaxVersion(), APIVersion: 3, Source: model.LongText(plugins.MinimaxSource()), Active: true},
	}
	for _, row := range rows {
		row.SourceHash = fmt.Sprintf("%x", common.Sha256Raw([]byte(row.Source)))
		require.NoError(t, model.DB.Create(&row).Error)
	}
	router := gin.New()
	router.GET("/api/plugin/task", ListTaskPlugins)
	router.POST("/api/plugin/task", UploadTaskPlugin)
	router.GET("/api/plugin/task/:key", GetTaskPlugin)
	router.GET("/api/task_plugin_options", GetTaskPluginOptions)
	type pluginItem struct {
		Meta                            jsplugin.Meta
		Link, Task, Provider, Transform bool
		SkippedRoutes                   []string `json:"skipped_routes"`
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/plugin/task", nil))
	var list struct {
		Success bool
		Data    []pluginItem
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &list))
	require.True(t, list.Success)
	items := map[string]pluginItem{}
	for _, item := range list.Data {
		items[item.Meta.Key] = item
	}
	for _, row := range rows {
		t.Run(row.Key, func(t *testing.T) {
			item, ok := items[row.Key]
			require.True(t, ok)
			assert.True(t, item.Link)
			assert.False(t, item.Task || item.Provider || item.Transform)
			assert.Equal(t, []string{}, item.SkippedRoutes)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/plugin/task/"+row.Key+"?version="+row.Version, nil))
			var detail struct {
				Success bool
				Data    pluginItem
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &detail))
			require.True(t, detail.Success)
			assert.True(t, detail.Data.Link)
			assert.False(t, detail.Data.Task || detail.Data.Provider || detail.Data.Transform)
			assert.Equal(t, []string{}, detail.Data.SkippedRoutes)

			payload, err := common.Marshal(map[string]any{"source": row.Source, "enabled": false})
			require.NoError(t, err)
			recorder = httptest.NewRecorder()
			upload := httptest.NewRequest(http.MethodPost, "/api/plugin/task", bytes.NewReader(payload))
			upload.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, upload)
			detail.Data = pluginItem{}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &detail))
			require.True(t, detail.Success, recorder.Body.String())
			assert.True(t, detail.Data.Link)
			assert.False(t, detail.Data.Task || detail.Data.Provider || detail.Data.Transform)
			assert.Equal(t, []string{}, detail.Data.SkippedRoutes)
		})
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/task_plugin_options", nil))
	var options struct {
		Success bool
		Data    []struct{ Key string }
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &options))
	require.True(t, options.Success)
	for _, option := range options.Data {
		assert.NotEqual(t, jsplugin.SeedancePluginKey, option.Key)
		assert.NotEqual(t, jsplugin.MinimaxPluginKey, option.Key)
	}
}
