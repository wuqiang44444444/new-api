package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The deploy-only lifecycle: the first stored version may become active
// through the existing rule but stays disabled; enabling is an explicit
// administrator action; re-saving an existing version never overwrites the
// administrator's enable choice; a later embedded version never replaces the
// active selection.
func TestMinimaxPluginDeployOnlyLifecycle(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&TaskPlugin{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM task_plugins") })

	seed := TaskPlugin{
		Key:        jsplugin.MinimaxPluginKey,
		APIVersion: 3,
		Version:    plugins.MinimaxVersion(),
		Source:     plugins.MinimaxSource(),
		SourceHash: "hash-1", Enabled: false, Remark: "embedded MiniMax Link extension artifact",
	}
	require.NoError(t, SaveTaskPlugin(&seed))
	stored, err := GetTaskPluginVersion(jsplugin.MinimaxPluginKey, plugins.MinimaxVersion())
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.True(t, stored.Active, "first version follows the existing active rule")
	assert.False(t, stored.Enabled, "deployment must not enable new admission")

	require.NoError(t, SetTaskPluginEnabled(jsplugin.MinimaxPluginKey, true))
	enabled, err := GetTaskPluginVersion(jsplugin.MinimaxPluginKey, plugins.MinimaxVersion())
	require.NoError(t, err)
	assert.True(t, enabled.Enabled)

	// The deploy sync itself never re-saves existing rows: the store's
	// EnsureSeeded contract (pkg/minimaxplugin) covers the enabled-choice
	// preservation; the active selection stays with the first version.
	current, err := GetTaskPluginVersion(jsplugin.MinimaxPluginKey, "")
	require.NoError(t, err)
	assert.Equal(t, plugins.MinimaxVersion(), current.Version)
}

// The upstream query schedule participates in snapshot change detection:
// re-arming the cadence on an otherwise-unchanged observation must register
// as a change so the shared CAS persists it (the JD 15-minute background
// cadence depends on this), and zero-schedule tasks are unaffected.
func TestTaskSnapshotDetectsQueryScheduleChange(t *testing.T) {
	task := &Task{Status: TaskStatusQueued, Progress: "10%"}
	before := task.Snapshot()
	assert.True(t, before.Equal(task.Snapshot()), "no change is a no-op")

	task.PrivateData.VideoUpstreamNextQueryAt = 1727000000
	assert.False(t, before.Equal(task.Snapshot()), "re-armed schedule is a change")

	other := &Task{Status: TaskStatusQueued, Progress: "10%"}
	otherSnap := other.Snapshot()
	assert.True(t, otherSnap.Equal(other.Snapshot()), "zero-schedule tasks keep the old equality behavior")
}

// MiniMax Link settings validation: one credential, the registered protocol,
// no asset library, and the runtime path fields stay host-owned.
func TestValidateMinimaxChannelSettings(t *testing.T) {
	channel := &Channel{
		Type:          constant.ChannelTypeMiniMaxLink,
		Key:           "sk-single",
		BaseURL:       common.GetPointer("https://modelservice.jdcloud.com"),
		OtherSettings: `{"video_upstream_protocol":"jdcloud_video_task_v1","video_upstream_create_path":"/hack","video_upstream_query_path_template":"/hack"}`,
	}
	require.NoError(t, channel.ValidateSettings())
	settings := channel.GetOtherSettings()
	assert.Equal(t, "jdcloud_video_task_v1", string(settings.VideoUpstreamProtocol))
	assert.Empty(t, settings.VideoUpstreamCreatePath)
	assert.Empty(t, settings.VideoUpstreamQueryPathTemplate)
	assert.Equal(t, "none", string(settings.AssetUpstreamProtocol))

	multiKey := &Channel{
		Type:          constant.ChannelTypeMiniMaxLink,
		Key:           "sk-a\nsk-b",
		BaseURL:       common.GetPointer("https://modelservice.jdcloud.com"),
		OtherSettings: `{"video_upstream_protocol":"jdcloud_video_task_v1"}`,
	}
	err := multiKey.ValidateSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one channel credential")

	wrongProtocol := &Channel{
		Type:          constant.ChannelTypeMiniMaxLink,
		Key:           "sk-single",
		BaseURL:       common.GetPointer("https://modelservice.jdcloud.com"),
		OtherSettings: `{"video_upstream_protocol":"hailuo_v1"}`,
	}
	err = wrongProtocol.ValidateSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported MiniMax Link video upstream protocol")
}

// Cross-type uniqueness: the same customer model cannot be enabled on both a
// Seedance and a MiniMax channel; disabled channels never block.
func TestMinimaxCrossTypeModelUniqueness(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Channel{}))
	require.NoError(t, DB.AutoMigrate(&TaskPlugin{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM channels")
		DB.Exec("DELETE FROM task_plugins")
	})

	// The uniqueness gate validates against the active declaration first, so
	// a deployed, explicitly enabled embedded artifact precedes the channels.
	seed := TaskPlugin{
		Key: jsplugin.MinimaxPluginKey, APIVersion: 3,
		Version: plugins.MinimaxVersion(), Source: plugins.MinimaxSource(),
		SourceHash: "hash-1", Enabled: false,
	}
	require.NoError(t, SaveTaskPlugin(&seed))
	require.NoError(t, SetTaskPluginEnabled(jsplugin.MinimaxPluginKey, true))

	seedanceChannel := &Channel{
		Name: "seedance-main", Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled,
		Key: "sk-s", Models: "MiniMax-H3,other-model", Group: "default",
		OtherSettings: `{"video_upstream_protocol":"modelark_v3_volcengine","asset_upstream_protocol":"none"}`,
	}
	require.NoError(t, DB.Create(seedanceChannel).Error)

	conflict := &Channel{
		Name: "minimax-main", Type: constant.ChannelTypeMiniMaxLink, Status: common.ChannelStatusEnabled,
		Key: "sk-m", Models: "MiniMax-H3", Group: "default",
		OtherSettings: `{"video_upstream_protocol":"jdcloud_video_task_v1","asset_upstream_protocol":"none"}`,
	}
	err := ValidateMiniMaxChannelModelUniqueness(DB, conflict)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already enabled")

	// A disabled channel never blocks; the same model may stay configured.
	disabled := &Channel{
		Name: "minimax-disabled", Type: constant.ChannelTypeMiniMaxLink, Status: common.ChannelStatusManuallyDisabled,
		Key: "sk-m2", Models: "MiniMax-H3", Group: "default",
		OtherSettings: `{"video_upstream_protocol":"jdcloud_video_task_v1","asset_upstream_protocol":"none"}`,
	}
	require.NoError(t, ValidateMiniMaxChannelModelUniqueness(DB, disabled))
}
