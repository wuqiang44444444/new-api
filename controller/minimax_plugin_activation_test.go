package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "key", Value: jsplugin.MinimaxPluginKey}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/plugin/task/minimax-link/activate", strings.NewReader(`{"version":"`+version+`"}`))
	ActivateTaskPlugin(ctx)

	require.Contains(t, recorder.Body.String(), `"success":true`)
	configuration, err := model.GetMinimaxPluginConfiguration()
	require.NoError(t, err)
	assert.Equal(t, version, configuration.Version)
	_, registeredNatively := jsplugin.DefaultRegistry.Get(jsplugin.MinimaxPluginKey)
	assert.False(t, registeredNatively)
}
