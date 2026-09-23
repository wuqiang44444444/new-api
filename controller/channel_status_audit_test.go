package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginCascadeStatusAuditRetainsManualActor(t *testing.T) {
	setupTaskPluginControllerTest(t)
	previousStore := seedanceplugin.Default
	seedanceplugin.Default = seedanceplugin.NewStore()
	t.Cleanup(func() { seedanceplugin.Default = previousStore })
	cleanupTaskPluginControllerRuntime(t, "lifecycle-only")
	admin := model.User{Username: "audit-admin", Role: common.RoleAdminUser}
	require.NoError(t, model.DB.Create(&admin).Error)
	loaded, err := jsplugin.DefaultRegistry.Register(lifecyclePluginSource, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("lifecycle-only") })
	require.NoError(t, model.SaveTaskPlugin(&model.TaskPlugin{Key: loaded.Meta.Key, APIVersion: 1, Version: "1", Source: lifecyclePluginSource, SourceHash: "hash", Enabled: true}))
	setting := `{"task_plugin_key":"lifecycle-only"}`
	channel := model.Channel{Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: "linked", Models: "doc", Group: "default", BaseURL: common.GetPointer("https://example.invalid"), Setting: &setting}
	require.NoError(t, channel.Insert())
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Set("id", admin.Id)
	c.Set("role", admin.Role)
	c.Set(common.RequestIdKey, "cascade-request")
	c.Params = gin.Params{{Key: "key", Value: "lifecycle-only"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/plugin/task/lifecycle-only/status?cascade=true&force=true", strings.NewReader(`{"enabled":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	SetTaskPluginStatus(c)
	assert.Contains(t, response.Body.String(), `"success":true`)
	var rows []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action = ?", "channel_status_change").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, admin.Id, rows[0].UserId)
	assert.Equal(t, "cascade-request", rows[0].RequestId)
	encoded, err := common.Marshal(rows[0].Other.Op.Params["source"])
	require.NoError(t, err)
	assert.Equal(t, `"manual"`, string(encoded))
}
