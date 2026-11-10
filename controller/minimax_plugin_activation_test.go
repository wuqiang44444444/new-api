package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxplugin"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivateMiniMaxLinkPublishesChannelConfiguration(t *testing.T) {
	setupTaskPluginControllerTest(t)
	cleanupTaskPluginControllerRuntime(t, jsplugin.MinimaxPluginKey)
	previous := minimaxplugin.Default
	previousSeedance := seedanceplugin.Default
	minimaxplugin.Default = minimaxplugin.NewStore()
	// Activation syncs both typed extensions against this test's database.
	seedanceplugin.Default = seedanceplugin.NewStore()
	t.Cleanup(func() {
		minimaxplugin.Default = previous
		seedanceplugin.Default = previousSeedance
	})
	require.NoError(t, minimaxplugin.Default.EnsureSeeded(context.Background()))
	version := plugins.MinimaxVersion()
	_, err := model.GetTaskPluginVersion(jsplugin.MinimaxPluginKey, version)
	require.NoError(t, err)

	existingChannel := model.Channel{
		Type: constant.ChannelTypeMiniMaxLink, Status: common.ChannelStatusEnabled,
		Name: "Existing JD channel", Models: "existing-video", Group: "default",
		Key: "provider-fixture-key", BaseURL: common.GetPointer("https://provider.invalid"),
		ModelMapping: common.GetPointer(`{"existing-video":"MiniMax-H3"}`),
	}
	existingChannel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: constant.VideoUpstreamProtocolJdCloudTaskV1})
	require.NoError(t, model.DB.Create(&existingChannel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "key", Value: jsplugin.MinimaxPluginKey}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/plugin/task/minimax-link/activate", strings.NewReader(`{"version":"`+version+`"}`))
	ActivateTaskPlugin(ctx)

	require.Contains(t, recorder.Body.String(), `"success":true`)
	configuration, err := model.GetMinimaxPluginConfiguration()
	require.NoError(t, err)
	assert.Equal(t, version, configuration.Version)
	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, existingChannel.Id).Error)
	assert.Equal(t, existingChannel.GetModelMapping(), reloaded.GetModelMapping())
	assert.Equal(t, existingChannel.Status, reloaded.Status)
	catalog, err := model.GetConfiguredMiniMaxPublicModels()
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	assert.Equal(t, "existing-video", catalog[0].API.Video.Creation.Model)
	_, registeredNatively := jsplugin.DefaultRegistry.Get(jsplugin.MinimaxPluginKey)
	assert.False(t, registeredNatively)
}
