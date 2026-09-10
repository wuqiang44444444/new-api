package service

import (
	"net/http/httptest"
	"testing"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pinSnapshotTestPluginSource = `
export const meta = {apiVersion: 1, key: "seedance-link", name: "Pin", version: "1.2.3", author: {name: "Test"}, models: ["doc"], fetchMode: "per_task"};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
`

// TestTaskExecutionSnapshotFromContextAcceptsGenerationlessPin pins the
// contract the Seedance middleware relies on: a pinned plugin with no native
// routing generation still produces a complete, credential-free plugin
// snapshot with generation 0.
func TestTaskExecutionSnapshotFromContextAcceptsGenerationlessPin(t *testing.T) {
	loaded, err := pluginruntime.NewRegistry().Register(pinSnapshotTestPluginSource, pluginruntime.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v3/contents/generations/tasks", nil)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: loaded})

	snapshot := TaskExecutionSnapshotFromContext(c)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.TaskPlugin)
	assert.Equal(t, "seedance-link", snapshot.TaskPlugin.Key)
	assert.Equal(t, "1.2.3", snapshot.TaskPlugin.Version)
	assert.Equal(t, 1, snapshot.TaskPlugin.APIVersion)
	assert.Equal(t, uint64(0), snapshot.TaskPlugin.Generation)
	assert.NotEmpty(t, snapshot.RequestPath)
}
