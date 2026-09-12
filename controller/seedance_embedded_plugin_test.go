package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deploymentOldSource = `export const meta = {apiVersion:1,key:"seedance-link",name:"Old",version:"1.0.1",author:{name:"Test"},seedanceProtocols:["feicai_videos_v1"]};
export const seedance = {feicai_videos_v1:{buildCreate(){},parseCreateResponse(){},parseTaskObservation(){}}};`

func TestSeedanceDeploymentPublishesNewVersionOnFirstSync(t *testing.T) {
	setupTaskPluginControllerTest(t)
	previous := seedanceplugin.Default
	seedanceplugin.Default = seedanceplugin.NewStore()
	t.Cleanup(func() {
		seedanceplugin.Default = previous
		delete(taskPluginSyncState.errors, jsplugin.SeedancePluginKey)
	})
	old := model.TaskPlugin{Key: jsplugin.SeedancePluginKey, Version: "1.0.1", APIVersion: 1, Source: deploymentOldSource, Enabled: true}
	require.NoError(t, model.SaveTaskPlugin(&old))
	// The outer native sync already read these old rows before deployment.
	syncSeedanceExtensionPlugins(t.Context(), []model.TaskPlugin{old})
	assert.Empty(t, taskPluginSyncState.errors[jsplugin.SeedancePluginKey])
	active, err := model.GetTaskPluginVersion(jsplugin.SeedancePluginKey, "")
	require.NoError(t, err)
	runtimeVersion, runtimeErrors := seedanceplugin.Default.Describe()
	assert.Equal(t, plugins.SeedanceVersion(), active.Version)
	assert.Equal(t, active.Version, runtimeVersion)
	assert.Empty(t, runtimeErrors)
}

func TestSeedanceDeploymentFailureStopsAdmissionAndRetries(t *testing.T) {
	setupTaskPluginControllerTest(t)
	previous := seedanceplugin.Default
	seedanceplugin.Default = seedanceplugin.NewStore()
	t.Cleanup(func() {
		seedanceplugin.Default = previous
		delete(taskPluginSyncState.errors, jsplugin.SeedancePluginKey)
	})
	old := model.TaskPlugin{Key: jsplugin.SeedancePluginKey, Version: "1.0.1", APIVersion: 1, Source: deploymentOldSource, Enabled: true}
	require.NoError(t, model.SaveTaskPlugin(&old))
	require.NoError(t, seedanceplugin.Default.SyncSnapshot(t.Context(), []model.TaskPlugin{old}))
	channel := model.Channel{Type: constant.ChannelTypeSeedanceLink, Models: "unsupported-provider", Status: 1}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFeicaiVideosV1, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
	require.NoError(t, model.DB.Create(&channel).Error)
	syncSeedanceExtensionPlugins(t.Context(), []model.TaskPlugin{old})
	assert.Contains(t, taskPluginSyncState.errors[jsplugin.SeedancePluginKey], "incompatible with channels")
	_, err := seedanceplugin.Default.ActiveFor(dto.VideoUpstreamProtocolFeicaiVideosV1)
	require.ErrorIs(t, err, seedanceplugin.ErrUnavailable)
	active, err := model.GetTaskPluginVersion(old.Key, "")
	require.NoError(t, err)
	assert.Equal(t, old.Version, active.Version, "failed promotion leaves persistent state unchanged")
	_, err = seedanceplugin.Default.ResolveVersion(t.Context(), old.Version)
	require.NoError(t, err, "frozen historical execution stays available")
	// Removing this test-only incompatible channel unblocks the same store.
	require.NoError(t, model.DB.Delete(&channel).Error)
	syncSeedanceExtensionPlugins(t.Context(), []model.TaskPlugin{old})
	assert.Empty(t, taskPluginSyncState.errors[jsplugin.SeedancePluginKey])
	activeVersion, _ := seedanceplugin.Default.Describe()
	assert.Equal(t, plugins.SeedanceVersion(), activeVersion)
}
