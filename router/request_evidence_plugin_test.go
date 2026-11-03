package router

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidencePluginGenerationCoversRegisteredNativePathsAndLocalRejection(t *testing.T) {
	setupRelayRouterTestDB(t)
	previous := system_setting.GetTaskRequestEvidenceConfig()
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, RetentionDays: 2, StorageDir: t.TempDir(), EncryptionKeyHex: "0101010101010101010101010101010101010101010101010101010101010101"}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, service.FlushRequestEvidence(ctx))
		system_setting.SetTaskRequestEvidenceConfig(previous)
		require.NoError(t, service.InitTaskRequestEvidenceStore(previous))
	})
	plugin := compileRouterPlugin(t, "evidence-paths", "1.0.0", `[
 {method:"POST",path:"/doubao/api/v3/images/generations",type:"submit"},
 {method:"GET",path:"/ali/api/v1/tasks/:task_id",type:"query",render:"native"}
 ]`)
	handlers := testPluginRouteHandlers(func(c *gin.Context, _ *jsplugin.RoutingGeneration, _ jsplugin.RouteBinding) {
		if c.Request.Method == "POST" {
			c.String(401, "local rejection")
			return
		}
		c.Set("id", 7)
		c.Set("token_id", 3)
		c.String(200, "known query")
	})
	outer, registry := newPluginRouterTest(t, []*jsplugin.LoadedPlugin{plugin}, handlers)
	outer.Use(service.APIRequestEvidence())
	outer.NoRoute((&pluginRouteDispatcher{registry: registry}).dispatch)
	assert.Equal(t, 401, performPluginRequest(outer, "POST", "/doubao/api/v3/images/generations").Code)
	assert.Equal(t, 200, performPluginRequest(outer, "GET", "/ali/api/v1/tasks/known").Code)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, service.FlushRequestEvidence(ctx))
	indexes, total, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "request-phase-two"})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	delivered := map[int]string{}
	for _, index := range indexes {
		events, err := model.ListTaskRequestEvidenceEvents(index.Id)
		require.NoError(t, err)
		require.Len(t, events, 2)
		for _, event := range events {
			if event.Stage != model.TaskRequestEvidenceStageClientDelivery {
				continue
			}
			content, status := service.ReadEvidenceEventContent(event, false)
			require.Equal(t, "available", status)
			delivered[event.StatusCode] = string(content.Body)
		}
	}
	assert.Equal(t, map[int]string{401: "local rejection", 200: "known query"}, delivered)
}
